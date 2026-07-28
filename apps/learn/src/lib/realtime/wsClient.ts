/**
 * WebSocket client for the JapaConnect Real-Time system (Section 10).
 *
 * Backend protocol (internal/websocket):
 *   - Connect:  GET /api/v1/ws?ticket={ticket_id}
 *   - Ticket:   POST /api/v1/auth/ws-ticket -> { "ticket": "..." }
 *   - Envelope: { "type": "<event>", "payload": <any> }
 *   - Server -> client events: notification, presence, typing,
 *     space_activity, feed_update, dm_message, moderation_alert
 *   - Client -> server events: subscribe_space / unsubscribe_space
 *     with payload { "space_id": "..." }
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
	| 'moderation_alert';

type EventHandler = (payload: unknown) => void;

const MAX_RECONNECT_DELAY = 30_000;

/** Mint a short-lived WS ticket via POST /api/v1/auth/ws-ticket. */
async function mintTicket(): Promise<string | null> {
	try {
		const res = await fetch('/api/v1/auth/ws-ticket', {
			method: 'POST',
			credentials: 'include',
			headers: { 'content-type': 'application/json' }
		});
		if (!res.ok) return null;
		const body = (await res.json()) as { ticket?: string };
		return body.ticket ?? null;
	} catch {
		return null;
	}
}

function resolveWsUrl(ticket: string): string {
	if (typeof window === 'undefined') {
		return `ws://localhost:8080/api/v1/ws?ticket=${encodeURIComponent(ticket)}`;
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

	/** Open a connection by minting a fresh ticket. */
	async connect(): Promise<void> {
		this.shouldReconnect = true;
		const ticket = await mintTicket();
		if (!ticket) {
			// Ticket minting failed (feature.dm off, auth lost, network). Schedule
			// a retry so we reconnect if the backend comes back online.
			this.scheduleReconnect();
			return;
		}
		this.open(ticket);
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
