<script lang="ts">
	// Ref: japaconnect-engineering/sections/25-ad-placement.md (Amendment — in-article).
	// Splits already-sanitized article HTML on block-element boundaries and
	// injects <AdSlot> units after paragraph N and every M paragraphs.
	// Offsets come from public system settings
	// (ads.in_article_after_paragraph / ads.in_article_every). String-only
	// splitting — SSR-safe, no DOM parsing.
	import { page } from '$app/state';
	import AdSlot from '$lib/components/ui/AdSlot.svelte';

	interface Props {
		/** Sanitized article HTML (run through sanitizeContentHtml upstream). */
		html: string;
		/** Ad slot name; defaults to the seeded article_inline slot. */
		slot?: string;
		/** Space context for targeted campaigns. */
		spaceId?: string;
	}

	let { html, slot = 'article_inline', spaceId = '' }: Props = $props();

	type Segment = { type: 'html'; frag: string } | { type: 'ad'; pick: number };

	const TAG = /<\/?([a-zA-Z][a-zA-Z0-9]*)[^>]*>/g;
	// Elements whose children are not paragraph-boundaries — a </p> inside a
	// <blockquote> or an <li> inside a <ul> must not split the fragment, or
	// the ad would land inside an unclosed element and break the layout.
	const CONTAINERS = new Set([
		'blockquote', 'ul', 'ol', 'dl', 'figure', 'pre', 'table',
		'div', 'section', 'article', 'aside', 'details', 'nav'
	]);
	const LEAF_BLOCKS = new Set(['p', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6']);

	// End offsets of each top-level block element. Depth is tracked so
	// closing tags inside nested containers are not treated as boundaries;
	// a container's own close only counts when it returns to depth 0.
	function blockEnds(s: string): number[] {
		const ends: number[] = [];
		let depth = 0;
		for (const m of s.matchAll(TAG)) {
			const tag = m[1].toLowerCase();
			const text = m[0];
			const end = (m.index ?? 0) + text.length;
			if (!text.startsWith('</')) {
				if (CONTAINERS.has(tag) && !text.endsWith('/>')) depth++;
				continue;
			}
			if (CONTAINERS.has(tag)) {
				if (depth > 0) depth--;
				if (depth === 0) ends.push(end);
			} else if (LEAF_BLOCKS.has(tag) && depth === 0) {
				ends.push(end);
			}
		}
		return ends;
	}

	// Split html into segments; an { type:'ad' } marker is inserted after
	// block `after` and then every `every` blocks (0 = once).
	function buildSegments(s: string, after: number, every: number): Segment[] {
		const segs: Segment[] = [];
		const ends = blockEnds(s);
		let adsInserted = 0;
		let pos = 0;
		const maxAds = 2; // slot max_display_per_page=2 — never exceed.

		for (const [i, boundary] of ends.entries()) {
			const blockIdx = i + 1;
			const shouldAd =
				adsInserted < maxAds &&
				blockIdx === after + (adsInserted === 0 ? 0 : adsInserted * every);
			if (shouldAd) {
				segs.push({ type: 'html', frag: s.slice(pos, boundary) });
				segs.push({ type: 'ad', pick: adsInserted });
				adsInserted++;
				pos = boundary;
				if (every <= 0) break;
			}
		}
		if (pos < s.length) {
			segs.push({ type: 'html', frag: s.slice(pos) });
		}
		return segs;
	}

	const firstAt = $derived(
		Math.max(1, Number(page.data.siteSettings?.['ads.in_article_after_paragraph'] ?? 3) || 3)
	);
	const everyN = $derived(
		Math.max(0, Number(page.data.siteSettings?.['ads.in_article_every'] ?? 6) || 0)
	);

	const segments = $derived<Segment[]>(
		blockEnds(html).length >= firstAt + 1 ? buildSegments(html, firstAt, everyN) : [{ type: 'html', frag: html }]
	);
</script>

{#each segments as seg, i (i)}
	{#if seg.type === 'html'}
		{@html seg.frag}
	{:else}
		<div class="not-prose my-6 flex justify-center">
			<AdSlot {slot} format="inline" {spaceId} pickIndex={seg.pick} />
		</div>
	{/if}
{/each}
