<script lang="ts">
	// AcademyEnrollmentsWidget — admin-managed sidebar widget (Section 59, ADR-066).
	// Uses the shared academy_enrollments store so it stays in sync with other widgets
	// after mutations (enroll, mark-complete, progress update).
	import { t } from '$lib/i18n';
	import { GraduationCap } from 'lucide-svelte';
	import { enrollmentsStore } from '$lib/stores/enrollments.svelte';

	interface Props {
		title?: string;
		limit?: number;
	}
	let { title = '', limit = 3 }: Props = $props();

	// Trigger the shared fetch (deduplicated — if another widget already
	// fetched, this is a no-op).
	$effect(() => {
		enrollmentsStore.fetch();
	});

	const academy_enrollments = $derived(enrollmentsStore.academy_enrollments ?? []);
	const loading = $derived(enrollmentsStore.loading && academy_enrollments.length === 0);
	const visibleEnrollments = $derived(academy_enrollments.slice(0, limit));
</script>

<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<div class="mb-3 flex items-center gap-2">
		<GraduationCap size={18} class="text-[var(--color-primary)]" />
		<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('academy.my_learning', 'My Learning')}</h2>
	</div>
	{#if loading}
		<div class="space-y-2">
			{#each Array(limit) as _}
				<div class="jc-skeleton h-12 w-full rounded"></div>
			{/each}
		</div>
	{:else if visibleEnrollments.length > 0}
		<ul class="space-y-2">
			{#each visibleEnrollments as e (e.id)}
				<li>
					<a
						href="/tracks/{e.track_slug}"
						class="block rounded-[var(--radius)] p-2 hover:bg-[var(--color-surface-subtle)]"
					>
						<p class="truncate text-sm font-medium text-[var(--color-text)]">{e.track_title}</p>
						<p class="mt-0.5 text-xs text-[var(--color-muted)]">
							{e.lessons_completed ?? 0}/{e.lessons_total ?? 0} {t('academy.lessons', 'lessons')} · {e.progress_pct ?? 0}%
						</p>
						<div class="mt-1.5 h-1 w-full overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
							<div
								class="h-full rounded-full bg-[var(--color-progress)]"
								style="width: {e.progress_pct ?? 0}%"
							></div>
						</div>
					</a>
				</li>
			{/each}
		</ul>
		{#if academy_enrollments.length > limit}
			<a
				href="/my-learning"
				class="mt-3 block text-center text-xs font-medium text-[var(--color-primary)] hover:underline"
			>
				{t('academy.view_all', 'View all')} ({academy_enrollments.length})
			</a>
		{/if}
	{:else}
		<p class="text-xs text-[var(--color-muted)]">{t('academy.no_enrollments', 'No academy_enrollments yet')}</p>
		<a
			href="/"
			class="mt-2 inline-block rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
		>
			{t('academy.explore', 'Explore')}
		</a>
	{/if}
</section>
