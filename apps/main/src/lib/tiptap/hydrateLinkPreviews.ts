import { mount, unmount } from 'svelte';
import { LinkPreviewCard, type LinkPreviewData } from '@japaconnect/ui';
import { api } from '$lib/api/client';
import { t } from '$lib/i18n';

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

	async function fetchPreview(url: string): Promise<LinkPreviewData | null> {
		const result = await api.get<LinkPreviewData>(
			`/link-preview?url=${encodeURIComponent(url)}`
		);
		return result.data ?? null;
	}

	function scan() {
		const placeholders = node.querySelectorAll<HTMLElement>(
			'div[data-type="link-preview"]:not([data-hydrated])'
		);
		for (const el of placeholders) {
			const url = el.dataset.url;
			if (!url) continue;
			// Never hydrate dangerous/protocol-relative targets — the card
			// would request them from the preview API. Leave the sanitized
			// placeholder unlinked instead.
			const trimmed = url.trim().toLowerCase();
			if (trimmed.startsWith('//') || /^(javascript|data|vbscript):/.test(trimmed)) continue;
			el.setAttribute('data-hydrated', 'true');
			el.className = 'link-preview-mount';
			const comp = mount(LinkPreviewCard, {
				target: el,
				props: { url, fetchPreview, t }
			});
			mounted.set(el, comp as Record<string, unknown>);
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
