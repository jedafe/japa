import { mount, unmount } from 'svelte';
import ContentEmbedCard from '$lib/components/content/ContentEmbedCard.svelte';
import type { ListEmbedSnapshot } from '$lib/types';

/**
 * Svelte action that scans a container for
 * `div[data-type="content-embed"][data-content-slug]` placeholders and mounts
 * a ContentEmbedCard for each one. The placeholders are emitted by the list
 * composer serializer (listSerialize.ts) for internal-content embeds; the
 * static `<a class="ce-card">` snapshot they contain is parsed into the
 * card's `initial` prop so something meaningful renders while the live
 * GET /content/{slug} fetch is in flight — and if the target was since
 * unpublished or deleted.
 *
 * Ref: docs/superpowers/specs/2026-09-28-list-content-type-design.md
 *      .jeos/sections/14-content-type-system.md (List amendment)
 *
 * Usage:
 *   <div class="prose" use:hydrateContentEmbeds>{@html body}</div>
 */
export function hydrateContentEmbeds(node: HTMLElement) {
	const mounted = new Map<HTMLElement, Record<string, unknown>>();

	/** Recover the serialized snapshot card (`<a class="ce-card">`) into a
	 *  ListEmbedSnapshot prop. Undefined when the placeholder is bare. */
	function readSnapshot(el: HTMLElement): ListEmbedSnapshot | undefined {
		const card = el.querySelector('a.ce-card');
		if (!card) return undefined;
		const title = card.querySelector('.ce-title')?.textContent?.trim() || undefined;
		const excerpt = card.querySelector('.ce-excerpt')?.textContent?.trim() || undefined;
		const image_url = card.querySelector('img')?.getAttribute('src') ?? undefined;
		if (!title && !excerpt && !image_url) return undefined;
		return { title, excerpt, image_url };
	}

	function hydrate(el: HTMLElement) {
		const slug = el.dataset.contentSlug;
		if (!slug || el.hasAttribute('data-hydrated')) return;

		el.setAttribute('data-hydrated', 'true');
		const initial = readSnapshot(el);
		// Replace the static snapshot markup — the card re-renders it as its
		// loading/error fallback so we never show both copies.
		el.replaceChildren();
		const comp = mount(ContentEmbedCard, {
			target: el,
			props: { slug, initial }
		});
		mounted.set(el, comp);
	}

	function scan() {
		const placeholders = node.querySelectorAll<HTMLElement>(
			'div[data-type="content-embed"][data-content-slug]:not([data-hydrated])'
		);
		for (const el of placeholders) {
			hydrate(el);
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
		}
	};
}
