import { api } from '$lib/api/client';

/**
 * Backend protocol (internal/websocket):
 *   - Connect:  GET /api/v1/ws?ticket={ticket_id}
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

export type WSEventType =
	| 'notification'
	| 'presence'
	| 'typing'
	| 'space_activity'
	| 'feed_update'
	| 'dm_message'
	| 'moderation_alert'
	| 'poll_updated';

type EventHandler = (payload: unknown) => void;

const MAX_RECONNECT_DELAY = 30_000;
const MAX_RECONNECT_ATTEMPTS = 10;

type MintResult = { ticket: string | null; status: number };

/** Mint a short-lived WS ticket via POST /api/v1/auth/ws-ticket. */
async function mintTicket(): Promise<MintResult> {
	try {
		const res = await api.post<{ ticket: string }>('/auth/ws-ticket');
		return { ticket: res.data?.ticket ?? null, status: res.status };
	} catch {
		return { ticket: null, status: 0 };
	}
}

function resolveWsUrl(ticket: string): string {
	// If a public WebSocket URL is configured, use it. In production this is
	// typically wss://api.example.com; in local dev it can be set to
	// ws://localhost:8080 to bypass the Vite proxy.
	const publicUrl = import.meta.env.PUBLIC_JC_WS_URL as string | undefined;
	if (publicUrl) {
		return `${publicUrl}?ticket=${encodeURIComponent(ticket)}`;
	}
	if (typeof window === 'undefined') {
		return '';
	}
	const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	return `${proto}//${window.location.host}/api/v1/ws?ticket=${encodeURIComponent(ticket)}`;
}

class WsClient {
	private socket: WebSocket | null = null;
	private handlers = new Map<string, Set<EventHandler>>();
	private reconnectAttempts = 0;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private shouldReconnect = false;
	private isConnecting = false;

	/** Open a connection by minting a fresh ticket. Idempotent. */
	async connect(): Promise<void> {
		this.shouldReconnect = true;
		this.clearReconnect();
		// Prevent concurrent mint/open cycles while a previous connect is still
		// in progress or an open socket already exists.
		if (this.isConnecting || this.socket) return;
		this.isConnecting = true;
		const { ticket, status } = await mintTicket();
		if (!ticket) {
			// Ticket minting failed (feature.dm off, auth lost, network).
			// Don't retry permanent conditions (401/403/404) to avoid a
			// ticket-spam loop; transient network/server errors keep the
			// exponential backoff.
			this.isConnecting = false;
			if (status === 401 || status === 403 || status === 404) {
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
		if (this.socket) return;
		this.isConnecting = true;

		const url = resolveWsUrl(ticket);
		let socket: WebSocket;
		try {
			socket = new WebSocket(url);
		} catch {
			this.isConnecting = false;
			this.scheduleReconnect();
			return;
		}

		this.socket = socket;

		socket.onopen = () => {
			this.isConnecting = false;
			this.reconnectAttempts = 0;
		};

		socket.onclose = () => {
			this.isConnecting = false;
			this.socket = null;
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
		// Cap reconnect attempts so a permanently-down backend doesn't spin
		// forever. The caller (realtime store) can re-trigger connect() if
		// conditions change (e.g. feature flag toggled, user re-authenticates).
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
