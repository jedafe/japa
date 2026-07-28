<script lang="ts">
	// CurriculumRail — module→lesson tree in the content's LEFT margin (ADR-060).
	// States: current (▶), completed (✓), locked (🔒), not-started (○).
	// Sticky/scrollable. Belongs to the content — visually distinct from the
	// collapsed nav, NOT a second nav.
	import { t } from '$lib/i18n';
	import { PlayCircle, CheckCircle, Lock, Circle } from 'lucide-svelte';
	import type { TrackDetail, LessonSummary } from '$lib/types';

	interface Props {
		track: TrackDetail;
		/** Current lesson slug (for the ▶ current marker). */
		currentLessonSlug: string;
		/** CSR-resolved set of completed lesson IDs. */
		completedLessonIds: Set<string>;
		/** CSR-resolved set of enrolled lesson IDs (for lock state). If the user
		 *  is enrolled, no lessons are locked. If not enrolled, only preview
		 *  lessons are unlocked. */
		isEnrolled: boolean;
	}
	let {
		track,
		currentLessonSlug,
		completedLessonIds,
		isEnrolled
	}: Props = $props();

	function lessonState(lesson: LessonSummary): 'current' | 'completed' | 'locked' | 'not-started' {
		if (lesson.slug === currentLessonSlug) return 'current';
		if (completedLessonIds.has(lesson.id)) return 'completed';
		if (!isEnrolled && !lesson.is_preview) return 'locked';
		return 'not-started';
	}

	function stateIcon(state: string) {
		switch (state) {
			case 'current':
				return PlayCircle;
			case 'completed':
				return CheckCircle;
			case 'locked':
				return Lock;
			default:
				return Circle;
		}
	}

	function stateColor(state: string): string {
		switch (state) {
			case 'current':
				return 'var(--color-primary)';
			case 'completed':
				return 'var(--color-progress)';
			case 'locked':
				return 'var(--color-locked)';
			default:
				return 'var(--color-muted)';
		}
	}
</script>

<aside
	class="curriculum-rail sticky top-0 h-screen overflow-y-auto border-r border-[var(--color-border)] bg-[var(--color-surface)]"
>
	<div class="px-4 py-4">
		<a
			href="/tracks/{track.slug}"
			class="block truncate text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]"
		>
			{track.title}
		</a>
		<p class="mt-1 text-xs text-[var(--color-muted)]">{t('learn.curriculum')}</p>
	</div>

	<nav class="px-2 pb-8" aria-label={t('learn.curriculum')}>
		{#each track.modules as module (module.id)}
			<div class="mb-3">
				<p class="px-2 py-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{module.title}
				</p>
				<ul class="space-y-0.5">
					{#each module.lessons as lesson (lesson.id)}
						{@const state = lessonState(lesson)}
						{@const Icon = stateIcon(state)}
						{@const href = state === 'locked' ? undefined : `/tracks/${track.slug}/${module.slug}/${lesson.slug}`}
						<li>
							{#if href}
								<a
									{href}
									class="flex items-center gap-2 rounded-[var(--radius)] px-2 py-1.5 text-sm transition-colors {state === 'current' ? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-text)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]'}"
									aria-current={state === 'current' ? 'page' : undefined}
								>
									<Icon size={14} class="shrink-0" style="color: {stateColor(state)}" />
									<span class="flex-1 truncate">{lesson.title}</span>
								</a>
							{:else}
								<span
									class="flex cursor-not-allowed items-center gap-2 rounded-[var(--radius)] px-2 py-1.5 text-sm text-[var(--color-locked)]"
									title={t('learn.locked')}
								>
									<Icon size={14} class="shrink-0" style="color: {stateColor(state)}" />
									<span class="flex-1 truncate">{lesson.title}</span>
								</span>
							{/if}
						</li>
					{/each}
				</ul>
			</div>
		{/each}
	</nav>
</aside>

<style>
	.curriculum-rail {
		width: var(--curriculum-rail-width);
	}

	@media (max-width: 1023px) {
		.curriculum-rail {
			display: none;
		}
	}
</style>
