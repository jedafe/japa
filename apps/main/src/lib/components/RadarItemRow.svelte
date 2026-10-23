<script lang="ts">
	/**
	 * RadarItemRow — a reusable row for a single radar feed item (v1:
	 * promoted scholarship). Renders the title, a type-specific summary
	 * (deadline + degree level for scholarships), a trust-tier badge, a
	 * freshness indicator, a "verified" vs "needs review" pill, and a
	 * deep-link to /radar/feed/{id}.
	 *
	 * Ref: Japa Intelligence System spec §7 (RadarItemRow)
	 *      ADR-058 (renders inside the unified app shell)
	 */
	import { t, i18n } from '$lib/i18n';
	import { GraduationCap, ExternalLink, Clock, ShieldCheck, ShieldAlert } from 'lucide-svelte';
	import RelativeTime from '$lib/components/ui/RelativeTime.svelte';
	import type { RadarItem } from '$lib/types';

	// Re-export so existing imports from this module keep working.
	export type { RadarItem };

	interface Props {
		item: RadarItem;
		/** Show the country label (hidden inside country-grouped sections). */
		showCountry?: boolean;
	}

	let { item, showCountry = false }: Props = $props();

	// Trust tier — the backend stores trust_level as an int16 (1–4) where
	// 1=Official (highest), 2=Trusted, 3=Aggregator, 4=Community (lowest).
	// Lower is better. ≤2 is "verified"; >2 is "needs review".
	const trustTier = $derived(item.trust_level ?? 4);
	const isVerified = $derived(trustTier <= 2);
	const trustLabel = $derived(
		trustTier <= 1
			? t('radar.trust_high', 'High trust')
			: trustTier <= 2
				? t('radar.trust_verified', 'Verified')
				: t('radar.trust_review', 'Needs review')
	);
	const trustClass = $derived(
		trustTier <= 2
			? 'border-[var(--color-success)]/40 bg-[var(--color-success)]/10 text-[var(--color-success)]'
			: 'border-[var(--color-warning)]/40 bg-[var(--color-warning)]/10 text-[var(--color-warning)]'
	);

	// Freshness — freshness_score is 0–100; higher is fresher. We render a
	// compact dot + label so the user can scan recency at a glance.
	const freshness = $derived(item.freshness_score ?? 0);
	const freshnessLabel = $derived(
		freshness >= 75
			? t('radar.freshness_fresh', 'Fresh')
			: freshness >= 40
				? t('radar.freshness_stale', 'Stale')
				: t('radar.freshness_old', 'Outdated')
	);
	const freshnessClass = $derived(
		freshness >= 75
			? 'text-[var(--color-success)]'
			: freshness >= 40
				? 'text-[var(--color-warning)]'
				: 'text-[var(--color-muted)]'
	);

	// Deadline formatting — the backend emits a pgtype.Date (YYYY-MM-DD) or
	// null. We render a human-readable date when present.
	const deadline = $derived(item.deadline ? String(item.deadline).slice(0, 10) : '');
	const deadlineLabel = $derived(
		deadline
			? new Date(deadline + 'T00:00:00').toLocaleDateString(i18n.locale, {
					year: 'numeric',
					month: 'short',
					day: 'numeric'
				})
			: ''
	);
</script>

<a
	href={`/radar/feed/${item.id}?item_type=${encodeURIComponent(item.type)}`}
	class="group block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
>
	<div class="flex items-start justify-between gap-3">
		<div class="min-w-0 flex-1">
			<div class="flex items-center gap-2">
				<GraduationCap size={16} class="shrink-0 text-[var(--color-primary)]" />
				<h3 class="truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
					{item.title}
				</h3>
			</div>

			{#if item.description}
				<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{item.description}</p>
			{/if}

			<!-- Type-specific summary: deadline + degree level for scholarships -->
			<div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
				{#if showCountry && item.country}
					<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{item.country}</span>
				{/if}
				{#if item.university}
					<span class="truncate">{item.university}</span>
				{/if}
				{#if item.degree_level}
					<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{item.degree_level}</span>
				{/if}
				{#if deadlineLabel}
					<span class="flex items-center gap-1">
						<Clock size={12} />
						{t('radar.deadline', 'Deadline')}: {deadlineLabel}
					</span>
				{/if}
			</div>
		</div>

		{#if item.application_url}
			<ExternalLink
				size={14}
				class="shrink-0 text-[var(--color-muted)] opacity-0 transition-opacity group-hover:opacity-100 rtl:-scale-x-100"
				aria-hidden="true"
			/>
		{/if}
	</div>

	<!-- Badges row: trust tier + freshness + verified/needs-review pill -->
	<div class="mt-3 flex flex-wrap items-center gap-2">
		<span class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium {trustClass}">
			{#if isVerified}
				<ShieldCheck size={12} />
			{:else}
				<ShieldAlert size={12} />
			{/if}
			{trustLabel}
		</span>

		<span class="inline-flex items-center gap-1 text-xs {freshnessClass}">
			<span class="h-1.5 w-1.5 rounded-full bg-current"></span>
			{freshnessLabel}
		</span>

		{#if item.created_at}
			<span class="text-xs text-[var(--color-muted)]">
				<RelativeTime iso={item.created_at} />
			</span>
		{/if}
	</div>
</a>
