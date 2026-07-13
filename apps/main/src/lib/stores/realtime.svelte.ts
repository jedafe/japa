import { browser } from '$app/environment';
import { wsClient } from '$lib/websocket';

// Client-only state - safe for SSR because browser check guards all mutations
let notificationCountOverride = $state(0);
let onlineUserIds = $state<Set<string>>(new Set());
let initialized = $state(false);

export function getRealtime() {
	return {
		get notificationCount() {
			// During SSR, return 0 to avoid hydration mismatch
			return browser ? notificationCountOverride : 0;
		},
		get onlineUserIds() {
			// During SSR, return empty set to avoid hydration mismatch
			return browser ? onlineUserIds : new Set<string>();
		},
		get isInitialized() {
			// During SSR, return false to avoid hydration mismatch
			return browser ? initialized : false;
		},
		setNotificationCount(count: number) {
			notificationCountOverride = count;
		},
		incrementNotificationCount() {
			notificationCountOverride++;
		},
		setOnlineUserIds(ids: Set<string>) {
			onlineUserIds = ids;
		},
		setInitialized(value: boolean) {
			initialized = value;
		}
	};
}

export const realtime = getRealtime();

export function initRealtimeListeners() {
	// Only initialize on client side
	if (!browser) return () => {};
	
	const unsubs: (() => void)[] = [];

	unsubs.push(
		wsClient.on('notification', () => {
			realtime.incrementNotificationCount();
		})
	);

	unsubs.push(
		wsClient.on('online_status', (data: { user_id: string; online: boolean }) => {
			const current = new Set<string>(realtime.onlineUserIds);
			if (data.online) {
				current.add(data.user_id);
			} else {
				current.delete(data.user_id);
			}
			realtime.setOnlineUserIds(current);
		})
	);

	return () => {
		unsubs.forEach((unsub) => unsub());
	};
}
