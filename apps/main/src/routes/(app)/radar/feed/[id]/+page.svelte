<script lang="ts">
	// Ref: japaconnect-engineering — Japa Intelligence System spec §7
	// Single radar item detail. Renders inside the unified app shell (ADR-058).
	// All data comes from the SSR load() — no client fetch, no hardcoded data
	// (Rule 16). The "signal strip" is the signature element: a three-segment
	// readout of confidence / trust tier / freshness that encodes the
	// provenance quality the intelligence system exists to surface.
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import {
		ArrowLeft,
		CalendarClock,
		Building2,
		Globe,
		GraduationCap,
		ExternalLink,
		ShieldCheck,
		ShieldAlert,
		Radar,
		Clock,
		Bookmark,
		BookmarkCheck
	} from 'lucide-svelte';
	import type { RadarItem } from '$lib/types';
	import { api } from '$lib/api/client';
	import { invalidate } from '$app/navigation';

	let { data } = $props();
	const item = $derived(data.item as RadarItem);

	// INTEL-017 fix: saved opportunities UI. The backend delegates to the
	// §73 saved_scholarships tracker via POST/DELETE /intel/radar/save.
	let saved = $state(false);
	let saving = $state(false);

	$effect(() => {
		// Seed saved state from SSR if the backend includes it in the item.
		saved = !!item.saved;
	});

	async function toggleSave() {
		if (saving) return;
		saving = true;
		const wasSaved = saved;
		saved = !wasSaved; // optimistic
		let ok = false;
		if (wasSaved) {
			const res = await api.delete(`/intel/radar/save/${item.id}`);
			ok = !res.error;
		} else {
			const res = await api.post(`/intel/radar/save`, { item_id: item.id });
			ok = !res.error;
		}
		if (!ok) saved = wasSaved; // rollback
		else await invalidate(`/api/v1/intel/radar/saved`);
		saving = false;
	}

	// Trust tier 1 = highest (verified official source), 4 = lowest.
	const trustPct = $derived(Math.max(0, Math.min(100, (4 - item.trust_level) * 25 + 25)));
	const confidencePct = $derived(
		Math.max(0, Math.min(100, Math.round(item.confidence_overall * 100)))
	);
	const freshnessPct = $derived(Math.max(0, Math.min(100, item.freshness_score)));

	const verified = $derived(item.trust_level <= 2);

	const deadlineDate = $derived(
		item.deadline ? new Date(item.deadline) : null
	);
	const deadlineValid = $derived(
		deadlineDate !== null && !isNaN(deadlineDate.getTime())
	);
	const daysToDeadline = $derived(
		deadlineValid
			? Math.ceil((deadlineDate!.getTime() - Date.now()) / 86_400_000)
			: null
	);
	const deadlinePassed = $derived(daysToDeadline !== null && daysToDeadline < 0);

	const publishedDate = $derived(
		item.created_at ? new Date(item.created_at) : null
	);
	const publishedValid = $derived(
		publishedDate !== null && !isNaN(publishedDate.getTime())
	);

	// Pull the bare domain out of the source URL for the provenance line.
	const sourceDomain = $derived(extractDomain(item.source_url));
	const applyUrl = $derived(item.application_url ?? item.source_url);

	function extractDomain(url: string): string {
		try {
			return new URL(url).hostname.replace(/^www\./, '');
		} catch {
			return url;
		}
	}

	function formatDate(d: Date): string {
		return d.toLocaleDateString(i18n.locale, {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}
</script>

<svelte:head>
	<title>{item.title} · {t('radar.title')}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto max-w-3xl px-4 py-6">
	<!-- Back link + save button -->
	<div class="flex items-center justify-between">
		<a
			href="/radar"
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={15} />
			{t('radar.back')}
		</a>
		<button
			onclick={toggleSave}
			disabled={saving}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
		>
			{#if saved}
				<BookmarkCheck size={14} class="text-[var(--color-primary)]" />
				{t('radar.detail.saved', 'Saved')}
			{:else}
				<Bookmark size={14} />
				{t('radar.detail.save', 'Save')}
			{/if}
		</button>
	</div>

	<article class="mt-5 overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
		<!-- Header band: type pill + country + verified status -->
		<div class="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-5 py-3">
			<span class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-primary)]/10 px-2.5 py-1 text-xs font-medium capitalize text-[var(--color-primary)]">
				<Radar size={12} />
				{item.type}
			</span>
			<span class="inline-flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
				<Globe size={12} />
				{item.country}
			</span>
			<span class="ml-auto inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium {verified ? 'bg-[var(--color-success)]/10 text-[var(--color-success)]' : 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'}">
				{#if verified}
					<ShieldCheck size={12} />
					{t('radar.detail.verified')}
				{:else}
					<ShieldAlert size={12} />
					{t('radar.detail.needs_review')}
				{/if}
			</span>
		</div>

		<div class="px-5 py-5">
			<!-- Title -->
			<h1 class="text-xl font-medium leading-snug text-[var(--color-text)] sm:text-2xl">
				{item.title}
			</h1>

			<!-- Deadline callout — the most actionable datum for a scholarship -->
			{#if deadlineValid}
				<div
					class="mt-4 flex items-center gap-3 rounded-[var(--radius)] border px-4 py-3 {deadlinePassed
						? 'border-[var(--color-danger)]/30 bg-[var(--color-danger)]/5'
						: 'border-[var(--color-primary)]/30 bg-[var(--color-primary)]/5'}"
				>
					<CalendarClock
						size={18}
						class={deadlinePassed ? 'text-[var(--color-danger)]' : 'text-[var(--color-primary)]'}
					/>
					<div class="min-w-0 flex-1">
						<p class="text-xs text-[var(--color-muted)]">{t('radar.detail.deadline')}</p>
						<p class="text-sm font-medium text-[var(--color-text)]">
							{formatDate(deadlineDate!)}
						</p>
					</div>
					{#if daysToDeadline !== null}
						<span
							class="shrink-0 text-xs font-medium {deadlinePassed
								? 'text-[var(--color-danger)]'
								: 'text-[var(--color-primary)]'}"
						>
							{#if deadlinePassed}
								{t('radar.detail.deadline_passed')}
							{:else}
								{daysToDeadline} {t('radar.detail.days_away')}
							{/if}
						</span>
					{/if}
				</div>
			{:else}
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('radar.detail.no_deadline')}</p>
			{/if}

			<!-- Quick facts grid -->
			<dl class="mt-5 grid grid-cols-1 gap-px overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-border)] sm:grid-cols-2">
				{#if item.university}
					<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
						<Building2 size={15} class="shrink-0 text-[var(--color-muted)]" />
						<div class="min-w-0">
							<dt class="text-xs text-[var(--color-muted)]">{t('radar.detail.university')}</dt>
							<dd class="truncate text-sm text-[var(--color-text)]">{item.university}</dd>
						</div>
					</div>
				{/if}
				{#if item.degree_level}
					<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
						<GraduationCap size={15} class="shrink-0 text-[var(--color-muted)]" />
						<div class="min-w-0">
							<dt class="text-xs text-[var(--color-muted)]">{t('radar.detail.degree')}</dt>
							<dd class="truncate text-sm text-[var(--color-text)]">{item.degree_level}</dd>
						</div>
					</div>
				{/if}
				{#if publishedValid}
					<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
						<Clock size={15} class="shrink-0 text-[var(--color-muted)]" />
						<div class="min-w-0">
							<dt class="text-xs text-[var(--color-muted)]">{t('radar.detail.published')}</dt>
							<dd class="text-sm text-[var(--color-text)]">{formatDate(publishedDate!)}</dd>
						</div>
					</div>
				{/if}
				{#if sourceDomain}
					<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
						<Globe size={15} class="shrink-0 text-[var(--color-muted)]" />
						<div class="min-w-0">
							<dt class="text-xs text-[var(--color-muted)]">{t('radar.detail.source')}</dt>
							<dd class="truncate text-sm text-[var(--color-text)]">{sourceDomain}</dd>
						</div>
					</div>
				{/if}
			</dl>

			<!-- Description -->
			{#if item.description}
				<section class="mt-6">
					<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
						{t('radar.detail.description')}
					</h2>
					<p class="text-sm leading-relaxed text-[var(--color-text)]">{item.description}</p>
				</section>
			{/if}

			<!-- Signal strip — signature provenance readout -->
			<section class="mt-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
				<h2 class="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
					{t('radar.detail.provenance')}
				</h2>
				<div class="space-y-3">
					{#each [{ label: t('radar.detail.confidence'), pct: confidencePct }, { label: t('radar.detail.trust_tier'), pct: trustPct }, { label: t('radar.detail.freshness'), pct: freshnessPct }] as seg (seg.label)}
						<div>
							<div class="mb-1 flex items-center justify-between text-xs">
								<span class="text-[var(--color-muted)]">{seg.label}</span>
								<span class="font-medium text-[var(--color-text)]">{seg.pct}%</span>
							</div>
							<div class="h-1.5 overflow-hidden rounded-full bg-[var(--color-border)]">
								<div
									class="h-full rounded-full bg-[var(--color-primary)] transition-[width] duration-500"
									style="width: {seg.pct}%"
								></div>
							</div>
						</div>
					{/each}
				</div>
			</section>

			<!-- Apply CTA -->
			{#if applyUrl}
				<a
					href={applyUrl}
					target="_blank"
					rel="noopener noreferrer nofollow"
					class="mt-6 inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
				>
					{t('radar.detail.apply')}
					<ExternalLink size={15} />
				</a>
			{/if}
		</div>
	</article>
</div>
