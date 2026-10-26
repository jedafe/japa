// §35/§124 — client-side push-subscription reconciliation.
// When the OS rotates or revokes the browser's push subscription
// (pushsubscriptionchange — e.g. after a VAPID key change), the service
// worker re-subscribes but cannot register the new endpoint with the
// backend: it has no access token. Reconciliation therefore lives in the
// page: on every authenticated load (and on the jc:push-subscription-changed
// message the worker broadcasts) we ensure the live browser subscription is
// present in /me/push-tokens, and rebuild it when the OS dropped it while
// permission is still granted.
// Ref: japaconnect-engineering/sections/35-notification-system.md
// Ref: .jeos/sections/124-progressive-web-app.md
import { getVapidKey, getPushTokens, subscribePush } from '$lib/api/hub';

export function urlBase64ToUint8Array(base64String: string): Uint8Array {
	const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
	const base64 = (base64String + padding).replace(/\-/g, '+').replace(/_/g, '/');
	const raw = atob(base64);
	const out = new Uint8Array(raw.length);
	for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
	return out;
}

export function arrayBufferToBase64(buffer: ArrayBuffer | null): string {
	if (!buffer) return '';
	const bytes = new Uint8Array(buffer);
	let binary = '';
	for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
	return btoa(binary);
}

function subscriptionBody(sub: PushSubscription): {
	endpoint: string;
	p256dh: string;
	auth: string;
} {
	return {
		endpoint: sub.endpoint,
		p256dh: arrayBufferToBase64(sub.getKey('p256dh')),
		auth: arrayBufferToBase64(sub.getKey('auth'))
	};
}

let syncing = false;

/**
 * Reconcile the browser's push subscription with the backend token list.
 * Safe to call on every authed load — it returns early when the
 * subscription is already in sync or push isn't possible here, and
 * concurrent invocations (layout effect + worker broadcast) collapse to
 * one in-flight sync.
 */
export async function syncPushSubscription(): Promise<void> {
	if (syncing) return;
	syncing = true;
	try {
		await doSyncPushSubscription();
	} finally {
		syncing = false;
	}
}

async function doSyncPushSubscription(): Promise<void> {
	if (
		typeof navigator === 'undefined' ||
		!('serviceWorker' in navigator) ||
		!('PushManager' in window) ||
		typeof Notification === 'undefined' ||
		Notification.permission === 'denied'
	) {
		return;
	}
	const reg = await navigator.serviceWorker.ready;
	let sub = await reg.pushManager.getSubscription();

	const tokensRes = await getPushTokens();
	if (tokensRes.error || !Array.isArray(tokensRes.data)) return;
	const endpoints = new Set(tokensRes.data.map((t) => t.endpoint));

	if (sub && endpoints.has(sub.endpoint)) return; // already in sync

	if (sub) {
		// The endpoint rotated (pushsubscriptionchange) — register the new
		// endpoint. The stale row lingers until a failed delivery prunes it
		// server-side; it can't be identified by browser here.
		await subscribePush(subscriptionBody(sub));
		return;
	}

	// No browser subscription but permission was already granted — the OS
	// dropped the subscription (or the worker's re-subscribe couldn't run).
	// Never subscribe when permission is 'default': silently prompting for
	// notification permission on page load is not acceptable.
	if (Notification.permission !== 'granted') return;
	const vapid = await getVapidKey();
	if (vapid.error || !vapid.data?.publicKey) return;
	try {
		sub = await reg.pushManager.subscribe({
			userVisibleOnly: true,
			applicationServerKey: urlBase64ToUint8Array(vapid.data.publicKey) as BufferSource
		});
	} catch {
		return;
	}
	await subscribePush(subscriptionBody(sub));
}
