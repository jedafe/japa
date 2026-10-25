// Reactive "hide chrome on scroll" state for sticky top bars (mobile).
// `hidden` is true while the user scrolls down past a small deadzone, so
// page-level sticky bars (filter chips, sort bars) can take the top slot;
// scrolling back up — or landing near the top — reveals the bar again.
export function createScrollHide() {
	let hidden = $state(false);

	$effect(() => {
		let lastY = window.scrollY;
		const onScroll = () => {
			const y = window.scrollY;
			const dy = y - lastY;
			if (y <= 64 || dy < -6) hidden = false;
			else if (dy > 6) hidden = true;
			lastY = y;
		};
		window.addEventListener('scroll', onScroll, { passive: true });
		return () => window.removeEventListener('scroll', onScroll);
	});

	return {
		get hidden() {
			return hidden;
		}
	};
}
