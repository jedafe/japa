import { writable } from 'svelte/store';
import { browser } from '$app/environment';

function createThemeStore() {
	const { subscribe, set, update } = writable<'light' | 'dark' | 'system'>('system');

	if (browser) {
		const stored = localStorage.getItem('jc_theme') as 'light' | 'dark' | 'system' | null;
		if (stored) {
			set(stored);
			applyTheme(stored);
		} else {
			applyTheme('system');
		}
	}

	return {
		subscribe,
		set: (value: 'light' | 'dark' | 'system') => {
			set(value);
			if (browser) {
				localStorage.setItem('jc_theme', value);
				applyTheme(value);
			}
		},
	};
}

function applyTheme(mode: 'light' | 'dark' | 'system') {
	if (!browser) return;

	const root = document.documentElement;
	root.classList.remove('light', 'dark');

	if (mode === 'system') {
		const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
		root.classList.add(prefersDark ? 'dark' : 'light');
	} else {
		root.classList.add(mode);
	}
}

export const theme = createThemeStore();

export const isDark = writable(false);

if (browser) {
	const mq = window.matchMedia('(prefers-color-scheme: dark)');
	mq.addEventListener('change', () => {
		const current = localStorage.getItem('jc_theme') as 'light' | 'dark' | 'system' | null;
		if (!current || current === 'system') {
			applyTheme('system');
			isDark.set(mq.matches);
		}
	});

	const stored = localStorage.getItem('jc_theme') as 'light' | 'dark' | 'system' | null;
	if (stored === 'dark' || (!stored && mq.matches)) {
		isDark.set(true);
	}
}
