import { mount, unmount } from 'svelte';
import { getVideoSource } from '$lib/api/videos';
import VideoEmbed from '$lib/components/content/VideoEmbed.svelte';
import type { VideoSource } from '$lib/types';

/**
 * Svelte action that scans a container for `.video-embed[data-video-source-id]`
 * placeholders and mounts a VideoEmbed component for each one. Used on view pages
 * (blog, story, content, forum) so video nodes created with the TipTap editor
 * render the correct player for the linked video_sources row.
 *
 * Ref: japaconnect-engineering/sections/106-video-content-and-import.md
 */
export function hydrateVideoEmbeds(node: HTMLElement) {
	const mounted = new Map<HTMLElement, Record<string, unknown>>();
	const inFlight = new Set<HTMLElement>();

	async function hydrate(el: HTMLElement) {
		const id = el.dataset.videoSourceId;
		if (!id || inFlight.has(el) || el.hasAttribute('data-hydrated')) return;

		inFlight.add(el);
		try {
			const res = await getVideoSource(id);
			if (!res.data) return;

			el.setAttribute('data-hydrated', 'true');
			const comp = mount(VideoEmbed, {
				target: el,
				props: { source: res.data as VideoSource }
			});
			mounted.set(el, comp);
		} finally {
			inFlight.delete(el);
		}
	}

	function scan() {
		const placeholders = node.querySelectorAll<HTMLElement>('div.video-embed[data-video-source-id]:not([data-hydrated])');
		for (const el of placeholders) {
			hydrate(el);
		}
	}

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
