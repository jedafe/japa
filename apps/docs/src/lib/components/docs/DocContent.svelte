<script lang="ts">
	import HelpfulWidget from './HelpfulWidget.svelte';
	import { sanitizeSidebarHtml } from '$lib/utils/safeHtml';
	import type { TocHeading } from '$lib/tiptap/serialize';

	interface Props {
		/** Page ID for helpful-vote localStorage persistence. */
		pageId: string;
		/** Docs audience (e.g. 'users') — passed to the helpful widget. */
		audience: string;
		/** Docs page slug — passed to the helpful widget. */
		slug: string;
		/** Server-rendered doc HTML (TipTap JSON → HTML). */
		html: string;
		lastReviewedAt?: string | null;
		lastReviewedBy?: string | null;
	}

	let { pageId, audience, slug, html, lastReviewedAt = null, lastReviewedBy = null }: Props = $props();

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
		let cleanup: (() => void) | undefined;
		// Highlight code blocks via dynamic import (browser-only).
		(async () => {
			try {
				const mod = await import('./highlightCodeblocks.svelte');
				if (!cancelled && contentEl) cleanup = mod.enhanceCodeblocks(contentEl);
			} catch {
				// Highlighting is progressive enhancement — never block reading.
			}
		})();
		return () => {
			cancelled = true;
			cleanup?.();
		};
	});
</script>

<div class="doc-content" bind:this={contentEl}>
	<!-- eslint-disable-next-line svelte/no-at-html-tags — server-rendered from trusted TipTap JSON, re-sanitized client-side as defense in depth -->
	{@html sanitizeSidebarHtml(html)}
	<HelpfulWidget {pageId} {audience} {slug} {lastReviewedAt} {lastReviewedBy} />
</div>

<style>
	/* No padding/max-width here — the parent .doc-article column already
	   provides them; nesting another padded column doubled the insets. */
	.doc-content {
		min-width: 0;
	}
</style>
