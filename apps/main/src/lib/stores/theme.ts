import { browser } from '$app/environment';

type Theme = 'light' | 'dark' | 'system';

let currentTheme: Theme = $state('system');

function applyTheme(theme: Theme) {
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
			return currentTheme;
		},
		set: (theme: Theme) => {
			currentTheme = theme;
			if (browser) {
				applyTheme(theme);
				fetch('/api/v1/users/me/preferences', {
					method: 'PUT',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ dark_mode: theme })
				}).catch(() => {});
			}
		},
		init: (serverTheme: Theme) => {
			currentTheme = serverTheme;
			if (browser) applyTheme(serverTheme);
		}
	};
}

export const theme = getTheme();
