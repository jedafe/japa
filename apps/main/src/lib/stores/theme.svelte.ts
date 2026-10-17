import type { ThemeName } from '$lib/types';

/**
 * Client-side theme controller. The initial theme is rendered server-side into
 * <html data-theme> (from the jc_theme cookie), so there is no flash; this store
 * only handles user-initiated toggles after hydration.
 *
 * 'system' resolves against prefers-color-scheme and follows OS changes while
 * active. The cookie stores the user's *selection* ('system' included); the
 * data-theme attribute always carries the resolved light|dark value.
 */
class ThemeStore {
	current = $state<ThemeName>('light');

	private media: MediaQueryList | null = null;
	// Bumped on OS scheme flips so `resolved` stays reactive while 'system'
	// is selected (matchMedia isn't tracked by $derived/$effect).
	private mediaTick = $state(0);
	private readonly onMediaChange = () => {
		this.mediaTick++;
		if (this.current === 'system') this.applyAttribute();
	};

	/** Sync the store with the server-resolved theme once, after hydration. */
	init(theme: ThemeName) {
		this.current = theme;
		this.applyAttribute();
	}

	toggle() {
		this.set(this.resolved === 'dark' ? 'light' : 'dark');
	}

	set(theme: ThemeName) {
		this.current = theme;
		if (typeof document === 'undefined') return;
		document.cookie = `jc_theme=${theme}; path=/; max-age=31536000; samesite=lax`;
		this.applyAttribute();
	}

	/** The theme actually applied to <html data-theme> — the selection
	 * resolved through prefers-color-scheme when 'system' is set. Use this
	 * for icons/labels that describe the rendered theme. */
	get resolved(): 'light' | 'dark' {
		void this.mediaTick;
		if (this.current !== 'system') return this.current;
		if (typeof window === 'undefined') return 'light';
		return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
	}

	private applyAttribute() {
		if (typeof document === 'undefined') return;
		document.documentElement.setAttribute('data-theme', this.resolved);
		// Follow OS theme changes only while 'system' is selected.
		if (typeof window !== 'undefined') {
			this.media ??= window.matchMedia('(prefers-color-scheme: dark)');
			if (this.current === 'system') {
				this.media.addEventListener('change', this.onMediaChange);
			} else {
				this.media.removeEventListener('change', this.onMediaChange);
			}
		}
	}
}

export const theme = new ThemeStore();
