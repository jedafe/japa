import type { ThemeName } from '$lib/types';

/**
 * Client-side theme controller. The initial theme is rendered server-side into
 * <html data-theme> (from the jc_theme cookie), so there is no flash; this store
 * only handles user-initiated toggles after hydration.
 */
class ThemeStore {
	current = $state<ThemeName>('light');

	/** Sync the store with the server-resolved theme once, after hydration. */
	init(theme: ThemeName) {
		this.current = theme;
	}

	toggle() {
		this.set(this.current === 'dark' ? 'light' : 'dark');
	}

	set(theme: ThemeName) {
		this.current = theme;
		if (typeof document === 'undefined') return;
		document.documentElement.setAttribute('data-theme', theme);
		document.cookie = `jc_theme=${theme}; path=/; max-age=31536000; samesite=lax`;
	}
}

export const theme = new ThemeStore();
