import type { Action } from 'svelte/action';
import type { LightboxItem } from '$lib/components/ui/MediaLightbox.svelte';

// Ref: §23 main-site-frontend — shared fullscreen media viewer.
// One MediaLightbox instance is mounted in (app)/+layout.svelte; any surface
// pushes items via openLightbox() or the use:imgLightbox delegated action.

interface LightboxState {
	open: boolean;
	index: number;
	items: LightboxItem[];
}

export const lightbox: LightboxState = $state({ open: false, index: 0, items: [] });

export function openLightbox(items: LightboxItem[], index = 0) {
	lightbox.items = items;
	lightbox.index = Math.max(0, Math.min(index, items.length - 1));
	lightbox.open = items.length > 0;
}

function toItem(el: HTMLImageElement): LightboxItem {
	return { id: el.src, url: el.currentSrc || el.src, alt_text: el.alt || null };
}

/**
 * Delegated click-to-zoom for <img> descendants of the attached container —
 * intended for {@html}-rendered bodies (posts, replies, pages) where the
 * images are not component-managed. Images inside <a>/<button> are skipped
 * so existing link/selection behaviour is preserved. Zoomable images get
 * tabindex so Enter works for keyboard users.
 */
export const imgLightbox: Action<HTMLElement> = (node) => {
	function open(e: Event) {
		const img = (e.target as HTMLElement).closest?.('img');
		if (!img || !node.contains(img)) return;
		if (img.closest('a, button')) return;
		const imgs = [...node.querySelectorAll('img')];
		const i = imgs.indexOf(img);
		if (i < 0) return;
		e.stopPropagation();
		openLightbox(imgs.map(toItem), i);
	}
	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ' ') {
			const img = (e.target as HTMLElement).closest?.('img');
			if (img && node.contains(img) && !img.closest('a, button')) {
				e.preventDefault();
				open(e);
			}
		}
	}
	function markFocusable() {
		for (const img of node.querySelectorAll('img')) {
			if (!img.closest('a, button') && !img.hasAttribute('tabindex')) {
				img.setAttribute('tabindex', '0');
				img.setAttribute('role', 'button');
			}
		}
	}
	node.addEventListener('click', open);
	node.addEventListener('keydown', onKeydown);
	node.classList.add('lightbox-scope');
	markFocusable();
	// Content hydration actions inject imgs asynchronously — keep tabindex
	// attributes current as the subtree mutates.
	const observer = new MutationObserver(markFocusable);
	observer.observe(node, { childList: true, subtree: true });
	return {
		destroy() {
			node.removeEventListener('click', open);
			node.removeEventListener('keydown', onKeydown);
			observer.disconnect();
			node.classList.remove('lightbox-scope');
		}
	};
};
