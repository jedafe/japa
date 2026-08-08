import { mount, unmount } from 'svelte';
import LinkPreviewCard from '$lib/components/create/editors/LinkPreviewCard.svelte';

/**
 * Svelte action that scans a container for `data-type="link-preview"` divs
 * and mounts a LinkPreviewCard Svelte component into each one. Used on view
 * pages (blog, content, forum) to hydrate the placeholder divs emitted by
 * the TipTap serializer.
 *
 * Ref: japaconnect-engineering/sections/15-block-editor-tiptap.md
 *      japaconnect-engineering/roadmaps/ROADMAP-MAIN.md (task 07.09)
 *
 * Usage:
 *   <div class="prose" use:hydrateLinkPreviews>{@html body}</div>
 */
export function hydrateLinkPreviews(node: HTMLElement) {
	const mounted = new Map<HTMLElement, Record<string, unknown>>();

	function scan() {
		const placeholders = node.querySelectorAll<HTMLElement>(
			'div[data-type="link-preview"]:not([data-hydrated])'
		);
		for (const el of placeholders) {
			const url = el.dataset.url;
			if (!url) continue;
			el.setAttribute('data-hydrated', 'true');
			el.className = 'link-preview-mount';
			const comp = mount(LinkPreviewCard, {
				target: el,
				props: { url },
			});
			mounted.set(el, comp);
		}
	}

	// Scan on init and on DOM mutations (in case content is swapped).
	scan();

	const observer = new MutationObserver(() => scan());
	observer.observe(node, { childList: true, subtree: true });

	return {
		destroy() {
			observer.disconnect();
			for (const [, comp] of mounted) {
				unmount(comp);
			}
			mounted.clear();
		},
	};
}
