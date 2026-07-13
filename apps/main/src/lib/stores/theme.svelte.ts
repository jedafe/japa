import { browser } from '$app/environment';
import { getToken, API_BASE } from '$lib/api';

type Theme = 'light' | 'dark' | 'system';

// Client-only state - safe for SSR because browser check guards all mutations
let currentTheme: Theme = $state('system');

function applyTheme(theme: Theme) {
	if (!browser) return;
	const html = document.documentElement;
	if (theme === 'system') {
		const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
		html.classList.toggle('dark', prefersDark);
	} else {
		html.classList.toggle('dark', theme === 'dark');
	}
}

export function getTheme() {
	return {
		get current() {
			// During SSR, always return 'system' to avoid hydration mismatch
			return browser ? currentTheme : 'system';
		},
		set: (theme: Theme) => {
			currentTheme = theme;
			if (browser) {
				applyTheme(theme);
				const token = getToken();
				fetch(`${API_BASE}/me/preferences`, {
					method: 'PUT',
					headers: {
						'Content-Type': 'application/json',
						...(token ? { Authorization: `Bearer ${token}` } : {})
					},
					body: JSON.stringify({ dark_mode: theme })
				}).catch(() => {});
			}
		},
		init: (serverTheme: Theme) => {
			currentTheme = serverTheme;
			applyTheme(serverTheme);
		}
	};
}

export const theme = getTheme();
