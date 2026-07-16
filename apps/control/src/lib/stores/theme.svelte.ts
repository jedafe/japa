let theme = $state<'light' | 'dark'>(
	typeof localStorage !== 'undefined'
		? (localStorage.getItem('jc_theme') as 'light' | 'dark') ?? 'dark'
		: 'dark'
);

export function getTheme() {
	return theme;
}

export function setTheme(value: 'light' | 'dark') {
	theme = value;
	localStorage.setItem('jc_theme', value);
	document.documentElement.classList.toggle('dark', value === 'dark');
}

export function toggleTheme() {
	setTheme(theme === 'dark' ? 'light' : 'dark');
}
