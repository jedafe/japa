// §124 — PWA install state + service-worker auth reporting, shared by all
// JC SvelteKit surfaces (main, academy, docs).
// Captures the browser's beforeinstallprompt event so surfaces (install
// banner, Devices settings row) can trigger the native install dialog on
// demand. iOS Safari never fires beforeinstallprompt, so standalone
// detection and manual Add-to-Home-Screen guidance are exposed too.
// Ref: .jeos/sections/124-progressive-web-app.md

interface BeforeInstallPromptEvent extends Event {
	prompt(): Promise<void>;
	userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
}

interface NavigatorStandalone extends Navigator {
	standalone?: boolean;
}

let deferredPrompt = $state<BeforeInstallPromptEvent | null>(null);
let installed = $state(false);
let initialized = false;
let authListenerAttached = false;
// Last auth state reported to the service worker — re-sent when the worker
// takes control (controllerchange), because at mount the controller is
// usually still null and the first postMessage would be dropped silently.
let lastAuthed: boolean | null = null;

// Attached on the first reportAuth call — NOT inside init(), which is
// gated by pwa.install_prompt_enabled. The auth signal must flow even when
// the admin has turned the install prompt off.
function attachAuthResend(): void {
	if (authListenerAttached || typeof navigator === 'undefined' || !('serviceWorker' in navigator))
		return;
	authListenerAttached = true;
	navigator.serviceWorker.addEventListener('controllerchange', () => {
		if (lastAuthed !== null) pwaInstall.reportAuth(lastAuthed);
	});
}

function detectIos(): boolean {
	if (typeof window === 'undefined') return false;
	return (
		/iphone|ipad|ipod/i.test(navigator.userAgent) ||
		// iPadOS "desktop mode" reports a Mac UA — detect it by touch points.
		(navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
	);
}

export const pwaInstall = {
	get canPrompt(): boolean {
		return deferredPrompt !== null;
	},
	get installed(): boolean {
		return installed || pwaInstall.standalone;
	},
	get standalone(): boolean {
		if (typeof window === 'undefined') return false;
		return (
			window.matchMedia('(display-mode: standalone)').matches ||
			(navigator as NavigatorStandalone).standalone === true
		);
	},
	get isIos(): boolean {
		return detectIos();
	},
	/** Show the manual iOS guidance when the browser can't prompt natively. */
	get needsManualIos(): boolean {
		return pwaInstall.isIos && !pwaInstall.standalone;
	},

	/** Attach the event listeners. Idempotent — safe to call per layout. */
	init(): void {
		if (typeof window === 'undefined' || initialized) return;
		initialized = true;
		window.addEventListener('beforeinstallprompt', (event) => {
			event.preventDefault();
			deferredPrompt = event as BeforeInstallPromptEvent;
		});
		window.addEventListener('appinstalled', () => {
			installed = true;
			deferredPrompt = null;
		});
	},

	/** Trigger the native install dialog. Resolves true when accepted. */
	async prompt(): Promise<boolean> {
		const evt = deferredPrompt;
		if (!evt) return false;
		deferredPrompt = null;
		await evt.prompt();
		const choice = await evt.userChoice.catch(() => null);
		return choice?.outcome === 'accepted';
	},

	/**
	 * Tell the service worker whether the current session is authenticated
	 * (`jc:pwa-auth`) so its caches only ever store guest content. The value
	 * is remembered and re-sent on controllerchange — the worker can't see
	 * the Cookie header, so this message is its only auth signal.
	 */
	reportAuth(authed: boolean): void {
		lastAuthed = authed;
		attachAuthResend();
		if (typeof navigator === 'undefined') return;
		navigator.serviceWorker?.controller?.postMessage({ type: 'jc:pwa-auth', authed });
	}
};
