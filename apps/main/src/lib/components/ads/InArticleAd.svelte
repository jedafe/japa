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

	type Segment = { type: 'html'; frag: string } | { type: 'ad' };

	const BLOCK_END = /<\/(p|h2|h3|h4|figure|blockquote|ul|ol)>/gi;

	// Paragraph = a full block element. Positions are block indexes.
	function countBlocks(s: string): number {
		return (s.match(BLOCK_END) ?? []).length;
	}

	// Split html into segments; an { type:'ad' } marker is inserted after
	// block `after` and then every `every` blocks (0 = once).
	function buildSegments(s: string, after: number, every: number): Segment[] {
		const segs: Segment[] = [];
		let blockIdx = 0;
		let adsInserted = 0;
		let pos = 0;
		const maxAds = 2; // slot max_display_per_page=2 — never exceed.

		for (const m of s.matchAll(BLOCK_END)) {
			blockIdx++;
			const boundary = (m.index ?? 0) + m[0].length;
			const shouldAd =
				adsInserted < maxAds &&
				blockIdx === after + (adsInserted === 0 ? 0 : adsInserted * every);
			if (shouldAd) {
				segs.push({ type: 'html', frag: s.slice(pos, boundary) });
				segs.push({ type: 'ad' });
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
		countBlocks(html) >= firstAt + 1 ? buildSegments(html, firstAt, everyN) : [{ type: 'html', frag: html }]
	);
</script>

{#each segments as seg, i (i)}
	{#if seg.type === 'html'}
		{@html seg.frag}
	{:else}
		<div class="not-prose my-6 flex justify-center">
			<AdSlot {slot} format="inline" {spaceId} />
		</div>
	{/if}
{/each}
