import { browser } from '$app/environment';
import type { ThemeName } from '$lib/types';

/**
 * Theme store — mirrors the Learn/Main app pattern. The server sets
 * data-theme on <html> via transformPageChunk in hooks.server.ts; this store
 * mirrors it client-side so the toggle can update without a full reload.
 */

let currentTheme = $state<ThemeName>('light');

export const theme = {
	get current(): ThemeName {
		return currentTheme;
	},
	init(serverTheme: ThemeName) {
		currentTheme = serverTheme;
	},
	toggle() {
		const next: ThemeName = currentTheme === 'dark' ? 'light' : 'dark';
		currentTheme = next;
		if (browser) {
			document.documentElement.setAttribute('data-theme', next);
			document.cookie = `jc_theme=${next}; path=/; max-age=31536000; samesite=lax`;
		}
	}
};
