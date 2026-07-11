import { browser } from '$app/environment';
import { PUBLIC_JC_WS_URL } from '$env/static/public';
import { getToken } from '$lib/api';

const WS_BASE_URL = PUBLIC_JC_WS_URL ?? 'ws://localhost:8080';

type EventCallback = (data: any) => void;

type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

interface WSMessage {
	type: string;
	data: any;
}

class WebSocketClient {
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
	private stateListeners = new Set<(state: ConnectionState) => void>();

	get state(): ConnectionState {
		return this._state;
	}

	private setState(state: ConnectionState) {
		this._state = state;
		this.stateListeners.forEach((cb) => cb(state));
	}

	async connect(): Promise<void> {
		if (!browser) return;
		if (this.ws?.readyState === WebSocket.OPEN) return;

		this.intentionalClose = false;
		this.setState('connecting');

		try {
			const token = getToken();
			if (!token) {
				this.setState('disconnected');
				return;
			}

			const res = await fetch('/api/v1/auth/ws-ticket', {
				method: 'POST',
				headers: { Authorization: `Bearer ${token}` }
			});

			if (!res.ok) {
				this.setState('disconnected');
				return;
			}

			const body = await res.json();
			this.ticket = body.data?.ticket ?? body.ticket;
			if (!this.ticket) {
				this.setState('disconnected');
				return;
			}

			const wsUrl = `${WS_BASE_URL}/api/v1/ws?ticket=${this.ticket}`;
			this.ws = new WebSocket(wsUrl);

			this.ws.onopen = () => {
				this.reconnectAttempts = 0;
				this.setState('connected');
				this.startHeartbeat();
			};

			this.ws.onmessage = (event: MessageEvent) => {
				try {
					const message = JSON.parse(event.data as string) as WSMessage;
					if (message.type === 'pong') return;
					this.emit(message.type, message.data);
				} catch {
					// ignore malformed messages
				}
			};

			this.ws.onclose = () => {
				this.stopHeartbeat();
				this.ws = null;
				if (!this.intentionalClose) {
					this.setState('reconnecting');
					this.attemptReconnect();
				} else {
					this.setState('disconnected');
				}
			};

			this.ws.onerror = () => {
				// onclose fires after onerror, so reconnect is handled there
			};
		} catch {
			this.setState('disconnected');
			this.attemptReconnect();
		}
	}

	disconnect() {
		this.intentionalClose = true;
		this.clearReconnectTimer();
		this.stopHeartbeat();
		if (this.ws) {
			this.ws.close();
			this.ws = null;
		}
		this.setState('disconnected');
	}

	on(event: string, callback: EventCallback): () => void {
		if (!this.listeners.has(event)) this.listeners.set(event, new Set());
		this.listeners.get(event)!.add(callback);
		return () => this.off(event, callback);
	}

	off(event: string, callback: EventCallback) {
		this.listeners.get(event)?.delete(callback);
	}

	onStateChange(callback: (state: ConnectionState) => void): () => void {
		this.stateListeners.add(callback);
		return () => this.stateListeners.delete(callback);
	}

	send(type: string, data: Record<string, unknown> = {}) {
		if (this.ws?.readyState !== WebSocket.OPEN) return;
		this.ws.send(JSON.stringify({ type, data }));
	}

	private emit(event: string, data: any) {
		this.listeners.get(event)?.forEach((cb) => cb(data));
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

	private attemptReconnect() {
		if (this.reconnectAttempts >= this.maxReconnectAttempts) {
			this.setState('disconnected');
			return;
		}
		this.reconnectAttempts++;
		const delay = this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
		this.clearReconnectTimer();
		this.reconnectTimer = setTimeout(() => this.connect(), delay);
	}

	private clearReconnectTimer() {
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
	}
}

export const wsClient = new WebSocketClient();
