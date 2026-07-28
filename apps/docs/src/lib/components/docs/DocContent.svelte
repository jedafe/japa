<script lang="ts">
	import { onMount } from 'svelte';
	import HelpfulWidget from './HelpfulWidget.svelte';
	import type { TocHeading } from '$lib/tiptap/serialize';

	interface Props {
		/** Server-rendered doc HTML (TipTap JSON → HTML). */
		html: string;
		lastReviewedAt?: string | null;
		lastReviewedBy?: string | null;
	}

	let { html, lastReviewedAt = null, lastReviewedBy = null }: Props = $props();

	let contentEl: HTMLDivElement | undefined = undefined;

	/**
	 * Post-render: enhance <pre><code> blocks inside the rendered prose with the
	 * CodeBlock chrome (language badge + copy button). We do this by reading the
	 * server-rendered HTML and replacing bare <pre> blocks. Highlighting is a
	 * dynamic import, browser-guarded (ADR-060).
	 */
	onMount(async () => {
		if (!contentEl) return;
		// Highlight code blocks via dynamic import (browser-only).
		try {
			const mod = await import('./highlightCodeblocks.svelte');
			mod.enhanceCodeblocks(contentEl);
		} catch {
			// Highlighting is progressive enhancement — never block reading.
		}
	});
</script>

<div class="doc-content" bind:this={contentEl}>
	<!-- eslint-disable-next-line svelte/no-at-html-tags — server-rendered from trusted TipTap JSON -->
	{@html html}
	<HelpfulWidget {lastReviewedAt} {lastReviewedBy} />
</div>

<style>
	.doc-content {
		max-width: 760px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
</style>
