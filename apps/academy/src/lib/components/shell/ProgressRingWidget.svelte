<script lang="ts">
	// ProgressRingWidget — admin-managed sidebar widget (Section 59, ADR-066).
	// Uses the shared academy_enrollments store so it stays in sync with other widgets
	// (ContinueAcademyCard, AcademyEnrollmentsWidget) after mutations.
	import { t } from '$lib/i18n';
	import ProgressRing from '$lib/components/academy/ProgressRing.svelte';
	import { enrollmentsStore } from '$lib/stores/enrollments.svelte';

	interface Props {
		title?: string;
	}
	let { title = '' }: Props = $props();

	// Trigger the shared fetch (deduplicated — if ContinueAcademyCard already
	// fetched, this is a no-op).
	$effect(() => {
		enrollmentsStore.fetch();
	});

	const academy_enrollments = $derived(enrollmentsStore.academy_enrollments ?? []);
	const loading = $derived(enrollmentsStore.loading && academy_enrollments.length === 0);

	// Aggregate progress: total completed lessons across all academy_enrollments
	// divided by total lessons across all academy_enrollments. This is the correct
	// way to compute aggregate progress — averaging per-track percentages
	// gives wrong results (e.g. 100% + 0% → 50% when the real aggregate is
	// 1/7 = 14%).
	const totalCompleted = $derived(
		academy_enrollments.reduce((sum, e) => sum + (e.lessons_completed ?? 0), 0)
	);
	const totalLessons = $derived(
		academy_enrollments.reduce((sum, e) => sum + (e.lessons_total ?? 0), 0)
	);
	const avgPct = $derived(
		totalLessons > 0 ? Math.round((totalCompleted / totalLessons) * 100) : 0
	);
</script>

<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
	<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('academy.progress', 'My Progress')}</h2>
	{#if loading}
		<div class="mx-auto grid h-20 w-20 place-items-center">
			<div class="jc-skeleton h-16 w-16 rounded-full"></div>
		</div>
		<p class="mt-2 text-xs text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
	{:else if academy_enrollments.length > 0}
		<div class="mx-auto grid h-20 w-20 place-items-center">
			<ProgressRing value={avgPct} />
		</div>
		<p class="mt-2 text-sm font-medium text-[var(--color-text)]">{avgPct}%</p>
		<p class="text-xs text-[var(--color-muted)]">{totalCompleted}/{totalLessons} {t('academy.lessons', 'lessons')}</p>
	{:else}
		<div class="mx-auto grid h-20 w-20 place-items-center">
			<ProgressRing value={0} />
		</div>
		<p class="mt-2 text-xs text-[var(--color-muted)]">{t('academy.no_enrollments', 'No academy_enrollments yet')}</p>
		<a
			href="/"
			class="mt-2 inline-block rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
		>
			{t('academy.explore', 'Explore')}
		</a>
	{/if}
</section>
