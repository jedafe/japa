<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { toolOgCard } from '#lib/seo/og.js';
	import { t, i18n } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { Home, ArrowLeft, Globe, AlertTriangle, ShieldCheck, MapPin, BookOpen } from 'lucide-svelte';
	import {
		housingPlatforms,
		housingRentTiers,
		rentTierLabel,
		isStaleVerification
	} from '#lib/utils/housing.js';
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

	const platforms = $derived(housingPlatforms(guide.search_platforms));
	const rentTiers = $derived(housingRentTiers(guide.rent_ranges));
	const stale = $derived(isStaleVerification(guide.last_verified_at));

	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'long', day: 'numeric' });
		} catch {
			return iso;
		}
	}
</script>

<SeoHead title={title} description={description} ogTitle={title} robots="noindex,nofollow"
	ogImage={toolOgCard(page.url.origin, page.url.pathname)}
/>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/tools/housing-guide"
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft class="rtl:-scale-x-100" size={16} />
			{t('housing_guide.back', 'Back to housing guide')}
		</a>
	</div>

	<div class="p-4 space-y-4">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<div class="flex items-start gap-3">
				<div class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]">
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
							{#if stale}
								<span class="inline-flex items-center gap-1 rounded-full bg-yellow-500/15 px-2 py-0.5 text-xs font-medium text-yellow-700 dark:text-yellow-400">
									<AlertTriangle size={12} />
									{t('housing_guide.stale_warning', 'May be outdated — not verified in 90+ days')}
								</span>
							{/if}
						{:else}
							<span class="text-xs text-[var(--color-muted)]">
								{t('housing_guide.not_verified', 'Not yet verified')}
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

		<!-- §96 [{name, url, description}] -->
		{#if platforms.length > 0}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('housing_guide.search_platforms', 'Search platforms')}</h2>
				<ul class="space-y-2">
					{#each platforms as platform (platform.name + (platform.url ?? ''))}
						<li class="text-sm">
							<span class="flex items-center gap-2 text-[var(--color-text)]">
								<Globe size={14} class="text-[var(--color-muted)]" />
								{#if platform.url}
									<a
										href={platform.url}
										target="_blank"
										rel="noopener noreferrer"
										class="font-medium text-[var(--color-primary-text)] hover:underline"
									>
										{platform.name}
									</a>
								{:else}
									<span class="font-medium">{platform.name}</span>
								{/if}
							</span>
							{#if platform.description}
								<p class="ms-6 text-xs text-[var(--color-muted)]">{platform.description}</p>
							{/if}
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		<!-- §96 {tier: {studio, 1bed, 2bed, currency}} -->
		{#if rentTiers.length > 0}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('housing_guide.rent_ranges', 'Rent ranges')}</h2>
				<div class="space-y-3">
					{#each rentTiers as row (row.tier)}
						<div class="text-sm">
							<p class="font-medium text-[var(--color-text)] capitalize">
								{rentTierLabel(row.tier)}
								{#if row.currency}
									<span class="ms-1 text-xs font-normal text-[var(--color-muted)]">({row.currency})</span>
								{/if}
							</p>
							{#if row.raw}
								<p class="text-[var(--color-text-subtle)]">{row.raw}</p>
							{:else}
								<dl class="mt-0.5 space-y-0.5">
									{#if row.studio}
										<div class="flex gap-2"><dt class="text-[var(--color-muted)]">{t('housing_guide.unit_studio', 'Studio')}:</dt><dd class="text-[var(--color-text-subtle)]">{row.studio}</dd></div>
									{/if}
									{#if row.oneBed}
										<div class="flex gap-2"><dt class="text-[var(--color-muted)]">{t('housing_guide.unit_1bed', '1-bed')}:</dt><dd class="text-[var(--color-text-subtle)]">{row.oneBed}</dd></div>
									{/if}
									{#if row.twoBed}
										<div class="flex gap-2"><dt class="text-[var(--color-muted)]">{t('housing_guide.unit_2bed', '2-bed')}:</dt><dd class="text-[var(--color-text-subtle)]">{row.twoBed}</dd></div>
									{/if}
								</dl>
							{/if}
						</div>
					{/each}
				</div>
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
				{#if page.data.features?.['scam_alert.enabled']}
					<a
						href="/scam-alerts?scam_type=housing_fraud&dest={guide.destination_space_id}"
						class="mt-3 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary-text)] hover:underline"
					>
						{t('scam_alerts.see_alerts', 'See housing scam alerts for this destination')} →
					</a>
				{/if}
			</div>
		{/if}

		{#if guide.tips}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('housing_guide.tips', 'Tips')}</h2>
				<p class="text-sm text-[var(--color-text-subtle)] whitespace-pre-line">{guide.tips}</p>
			</div>
		{/if}

		<!-- §96 editorial cross-link — the in-depth guide that claims this
		     row (guide_slug on the detail DTO). -->
		{#if guide.guide_slug}
			<a
				href="/guide/{guide.guide_slug}"
				class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-opacity hover:opacity-90"
			>
				<BookOpen size={18} />
				{t('housing_guide.read_guide', 'Read the in-depth guide')}
			</a>
		{/if}

		{#if guide.reviewer_name}
			<p class="text-xs text-[var(--color-muted)]">
				{t('housing_guide.reviewed_by', 'Reviewed by')} {guide.reviewer_name}
			</p>
		{/if}
	</div>
</div>
