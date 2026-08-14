<script lang="ts">
	import HelpfulWidget from './HelpfulWidget.svelte';
	import type { TocHeading } from '$lib/tiptap/serialize';

	interface Props {
		/** Page ID for helpful-vote localStorage persistence. */
		pageId: string;
		/** Server-rendered doc HTML (TipTap JSON → HTML). */
		html: string;
		lastReviewedAt?: string | null;
		lastReviewedBy?: string | null;
	}

	let { pageId, html, lastReviewedAt = null, lastReviewedBy = null }: Props = $props();

	let contentEl: HTMLDivElement | undefined = undefined;

	/**
	 * Post-render: enhance <pre><code> blocks inside the rendered prose with the
	 * CodeBlock chrome (language badge + copy button). We do this by reading the
	 * server-rendered HTML and replacing bare <pre> blocks. Highlighting is a
	 * dynamic import, browser-guarded (ADR-060).
	 *
	 * Uses $effect keyed on html so the enhancement re-runs on client-side
	 * navigation (same route, different params) — onMount only runs once per
	 * component instance and would leave new code blocks unenhanced after a
	 * page change.
	 */
	$effect(() => {
		// Track html so the effect re-runs when it changes.
		const _html = html;
		if (!contentEl) return;
		let cancelled = false;
		// Highlight code blocks via dynamic import (browser-only).
		(async () => {
			try {
				const mod = await import('./highlightCodeblocks.svelte');
				if (!cancelled && contentEl) mod.enhanceCodeblocks(contentEl);
			} catch {
				// Highlighting is progressive enhancement — never block reading.
			}
		})();
		return () => { cancelled = true; };
	});
</script>

<div class="doc-content" bind:this={contentEl}>
	<!-- eslint-disable-next-line svelte/no-at-html-tags — server-rendered from trusted TipTap JSON -->
	{@html html}
	<HelpfulWidget {pageId} {lastReviewedAt} {lastReviewedBy} />
</div>

<style>
	.doc-content {
		max-width: 760px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
</style>
