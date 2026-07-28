<script lang="ts">
	// Lesson Player — Learn's reading exception (ADR-060, dev.to-single analog).
	// Three zones: CurriculumRail (left margin) · lesson content (center, by type) ·
	// lesson meta (right sidebar via SetSidebar).
	// Mark complete → POST /learn/lessons/:id/complete (optimistic Set, marking
	// guard, rollback, auto-next). No ads. Access verified server-side.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import CurriculumRail from '$lib/components/learn/CurriculumRail.svelte';
	import ProgressRing from '$lib/components/learn/ProgressRing.svelte';
	import { CheckCircle, ArrowRight, ArrowLeft, NotebookPen } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { Enrollment } from '$lib/types';

	let { data }: { data: PageData } = $props();
	const track = $derived(data.track);
	const lesson = $derived(data.lesson);

	// CSR ONLY — completed-lessons set + enrollment (ADR-060 SSR/CSR boundary).
	let completedLessonIds = $state<Set<string>>(new Set());
	let enrollment = $state<Enrollment | null>(null);
	let loadingState = $state(true);
	let marking = $state(false);

	$effect(() => {
		if (!data.user) {
			loadingState = false;
			return;
		}
		(async () => {
			// Fetch enrollments to get the completed-lessons set + progress.
			const r = await api.get<Enrollment[]>('/me/enrollments');
			loadingState = false;
			if (r.data) {
				enrollment = r.data.find((e) => e.track_id === track.id) ?? null;
			}
			// Seed the completed set from the SSR lesson's is_completed flag.
			if (lesson.is_completed) {
				completedLessonIds = new Set([lesson.id]);
			}
		})();
	});

	const isEnrolled = $derived(!!enrollment || !!lesson.is_preview);
	const isCompleted = $derived(completedLessonIds.has(lesson.id));
	const progressPct = $derived(enrollment?.progress_pct ?? 0);
	const totalLessons = $derived(track.modules.reduce((n, m) => n + m.lessons.length, 0));

	// Mark complete — optimistic Set, marking guard, rollback, auto-next.
	async function markComplete() {
		if (marking || isCompleted) return;
		marking = true;
		// Optimistic: add to the set immediately.
		const prev = new Set(completedLessonIds);
		completedLessonIds = new Set([...completedLessonIds, lesson.id]);
		const r = await api.post<{ ok: boolean }>(`/learn/lessons/${lesson.id}/complete`);
		marking = false;
		if (r.error) {
			// Rollback on failure.
			completedLessonIds = prev;
			return;
		}
		// Auto-next: navigate to the next lesson if one exists.
		if (lesson.next_lesson) {
			goto(`/tracks/${track.slug}/${lesson.next_lesson.module_slug}/${lesson.next_lesson.slug}`);
		}
	}

	// Video progress tracking (browser-guarded init).
	let videoEl = $state<HTMLVideoElement | null>(null);
	let lastProgressSave = 0;
	function onVideoTimeUpdate() {
		if (!videoEl || !data.user) return;
		const now = Math.floor(videoEl.currentTime);
		// Save progress every 15 seconds.
		if (now - lastProgressSave >= 15) {
			lastProgressSave = now;
			api.post(`/learn/lessons/${lesson.id}/progress`, { progress_secs: now });
		}
	}
</script>

<svelte:head>
	<title>{lesson.title} · {track.title} · {t('app.name')}</title>
	<meta name="description" content={lesson.title} />
	<link rel="canonical" href={`https://learn.japaconnect.com/tracks/${track.slug}/${lesson.module_title ?? ''}/${lesson.slug}`} />
	<script type="application/ld+json">
		{JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'LearningResource',
			name: lesson.title,
			isPartOf: { '@type': 'Course', name: track.title },
			teaches: lesson.title
		})}
	</script>
</svelte:head>

<!-- Right sidebar: lesson meta (progress ring, next lesson, notes).
     SetSidebar registers its children as the AppShell right-sidebar snippet. -->
<SetSidebar>
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('learn.progress')}</h2>
		<div class="flex items-center gap-3">
			<ProgressRing value={progressPct} />
			<div class="text-sm">
				<p class="text-[var(--color-text)]">{enrollment?.lessons_completed ?? 0}/{enrollment?.lessons_total ?? totalLessons}</p>
				<p class="text-[var(--color-muted)]">{t('learn.lessons')}</p>
			</div>
		</div>
	</section>

	{#if lesson.next_lesson}
		<a
			href="/tracks/{track.slug}/${lesson.next_lesson.module_slug}/${lesson.next_lesson.slug}"
			class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 hover:bg-[var(--color-surface-subtle)]"
		>
			<p class="mb-1 text-xs text-[var(--color-muted)]">{t('learn.up_next')}</p>
			<p class="flex items-center gap-1 text-sm font-medium text-[var(--color-primary)]">
				{lesson.next_lesson.title} <ArrowRight size={14} />
			</p>
		</a>
	{/if}

	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<NotebookPen size={14} /> Notes
		</h2>
		<textarea
			placeholder="Write your notes…"
			class="h-24 w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
		></textarea>
	</section>
</SetSidebar>

<!-- Three-zone layout: curriculum rail | lesson content. Right meta via sidebar. -->
<div class="lesson-player-grid">
	<!-- Zone 1: Curriculum rail (content's left margin) -->
	<CurriculumRail
		{track}
		currentLessonSlug={lesson.slug}
		{completedLessonIds}
		{isEnrolled}
	/>

	<!-- Zone 2: Lesson content (center, by type) -->
	<article class="lesson-content px-4 py-6 md:px-8 lg:px-12">
		<!-- Lesson header -->
		<header class="mb-6 max-w-3xl">
			{#if lesson.module_title}
				<p class="text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{lesson.module_title}
				</p>
			{/if}
			<h1 class="mt-1 text-2xl font-bold text-[var(--color-text)] md:text-3xl">{lesson.title}</h1>
		</header>

		<!-- Lesson body by type -->
		<div class="max-w-3xl">
			{#if lesson.lesson_type === 'article'}
				<!-- Article: SSR TipTap .prose -->
				{#if lesson.content_html}
					<div class="prose">{@html lesson.content_html}</div>
				{:else}
					<p class="text-[var(--color-muted)]">{t('common.empty')}</p>
				{/if}
			{:else if lesson.lesson_type === 'video'}
				<!-- Video: browser-guarded init, progress tracking -->
				{#if lesson.video_url}
					<!-- svelte-ignore a11y_media_has_caption -->
					<video
						bind:this={videoEl}
						src={lesson.video_url}
						controls
						class="w-full rounded-[var(--radius-card)] bg-black"
						ontimeupdate={onVideoTimeUpdate}
					></video>
				{:else}
					<p class="text-[var(--color-muted)]">{t('common.empty')}</p>
				{/if}
			{:else if lesson.lesson_type === 'quiz'}
				<!-- Quiz engine — TODO: wire to GET/POST /learn/quizzes/:id -->
				{#if lesson.quiz}
					<p class="text-[var(--color-text-subtle)]">{lesson.quiz.title}</p>
					<p class="mt-2 text-sm text-[var(--color-muted)]">{t('lesson.submit_quiz')}</p>
				{:else}
					<p class="text-[var(--color-muted)]">{t('common.empty')}</p>
				{/if}
			{:else if lesson.lesson_type === 'checklist'}
				<!-- Checklist: TaskList -->
				{#if lesson.checklist && lesson.checklist.length > 0}
					<ul class="space-y-2">
						{#each lesson.checklist as item (item.id)}
							<li class="flex items-start gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								<input type="checkbox" id={item.id} class="mt-1" />
								<label for={item.id} class="text-sm text-[var(--color-text-subtle)]">{item.label}</label>
							</li>
						{/each}
					</ul>
				{:else}
					<p class="text-[var(--color-muted)]">{t('common.empty')}</p>
				{/if}
			{/if}
		</div>

		<!-- Mark complete + prev/next nav -->
		<div class="mt-8 flex max-w-3xl flex-wrap items-center justify-between gap-4 border-t border-[var(--color-border)] pt-6">
			{#if lesson.prev_lesson}
				<a
					href="/tracks/{track.slug}/{lesson.prev_lesson.module_slug}/${lesson.prev_lesson.slug}"
					class="inline-flex items-center gap-1 text-sm text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
				>
					<ArrowLeft size={16} /> {t('learn.prev_lesson')}
				</a>
			{:else}
				<span></span>
			{/if}

			<button
				type="button"
				onclick={markComplete}
				disabled={marking || isCompleted || !isEnrolled}
				class="inline-flex items-center gap-2 rounded-[var(--radius)] px-4 py-2 text-sm font-medium transition-colors {isCompleted ? 'bg-[var(--color-surface-subtle)] text-[var(--color-progress)]' : 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'} disabled:opacity-60"
			>
				{#if isCompleted}
					<CheckCircle size={16} /> {t('mark.completed')}
				{:else if marking}
					{t('learn.marking')}
				{:else}
					{t('learn.mark_complete')}
				{/if}
			</button>

			{#if lesson.next_lesson}
				<a
					href="/tracks/{track.slug}/{lesson.next_lesson.module_slug}/${lesson.next_lesson.slug}"
					class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline"
				>
					{t('learn.next_lesson')} <ArrowRight size={16} />
				</a>
			{:else}
				<span></span>
			{/if}
		</div>
	</article>
</div>

<style>
	.lesson-player-grid {
		display: grid;
		grid-template-columns: var(--curriculum-rail-width) 1fr;
		min-height: calc(100vh - var(--nav-height));
	}

	@media (max-width: 1023px) {
		.lesson-player-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
