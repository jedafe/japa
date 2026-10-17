<script lang="ts">
	import { untrack } from 'svelte';
	import { buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t, siteName} from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import type { VisaRouteWithAi } from '$lib/types';

	/**
	 * Visa route detail page — §90, ADR-104.
	 * Renders route name, destination, requirements, typical fee, path to PR,
	 * renewal terms, restrictions, official source, AI summary, and HITL metadata.
	 */
	let { data } = $props();

	let route = $state<VisaRouteWithAi>(untrack(() => data.route));

	const seo = $derived(
		buildSeoMeta({
			title: `${route.name} — ${t('visa_routes.seo_title', 'Visa Route Comparison — {site} Tools')}`,
			description:
				route.eligibility_summary ??
				t(
					'visa_routes.seo_description',
					'Compare visa routes for your destination side by side — eligibility, fees, path to PR, and renewal terms.'
				),
			path: `/visa-routes/${page.params.id}`,
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-visa-routes.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'Article',
				headline: route.name,
				description: route.eligibility_summary ?? '',
				author: { '@type': 'Organization', name: siteName() },
				mainEntityOfPage: `${page.url.origin}/visa-routes/${page.params.id}`
			}
		})
	);

	function formatFee(fee: number | null | undefined): string {
		if (fee == null) return '—';
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: 'USD',
			maximumFractionDigits: 0
		}).format(fee);
	}

	function orDash(value: string | null | undefined): string {
		return value && value.trim() ? value : '—';
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
</script>

<SeoHead seo={seo} ogType="article" twitterCard="summary" />

<div class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/visa-routes"
		class="inline-flex items-center gap-1.5 text-sm text-muted hover:text-text transition-colors mb-6"
	>
		<Icon name="arrow-left" size={16} />
		{t('visa_routes.back_to_list', 'Back to visa routes')}
	</a>

	<div class="p-6 bg-surface border border-border rounded-lg space-y-4">
		<div>
			<h1 class="text-2xl font-bold text-text">{route.name}</h1>
			<p class="text-sm text-text-subtle mt-1">
				{route.visa_category}
				{#if route.destination_space_name}
					· {route.destination_space_name}
				{/if}
			</p>
		</div>
	</div>

	{#if route.eligibility_summary}
		<div class="mt-6 p-6 bg-surface border border-border rounded-lg">
			<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
				{t('visa_routes.eligibility_summary', 'AI summary')}
			</h2>
			<p class="text-sm text-text-subtle leading-relaxed">{route.eligibility_summary}</p>
		</div>
	{/if}

	<div class="mt-6 p-6 bg-surface border border-border rounded-lg space-y-4">
		{#if route.requirements && typeof route.requirements === 'object' && Object.keys(route.requirements).length > 0}
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-2">
					{t('visa_routes.requirements', 'Requirements')}
				</h2>
				<dl class="space-y-1">
					{#each objectEntries(route.requirements) as [key, val] (key)}
						<div class="flex items-start gap-2 text-sm">
							<dt class="font-medium text-text flex-shrink-0">{key}:</dt>
							<dd class="text-text-subtle">{formatValue(val)}</dd>
						</div>
					{/each}
				</dl>
			</div>
		{/if}

		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-1">
					{t('visa_routes.fee', 'Typical fee')}
				</h2>
				<p class="text-sm text-text">{formatFee(route.typical_fee_usd)}</p>
			</div>

			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-1">
					{t('visa_routes.path_to_pr', 'Path to PR')}
				</h2>
				<p class="text-sm text-text">{orDash(route.path_to_pr)}</p>
			</div>

			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-1">
					{t('visa_routes.renewal_terms', 'Renewal terms')}
				</h2>
				<p class="text-sm text-text">{orDash(route.renewal_terms)}</p>
			</div>

			<div>
				<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-1">
					{t('visa_routes.restrictions', 'Restrictions')}
				</h2>
				<p class="text-sm text-text">{orDash(route.restrictions)}</p>
			</div>
		</div>

		{#if route.gov_source_url}
			<div class="pt-3 border-t border-border">
				<a
					href={route.gov_source_url}
					target="_blank"
					rel="noopener noreferrer"
					class="inline-flex items-center gap-1.5 text-sm text-[var(--tool-accent)] hover:underline"
				>
					<Icon name="external-link" size={14} />
					{t('visa_routes.view_source', 'View official source')}
				</a>
			</div>
		{/if}
	</div>

	{#if route.ai_generated}
		<div class="mt-6 p-4 bg-amber-500/5 border border-amber-500/20 rounded-lg flex items-start gap-2">
			<Icon name="alert-triangle" size={14} class="text-amber-600 flex-shrink-0 mt-0.5" />
			<div class="text-xs text-text leading-relaxed">
				<p>
					{t('common.ai_generated', 'AI-generated content')}
					{#if route.ai_model}({route.ai_model}){/if}
				</p>
				{#if route.hitl_reviewed_by}
					<p class="mt-1 text-muted">
						{t('common.reviewed_by', 'Reviewed by')} {route.hitl_reviewed_by}
						{#if route.hitl_reviewed_at}
							· {formatDate(route.hitl_reviewed_at)}
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
