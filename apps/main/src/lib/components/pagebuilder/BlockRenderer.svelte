<script lang="ts">
	import HeroBlock from './blocks/HeroBlock.svelte';
	import RichTextBlock from './blocks/RichTextBlock.svelte';
	import CtaButtonBlock from './blocks/CtaButtonBlock.svelte';
	import StatsBlock from './blocks/StatsBlock.svelte';
	import SpacerDividerBlock from './blocks/SpacerDividerBlock.svelte';
	import DynamicListingBlock from './blocks/DynamicListingBlock.svelte';
	import ImageGalleryBlock from './blocks/ImageGalleryBlock.svelte';
	import SliderBlock from './blocks/SliderBlock.svelte';
	import FaqBlock from './blocks/FaqBlock.svelte';
	import TestimonialBlock from './blocks/TestimonialBlock.svelte';
	import TableBlock from './blocks/TableBlock.svelte';
	import TocBlock from './blocks/TocBlock.svelte';
	import CountdownBlock from './blocks/CountdownBlock.svelte';
	import ComparisonEmbedBlock from './blocks/ComparisonEmbedBlock.svelte';
	import FormEmbedBlock from './blocks/FormEmbedBlock.svelte';
	import AdvertisementBlock from './blocks/AdvertisementBlock.svelte';
	import type { LandingBlock } from './types';
	// Self-import for recursive grid_row rendering (Svelte 5 pattern).
	import BlockRenderer from './BlockRenderer.svelte';

	interface Props {
		block: LandingBlock;
	}
	let { block }: Props = $props();
</script>

{#if block.type === 'hero'}
	<HeroBlock props={block.props as any} />
{:else if block.type === 'rich_text'}
	<RichTextBlock props={block.props as any} />
{:else if block.type === 'cta' || block.type === 'cta_button'}
	<CtaButtonBlock props={block.props as any} />
{:else if block.type === 'stats'}
	<StatsBlock props={block.props as any} />
{:else if block.type === 'spacer' || block.type === 'spacer_divider'}
	<SpacerDividerBlock props={block.props as any} />
{:else if block.type === 'dynamic_listing'}
	<DynamicListingBlock props={block.props as any} />
{:else if block.type === 'image_gallery' || block.type === 'gallery'}
	<ImageGalleryBlock props={block.props as any} />
{:else if block.type === 'slider'}
	<SliderBlock props={block.props as any} />
{:else if block.type === 'faq' || block.type === 'faq_accordion'}
	<FaqBlock props={block.props as any} />
{:else if block.type === 'testimonial' || block.type === 'testimonial_quote'}
	<TestimonialBlock props={block.props as any} />
{:else if block.type === 'table'}
	<TableBlock props={block.props as any} />
{:else if block.type === 'toc' || block.type === 'table_of_contents'}
	<TocBlock props={block.props as any} />
{:else if block.type === 'countdown'}
	<CountdownBlock props={block.props as any} />
{:else if block.type === 'comparison_embed'}
	<ComparisonEmbedBlock props={block.props as any} />
{:else if block.type === 'form_embed'}
	<FormEmbedBlock props={block.props as any} />
{:else if block.type === 'advertisement' || block.type === 'ad'}
	<AdvertisementBlock props={block.props as any} />
{:else if block.type === 'grid_row'}
	<div class="grid gap-4" style={`grid-template-columns: repeat(${(block.props as any)?.columns ?? 2}, 1fr)`}>
		{#each (block.props as any)?.children ?? [] as child (child.id ?? crypto.randomUUID())}
			<BlockRenderer block={child} />
		{/each}
	</div>
{:else}
	<!-- Unknown block type — render nothing rather than crashing. -->
{/if}
