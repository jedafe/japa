<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Route, ArrowLeft, ExternalLink, DollarSign, Map, RefreshCw, AlertTriangle, ClipboardCheck, GitCompare } from 'lucide-svelte';
	import type { PageData } from './$types';

	/**
	 * Visa Route detail page — Section 90, ADR-098.
	 *
	 * Ref: japaconnect-engineering/sections/90-visa-route-comparison.md
	 */
	let { data }: { data: PageData } = $props();

	const route = $derived(data.route!);

	const title = $derived(`${route.name} — ${t('visa_routes.title', 'Visa Route Comparison')}`);
	const description = $derived(route.eligibility_summary ?? t('visa_routes.subtitle', 'Compare visa routes side by side.'));

	function formatFee(fee: number | null | undefined): string {
		if (fee == null) return '—';
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: 'USD',
			maximumFractionDigits: 0
		}).format(fee);
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/tools/visa-routes"
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('visa_routes.back', 'Back to visa routes')}
		</a>
	</div>

	<div class="p-4 space-y-4">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<div class="flex items-start gap-3">
				<div class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
					<Route size={20} />
				</div>
				<div class="min-w-0 flex-1">
					<h1 class="text-lg font-semibold text-[var(--color-text)]">{route.name}</h1>
					<span class="inline-flex items-center rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
						{route.visa_category}
					</span>
				</div>
			</div>

			{#if route.eligibility_summary}
				<p class="mt-4 text-sm text-[var(--color-text-subtle)] whitespace-pre-line">{route.eligibility_summary}</p>
			{/if}
		</div>

		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 space-y-3">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('visa_routes.overview', 'Route overview')}</h2>
			<div class="grid gap-3 sm:grid-cols-2">
				<div class="flex items-start gap-2">
					<DollarSign size={16} class="mt-0.5 text-[var(--color-muted)]" />
					<div>
						<span class="text-xs text-[var(--color-muted)]">{t('visa_routes.fee', 'Typical fee')}</span>
						<p class="text-sm text-[var(--color-text)]">{formatFee(route.typical_fee_usd)}</p>
					</div>
				</div>
				<div class="flex items-start gap-2">
					<Map size={16} class="mt-0.5 text-[var(--color-muted)]" />
					<div>
						<span class="text-xs text-[var(--color-muted)]">{t('visa_routes.path_to_pr', 'Path to PR')}</span>
						<p class="text-sm text-[var(--color-text)]">{route.path_to_pr || '—'}</p>
					</div>
				</div>
				<div class="flex items-start gap-2">
					<RefreshCw size={16} class="mt-0.5 text-[var(--color-muted)]" />
					<div>
						<span class="text-xs text-[var(--color-muted)]">{t('visa_routes.renewal_terms', 'Renewal terms')}</span>
						<p class="text-sm text-[var(--color-text)]">{route.renewal_terms || '—'}</p>
					</div>
				</div>
				<div class="flex items-start gap-2">
					<AlertTriangle size={16} class="mt-0.5 text-[var(--color-muted)]" />
					<div>
						<span class="text-xs text-[var(--color-muted)]">{t('visa_routes.restrictions', 'Restrictions')}</span>
						<p class="text-sm text-[var(--color-text)]">{route.restrictions || '—'}</p>
					</div>
				</div>
			</div>
		</div>

		<div class="flex flex-wrap items-center gap-3">
			{#if route.gov_source_url}
				<a
					href={route.gov_source_url}
					target="_blank"
					rel="noopener noreferrer"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90"
				>
					{t('visa_routes.view_source', 'View official source')}
					<ExternalLink size={14} />
				</a>
			{/if}
			<a
				href="/compare?type=visa-types&preselect={encodeURIComponent(route.slug)}"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
			>
				<GitCompare size={16} />
				{t('visa_routes.compare', 'Compare this route')}
			</a>
			<a
				href="/tools/visa-checklist"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
			>
				<ClipboardCheck size={16} />
				{t('visa_routes.to_checklist', 'Document checklist')}
			</a>
		</div>
	</div>
</div>
