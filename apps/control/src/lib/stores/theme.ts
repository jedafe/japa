import { writable } from 'svelte/store';

export const theme = writable<'light' | 'dark'>('dark');

if (typeof window !== 'undefined') {
	const stored = localStorage.getItem('jc_theme') as 'light' | 'dark' | null;
	if (stored) theme.set(stored);

	theme.subscribe((value) => {
		localStorage.setItem('jc_theme', value);
		document.documentElement.classList.toggle('dark', value === 'dark');
	});
}
