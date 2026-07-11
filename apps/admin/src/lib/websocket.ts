import { browser } from '$app/environment';
import { PUBLIC_JC_WS_URL } from '$env/static/public';

const WS_BASE_URL = PUBLIC_JC_WS_URL ?? 'ws://localhost:8080';

type EventCallback = (data: any) => void;
type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

interface WSMessage {
	type: string;
	data: any;
}

class AdminWebSocketClient {
	private ws: WebSocket | null = null;
	private ticket: string | null = null;
	private reconnectAttempts = 0;
	private maxReconnectAttempts = 5;
	private baseReconnectDelay = 1000;
	private listeners = new Map<string, Set<EventCallback>>();
	private heartbeatInterval: ReturnType<typeof setInterval> | null = null;
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private intentionalClose = false;
	private _state: ConnectionState = 'disconnected';

	get state(): ConnectionState {
		return this._state;
	}

	private setState(state: ConnectionState) {
		this._state = state;
	}

	async connect(token?: string): Promise<void> {
		if (!browser) return;
		if (this.ws?.readyState === WebSocket.OPEN) return;

		this.intentionalClose = false;
		this.setState('connecting');

		try {
			// Use the token passed from server-side layout data.
			// Falls back to reading from page data if not provided.
			if (!token) {
				this.setState('disconnected');
				return;
			}

			const ticketRes = await fetch('/api/v1/auth/ws-ticket', {
				method: 'POST',
				headers: { Authorization: `Bearer ${token}` }
			});
			if (!ticketRes.ok) {
				this.setState('disconnected');
				return;
			}
			const { data } = await ticketRes.json();
			this.ticket = data.ticket;

			const ws = new WebSocket(`${WS_BASE_URL}/ws?ticket=${this.ticket}`);
			this.ws = ws;

			ws.onopen = () => {
				this.reconnectAttempts = 0;
				this.setState('connected');
				this.startHeartbeat();
				this.emit('connected', {});
			};

			ws.onmessage = (event) => {
				try {
					const msg: WSMessage = JSON.parse(event.data);
					if (msg.type === 'pong') return;
					this.emit(msg.type, msg.data);
				} catch {
					// ignore malformed messages
				}
			};

			ws.onclose = () => {
				this.stopHeartbeat();
				this.ws = null;
				this.ticket = null;
				if (!this.intentionalClose) {
					this.setState('reconnecting');
					this.scheduleReconnect();
				} else {
					this.setState('disconnected');
				}
			};

			ws.onerror = () => {
				ws.close();
			};
		} catch {
			this.setState('disconnected');
		}
	}

	disconnect() {
		this.intentionalClose = true;
		this.stopHeartbeat();
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
		if (this.ws) {
			this.ws.close();
			this.ws = null;
		}
		this.ticket = null;
		this.setState('disconnected');
	}

	on(event: string, callback: EventCallback): () => void {
		if (!this.listeners.has(event)) {
			this.listeners.set(event, new Set());
		}
		this.listeners.get(event)!.add(callback);
		return () => {
			this.listeners.get(event)?.delete(callback);
		};
	}

	private emit(event: string, data: any) {
		this.listeners.get(event)?.forEach((cb) => cb(data));
		this.listeners.get('*')?.forEach((cb) => cb({ type: event, data }));
	}

	private startHeartbeat() {
		this.stopHeartbeat();
		this.heartbeatInterval = setInterval(() => {
			if (this.ws?.readyState === WebSocket.OPEN) {
				this.ws.send(JSON.stringify({ type: 'ping' }));
			}
		}, 30_000);
	}

	private stopHeartbeat() {
		if (this.heartbeatInterval) {
			clearInterval(this.heartbeatInterval);
			this.heartbeatInterval = null;
		}
	}

	private scheduleReconnect() {
		if (this.reconnectAttempts >= this.maxReconnectAttempts) {
			this.setState('disconnected');
			return;
		}
		const delay = this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts);
		this.reconnectAttempts++;
		this.reconnectTimer = setTimeout(() => {
			this.connect();
		}, delay);
	}
}

export const wsClient = new AdminWebSocketClient();
