<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Home, ArrowLeft, Globe, AlertTriangle, ShieldCheck, MapPin } from 'lucide-svelte';
	import type { PageData } from './$types';

	/**
	 * Housing Guide detail page — Section 96, ADR-104.
	 *
	 * Ref: japaconnect-engineering/sections/96-housing-guide.md
	 */
	let { data }: { data: PageData } = $props();

	const guide = $derived(data.guide!);

	const title = $derived(`${guide.city_space_name ? `${guide.city_space_name} — ` : ''}${t('housing_guide.title', 'Housing Guide')}`);
	const description = $derived(guide.tips ?? t('housing_guide.subtitle', 'Find safe housing abroad.'));

	function isArray(val: unknown): val is string[] {
		return Array.isArray(val);
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
			return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' });
		} catch {
			return iso;
		}
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
			href="/tools/housing-guide"
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('housing_guide.back', 'Back to housing guide')}
		</a>
	</div>

	<div class="p-4 space-y-4">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<div class="flex items-start gap-3">
				<div class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
					<Home size={20} />
				</div>
				<div class="min-w-0 flex-1">
					<h1 class="text-lg font-semibold text-[var(--color-text)]">
						{guide.city_space_name ?? t('housing_guide.country_level', 'Country guide')}
					</h1>
					<div class="mt-1 flex flex-wrap gap-2">
						{#if guide.accommodation_type}
							<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-muted)] capitalize">
								{guide.accommodation_type}
							</span>
						{/if}
						{#if guide.last_verified_at}
							<span class="text-xs text-[var(--color-muted)]">
								{t('housing_guide.last_verified', 'Last verified')}: {formatDate(guide.last_verified_at)}
							</span>
						{/if}
					</div>
				</div>
			</div>
		</div>

		{#if guide.landlord_requirements}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('housing_guide.landlord_requirements', 'Landlord requirements')}</h2>
				<p class="text-sm text-[var(--color-text-subtle)] whitespace-pre-line">{guide.landlord_requirements}</p>
			</div>
		{/if}

		{#if guide.search_platforms}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('housing_guide.search_platforms', 'Search platforms')}</h2>
				{#if isArray(guide.search_platforms)}
					<ul class="space-y-1">
						{#each guide.search_platforms as platform (platform)}
							<li class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<Globe size={14} class="text-[var(--color-muted)]" />
								{platform}
							</li>
						{/each}
					</ul>
				{:else}
					<dl class="space-y-1">
						{#each objectEntries(guide.search_platforms as Record<string, unknown>) as [key, val] (key)}
							<div class="flex items-start gap-2 text-sm">
								<dt class="font-medium text-[var(--color-text)]">{key}:</dt>
								<dd class="text-[var(--color-text-subtle)]">{formatValue(val)}</dd>
							</div>
						{/each}
					</dl>
				{/if}
			</div>
		{/if}

		{#if guide.rent_ranges}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('housing_guide.rent_ranges', 'Rent ranges')}</h2>
				<dl class="space-y-1">
					{#each objectEntries(guide.rent_ranges as Record<string, unknown>) as [key, val] (key)}
						<div class="flex items-start gap-2 text-sm">
							<dt class="font-medium text-[var(--color-text)] flex-shrink-0">{key}:</dt>
							<dd class="text-[var(--color-text-subtle)]">{formatValue(val)}</dd>
						</div>
					{/each}
				</dl>
			</div>
		{/if}

		{#if guide.scam_categories && guide.scam_categories.length > 0}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
					<AlertTriangle size={16} class="text-yellow-600" />
					{t('housing_guide.scam_categories', 'Scam categories')}
				</h2>
				<ul class="space-y-1">
					{#each guide.scam_categories as scam (scam)}
						<li class="flex items-start gap-2 text-sm text-[var(--color-text)]">
							<ShieldCheck size={14} class="mt-0.5 text-[var(--color-muted)]" />
							{scam}
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if guide.tips}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('housing_guide.tips', 'Tips')}</h2>
				<p class="text-sm text-[var(--color-text-subtle)] whitespace-pre-line">{guide.tips}</p>
			</div>
		{/if}

		{#if guide.reviewer_name}
			<p class="text-xs text-[var(--color-muted)]">
				{t('housing_guide.reviewed_by', 'Reviewed by')} {guide.reviewer_name}
			</p>
		{/if}
	</div>
</div>
