/**
 * WebSocket client for the JapaConnect Real-Time system (Section 10).
 *
 * Backend protocol (internal/websocket):
 *   - Connect:  GET /api/v1/ws (ticket via Sec-WebSocket-Protocol: jc-ticket.<uuid>)
 *   - Ticket:   POST /api/v1/auth/ws-ticket -> { "ticket": "..." }
 *   - Envelope: { "type": "<event>", "payload": <any> }
 *   - Server -> client events: notification, presence, typing,
 *     space_activity, feed_update, dm_message, moderation_alert
 *   - Client -> server events: subscribe_space / unsubscribe_space
 *     with payload { "space_id": "..." }, and typing with payload
 *     { "conversation_id": "...", "is_typing": true|false }
 *   - Liveness: server sends WS-level Ping frames every 30s; the browser
 *     auto-responds with Pong (no manual JSON ping needed).
 *
 * Auto-reconnect uses exponential backoff capped at 30s. The class is pure
 * TypeScript (no Svelte) so it can be driven from a Runes store.
 */

import { PUBLIC_JC_API_URL } from '$env/static/public';
import { buildApiUrl } from '@japaconnect/api-url';

export type WSEventType =
	| 'notification'
	| 'presence'
	| 'typing'
	| 'space_activity'
	| 'feed_update'
	| 'dm_message'
	| 'moderation_alert';

type EventHandler = (payload: unknown) => void;

const MAX_RECONNECT_DELAY = 30_000;
const MAX_RECONNECT_ATTEMPTS = 10;
// A connection must stay open this long before the backoff counter resets.
const STABLE_CONNECTION_MS = 15_000;

type MintResult = { ticket: string | null; status: number };

/** Mint a short-lived WS ticket via POST /api/v1/auth/ws-ticket. */
async function mintTicket(): Promise<MintResult> {
	try {
		const res = await fetch(buildApiUrl('/auth/ws-ticket', PUBLIC_JC_API_URL), {
			method: 'POST',
			credentials: 'include',
			headers: { 'content-type': 'application/json', 'x-jc-surface': 'academy' }
		});
		if (!res.ok) return { ticket: null, status: res.status };
		// Standard envelope: { "data": { "ticket": "..." } }
		const body = (await res.json()) as { data?: { ticket?: string } };
		return { ticket: body.data?.ticket ?? null, status: res.status };
	} catch {
		return { ticket: null, status: 0 };
	}
}

function resolveWsUrl(): string {
	// In production PUBLIC_JC_API_URL is absolute (https://api.example.com/api/v1)
	// — derive the WS origin from it. In dev it's a relative /api/v1 path that
	// goes through the Vite proxy, so fall back to the page origin.
	try {
		const api = new URL(PUBLIC_JC_API_URL, 'http://localhost:8080');
		if (api.protocol === 'https:' || api.protocol === 'http:') {
			const wsProto = api.protocol === 'https:' ? 'wss:' : 'ws:';
			return `${wsProto}//${api.host}/api/v1/ws`;
		}
	} catch {
		// fall through to same-origin
	}
	if (typeof window === 'undefined') {
		return `ws://localhost:8080/api/v1/ws`;
	}
	const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	return `${proto}//${window.location.host}/api/v1/ws`;
}

class WsClient {
	private socket: WebSocket | null = null;
	private handlers = new Map<string, Set<EventHandler>>();
	private reconnectAttempts = 0;
	private lastOpenedAt = 0;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private shouldReconnect = false;
	private isConnecting = false;

	/** Open a connection by minting a fresh ticket. Idempotent. */
	async connect(): Promise<void> {
		this.shouldReconnect = true;
		this.clearReconnect();
		// Prevent concurrent mint/open cycles while a connect is in progress
		// or an open socket already exists.
		if (this.isConnecting || this.socket) return;
		this.isConnecting = true;
		const { ticket, status } = await mintTicket();
		if (!ticket) {
			// Ticket minting failed (auth lost, network, feature gate).
			// A 404 means the realtime route/flag is off — retrying is
			// pointless until it changes. Other failures use bounded
			// exponential backoff (cap keeps a dead session from minting
			// forever).
			this.isConnecting = false;
			if (status === 404) {
				this.shouldReconnect = false;
				return;
			}
			this.scheduleReconnect();
			return;
		}
		await this.open(ticket);
	}

	/** Gracefully close the connection and stop reconnecting. */
	disconnect(): void {
		this.shouldReconnect = false;
		this.clearReconnect();
		if (this.socket) {
			this.socket.onclose = null;
			this.socket.onerror = null;
			this.socket.onmessage = null;
			this.socket.onopen = null;
			try {
				this.socket.close();
			} catch {
				/* ignore */
			}
			this.socket = null;
		}
	}

	/** Register a handler for a named event type. */
	on(event: WSEventType, handler: EventHandler): void {
		let set = this.handlers.get(event);
		if (!set) {
			set = new Set();
			this.handlers.set(event, set);
		}
		set.add(handler);
	}

	/** Remove a previously-registered handler. */
	off(event: WSEventType, handler: EventHandler): void {
		this.handlers.get(event)?.delete(handler);
	}

	/** Subscribe to live updates for a space (client -> server). */
	subscribeSpace(spaceId: string): void {
		this.send({ type: 'subscribe_space', payload: { space_id: spaceId } });
	}

	/** Unsubscribe from a space's live updates (client -> server). */
	unsubscribeSpace(spaceId: string): void {
		this.send({ type: 'unsubscribe_space', payload: { space_id: spaceId } });
	}

	/** Send a typing indicator to the other participant in a DM conversation
	 * (client -> server). The server forwards it to the other participant
	 * only. Throttle on the caller side (e.g. every 3s, not per keystroke)
	 * to avoid excessive server-side DB lookups. */
	sendTyping(conversationId: string, isTyping: boolean): void {
		this.send({
			type: 'typing',
			payload: { conversation_id: conversationId, is_typing: isTyping }
		});
	}

	/** True when a WebSocket is currently open. */
	get isOpen(): boolean {
		return this.socket?.readyState === WebSocket.OPEN;
	}

	private send(event: { type: string; payload: unknown }): void {
		if (!this.socket || this.socket.readyState !== WebSocket.OPEN) return;
		try {
			this.socket.send(JSON.stringify(event));
		} catch {
			/* ignore */
		}
	}

	private async open(ticket: string): Promise<void> {
		if (this.isConnecting || this.socket) return;
		this.isConnecting = true;

		const url = resolveWsUrl();
		let socket: WebSocket;
		try {
			socket = new WebSocket(url, [`jc-ticket.${ticket}`]);
		} catch {
			this.isConnecting = false;
			this.scheduleReconnect();
			return;
		}

		this.socket = socket;

		socket.onopen = () => {
			this.isConnecting = false;
			this.lastOpenedAt = Date.now();
		};

		socket.onclose = () => {
			this.isConnecting = false;
			this.socket = null;
			// Only a connection that survived the stability window resets the
			// backoff; a socket closed right after open keeps counting attempts.
			if (Date.now() - this.lastOpenedAt >= STABLE_CONNECTION_MS) {
				this.reconnectAttempts = 0;
			}
			if (this.shouldReconnect) this.scheduleReconnect();
		};

		socket.onerror = () => {
			// onclose will follow and handle reconnect.
		};

		socket.onmessage = (ev: MessageEvent) => {
			this.handleMessage(ev);
		};
	}

	private handleMessage(ev: MessageEvent): void {
		let payload: { type?: string; payload?: unknown };
		try {
			payload = JSON.parse(ev.data);
		} catch {
			return;
		}
		const type = payload?.type;
		if (!type) return;
		const set = this.handlers.get(type);
		if (!set) return;
		for (const handler of set) {
			try {
				handler(payload.payload);
			} catch {
				/* swallow handler errors */
			}
		}
	}

	private scheduleReconnect(): void {
		if (!this.shouldReconnect) return;
		// Cap attempts so a permanently-down backend doesn't mint tickets
		// forever; realtime.connect() re-arms the client when called again.
		if (this.reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
			this.shouldReconnect = false;
			return;
		}
		this.clearReconnect();
		const delay = Math.min(1000 * 2 ** this.reconnectAttempts, MAX_RECONNECT_DELAY);
		this.reconnectAttempts++;
		this.reconnectTimer = setTimeout(() => {
			void this.connect();
		}, delay);
	}

	private clearReconnect(): void {
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
	}
}

export const wsClient = new WsClient();
export { WsClient };
