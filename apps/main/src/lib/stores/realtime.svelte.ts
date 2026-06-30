import { wsClient } from '$lib/websocket';

let notificationCountOverride = $state(0);
let onlineUserIds = $state<Set<string>>(new Set());
let initialized = $state(false);

export function getRealtime() {
	return {
		get notificationCount() {
			return notificationCountOverride;
		},
		get onlineUserIds() {
			return onlineUserIds;
		},
		get isInitialized() {
			return initialized;
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
	const unsubs: (() => void)[] = [];

	unsubs.push(
		wsClient.on('notification', () => {
			realtime.incrementNotificationCount();
		})
	);

	unsubs.push(
		wsClient.on('online_status', (data: { user_id: string; online: boolean }) => {
			const current = new Set(realtime.onlineUserIds);
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
