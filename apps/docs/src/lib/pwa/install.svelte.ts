// §124 — PWA install state shared across the app.
// Captures the browser's beforeinstallprompt event so surfaces (banner,
// Devices settings row) can trigger the native install dialog on demand.
// Also detects iOS standalone and offers manual Add-to-Home-Screen guidance
// (iOS Safari never fires beforeinstallprompt).
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
		if (typeof window === 'undefined') return false;
		return /iphone|ipad|ipod/i.test(navigator.userAgent);
	},
	/** Show the manual iOS guidance when the browser can't prompt natively. */
	get needsManualIos(): boolean {
		return pwaInstall.isIos && !pwaInstall.standalone;
	},

	init(): void {
		if (typeof window === 'undefined') return;
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
	}
};
