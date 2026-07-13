let _theme = $state<'light' | 'dark'>('dark');

function setTheme(val: 'light' | 'dark') {
	_theme = val;
}

if (typeof window !== 'undefined') {
	const stored = localStorage.getItem('jc_theme') as 'light' | 'dark' | null;
	if (stored) _theme = stored;

	$effect.root(() => {
		$effect(() => {
			localStorage.setItem('jc_theme', _theme);
			document.documentElement.classList.toggle('dark', _theme === 'dark');
		});
	});
}

export const theme = {
	get current() { return _theme; },
	set: setTheme
};
