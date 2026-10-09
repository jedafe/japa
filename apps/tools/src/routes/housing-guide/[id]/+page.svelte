<script lang="ts">
	import { untrack } from 'svelte';
	import { buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import type { HousingGuideWithAi } from '$lib/types';

	/**
	 * Housing guide detail page — §96, ADR-104.
	 * Renders the available guide fields: location, accommodation type, rent
	 * ranges, search platforms, landlord requirements, scam categories, tips,
	 * reviewer, and AI/HITL metadata.
	 */
	let { data } = $props();

	let guide = $state<HousingGuideWithAi>(untrack(() => data.guide));

	const title = $derived(
		[guide.city_space_name, guide.destination_space_name].filter(Boolean).join(', ') ||
			t('housing_guide.title', 'Housing Guide')
	);

	const seo = $derived(
		buildSeoMeta({
			title: `${title} — ${t('housing_guide.seo_title', 'Housing Guide — JapaConnect Tools')}`,
			description:
				guide.tips ??
				guide.landlord_requirements ??
				t(
					'housing_guide.seo_description',
					'Destination-specific housing guides — platforms, landlord requirements, rent ranges, scam categories, and tips.'
				),
			path: `/housing-guide/${page.params.id}`,
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-housing-guide.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'HowTo',
				name: title,
				description: guide.tips ?? guide.landlord_requirements ?? '',
				step: (guide.search_platforms && Array.isArray(guide.search_platforms) ? guide.search_platforms : []).slice(0, 10).map((p, i) => ({
					'@type': 'HowToStep',
					position: i + 1,
					name: String(p),
					text: String(p)
				}))
			}
		})
	);

	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	function isPlatformArray(
		platforms: string[] | Record<string, unknown> | null | undefined
	): platforms is string[] {
		return Array.isArray(platforms);
	}

	function objectEntries(val: Record<string, unknown> | null | undefined): [string, unknown][] {
		if (!val || typeof val !== 'object') return [];
		return Object.entries(val);
	}

	function formatValue(val: unknown): string {
		if (val === null || val === undefined) return '';
		if (typeof val === 'string') return val;
		if (typeof val === 'number' || typeof val === 'boolean') return String(val);
		try {
			return JSON.stringify(val);
		} catch {
			return String(val);
		}
	}
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/housing-guide"
		class="inline-flex items-center gap-1.5 text-sm text-muted hover:text-text transition-colors mb-6"
	>
		<Icon name="arrow-left" size={16} />
		{t('housing_guide.back_to_list', 'Back to housing guide')}
	</a>

	<div class="p-6 bg-surface border border-border rounded-lg space-y-4">
		<div>
			<h1 class="text-2xl font-bold text-text">{title}</h1>
			<div class="flex flex-wrap gap-2 mt-2">
				{#if guide.accommodation_type}
					<span class="text-xs px-2 py-0.5 bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] rounded-full font-medium">
						{guide.accommodation_type}
					</span>
				{/if}
			</div>
			{#if guide.last_verified_at}
				<p class="text-xs text-muted mt-2">
					{t('housing_guide.last_verified', 'Last verified:')}
					{formatDate(guide.last_verified_at)}
				</p>
			{/if}
		</div>
	</div>

	<div class="mt-6 p-6 bg-surface border border-border rounded-lg space-y-6">
		{#if guide.landlord_requirements}
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
					{t('housing_guide.landlord_requirements', 'Landlord requirements')}
				</h2>
				<p class="text-sm text-text-subtle">{guide.landlord_requirements}</p>
			</div>
		{/if}

		{#if guide.search_platforms}
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
					{t('housing_guide.search_platforms', 'Search platforms')}
				</h2>
				{#if isPlatformArray(guide.search_platforms)}
					<ul class="space-y-1">
						{#each guide.search_platforms as platform (platform)}
							<li class="flex items-center gap-2 text-sm text-text">
								<Icon name="globe" size={14} class="text-muted flex-shrink-0" />
								{platform}
							</li>
						{/each}
					</ul>
				{:else}
					<dl class="space-y-1">
						{#each objectEntries(guide.search_platforms) as [key, val] (key)}
							<div class="flex items-start gap-2 text-sm">
								<dt class="font-medium text-text flex-shrink-0">{key}:</dt>
								<dd class="text-text-subtle">{formatValue(val)}</dd>
							</div>
						{/each}
					</dl>
				{/if}
			</div>
		{/if}

		{#if guide.rent_ranges}
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
					{t('housing_guide.rent_ranges', 'Rent ranges')}
				</h2>
				<dl class="space-y-1">
					{#each objectEntries(guide.rent_ranges) as [key, val] (key)}
						<div class="flex items-start gap-2 text-sm">
							<dt class="font-medium text-text flex-shrink-0">{key}:</dt>
							<dd class="text-text-subtle">{formatValue(val)}</dd>
						</div>
					{/each}
				</dl>
			</div>
		{/if}

		{#if guide.scam_categories && guide.scam_categories.length > 0}
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
					{t('housing_guide.scam_categories', 'Scam categories')}
				</h2>
				<ul class="space-y-1">
					{#each guide.scam_categories as scam (scam)}
						<li class="flex items-start gap-2 text-sm text-text">
							<Icon name="alert-triangle" size={14} class="text-yellow-600 flex-shrink-0 mt-0.5" />
							{scam}
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if guide.tips}
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
					{t('housing_guide.tips', 'Tips')}
				</h2>
				<p class="text-sm text-text-subtle">{guide.tips}</p>
			</div>
		{/if}

		{#if guide.reviewer_name}
			<p class="text-xs text-muted pt-2 border-t border-border">
				{t('housing_guide.reviewed_by', 'Reviewed by')} {guide.reviewer_name}
			</p>
		{/if}
	</div>

	{#if guide.ai_generated}
		<div class="mt-6 p-4 bg-amber-500/5 border border-amber-500/20 rounded-lg flex items-start gap-2">
			<Icon name="alert-triangle" size={14} class="text-amber-600 flex-shrink-0 mt-0.5" />
			<div class="text-xs text-text leading-relaxed">
				<p>
					{t('common.ai_generated', 'AI-generated content')}
					{#if guide.ai_model}({guide.ai_model}){/if}
				</p>
				{#if guide.hitl_reviewed_by}
					<p class="mt-1 text-muted">
						{t('common.reviewed_by', 'Reviewed by')} {guide.hitl_reviewed_by}
						{#if guide.hitl_reviewed_at}
							· {formatDate(guide.hitl_reviewed_at)}
						{/if}
					</p>
				{:else}
					<p class="mt-1 text-amber-600">
						{t(
							'common.hitl_pending',
							'This content has not been human-reviewed yet. Verify with official sources.'
						)}
					</p>
				{/if}
			</div>
		</div>
	{/if}
</div>
