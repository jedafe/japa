<script lang="ts">
	// Lesson Player — Academy's reading exception (ADR-060, dev.to-single analog).
	// Three zones: CurriculumRail (left margin) · lesson content (center, by type) ·
	// lesson meta (right sidebar via SetSidebar).
	// Mark complete → POST /academy/lessons/:id/complete (optimistic Set, marking
	// guard, rollback, auto-next). No ads. Access verified server-side.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { loginUrl } from '$lib/appOrigins';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import CurriculumRail from '$lib/components/academy/CurriculumRail.svelte';
	import ProgressRing from '$lib/components/academy/ProgressRing.svelte';
	import { CheckCircle, ArrowRight, ArrowLeft, NotebookPen } from 'lucide-svelte';
	import { enrollmentsStore } from '$lib/stores/enrollments.svelte';
	import type { PageData } from './$types';
	import type { Enrollment, Quiz, QuizQuestion, QuizResult, QuizSubmission } from '$lib/types';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { excerptFromHtml, safeJsonLd } from '$lib/seo';

	let { data }: { data: PageData } = $props();
	const track = $derived(data.track);
	const lesson = $derived(data.lesson);

	// CSR ONLY — completed-lessons set + enrollment (ADR-060 SSR/CSR boundary).
	// Uses the shared enrollmentsStore so sidebar widgets (Continue Learning,
	// My Progress, My Learning) stay in sync after mark-complete.
	let completedLessonIds = $state<Set<string>>(new Set());
	let marking = $state(false);
	// Certificate polling — when the track reaches 100% completion, poll
	// /me/certificates until the async academy:issue_certificate task
	// generates the certificate (ROADMAP-ACADEMY 07.05).
	let trackCompleted = $state(false);
	let certPolling = $state(false);
	let issuedCert = $state<{ id: string; certificate_number: string } | null>(null);

	$effect(() => {
		if (!data.user) {
			return;
		}
		(async () => {
			await enrollmentsStore.fetch();
			// Fetch the full set of completed lesson IDs for this track so the
			// curriculum rail restores completion state on page refresh.
			// The SSR lesson's is_completed flag only covers the current lesson.
			const pr = await api.get<{ completed_lesson_ids?: string[] }>(
				`/academy/progress/${track.id}`
			);
			if (pr.data?.completed_lesson_ids) {
				completedLessonIds = new Set(pr.data.completed_lesson_ids);
			} else if (lesson.is_completed) {
				// Fallback: seed from the SSR flag if the progress endpoint fails.
				completedLessonIds = new Set([lesson.id]);
			}
		})();
	});

	// Derive the enrollment for this track from the shared store.
	const enrollment = $derived(
		enrollmentsStore.academy_enrollments?.find((e) => e.track_id === track.id) ?? null
	);
	const loadingState = $derived(enrollmentsStore.loading && !enrollmentsStore.academy_enrollments);

	// isEnrolled is strictly the enrollment record — it drives the curriculum
	// rail's per-lesson lock state (locked = !enrolled && !is_preview).
	// canInteract additionally admits guests on preview lessons for the
	// mark-complete button (which redirects them to login).
	const isEnrolled = $derived(!!enrollment);
	const canInteract = $derived(isEnrolled || !!lesson.is_preview);
	const isCompleted = $derived(completedLessonIds.has(lesson.id));
	const progressPct = $derived(enrollment?.progress_pct ?? 0);
	const totalLessons = $derived(track.modules.reduce((n, m) => n + m.lessons.length, 0));

	// Mark complete — optimistic Set, marking guard, rollback, auto-next.
	// After success, refresh the shared academy_enrollments store so all sidebar
	// widgets (Continue Learning, My Progress, My Learning) update reactively.
	async function markComplete() {
		// A guest can reach a *preview* lesson with "Mark complete" enabled.
		// Guard against firing an authed mutation without a session.
		if (!data.user) {
			window.location.href = loginUrl(`/tracks/${page.params.slug}/${page.params.module}/${page.params.lesson}`);
			return;
		}
		if (marking || isCompleted) return;
		marking = true;
		// Optimistic: add to the set immediately.
		const prev = new Set(completedLessonIds);
		completedLessonIds = new Set([...completedLessonIds, lesson.id]);
		const r = await api.post<{ ok: boolean }>(`/academy/lessons/${lesson.id}/complete`);
		marking = false;
		if (r.error) {
			// Rollback on failure.
			completedLessonIds = prev;
			return;
		}
		// Refresh the shared store so sidebar widgets reflect the new progress.
		enrollmentsStore.refresh();
		// Re-fetch the completed lesson IDs so the curriculum rail stays in
		// sync (the optimistic set already has this lesson, but the re-fetch
		// catches any other changes from other tabs/devices).
		const pr = await api.get<{ completed_lesson_ids?: string[] }>(
			`/academy/progress/${track.id}`
		);
		if (pr.data?.completed_lesson_ids) {
			completedLessonIds = new Set(pr.data.completed_lesson_ids);
		}
		// Check if the track is now 100% complete → poll for the certificate.
		// The backend issues the certificate asynchronously (academy:issue_certificate
		// or the synchronous maybeIssueCertificate path). Poll /me/certificates
		// for up to 15s (ROADMAP-ACADEMY 07.05).
		if (completedLessonIds.size >= totalLessons && totalLessons > 0) {
			trackCompleted = true;
			pollForCertificate();
		}
		// Auto-next: navigate to the next lesson if one exists.
		if (lesson.next_lesson) {
			goto(`/tracks/${track.slug}/${lesson.next_lesson.module_slug}/${lesson.next_lesson.slug}`);
		}
	}

	// Poll /me/certificates until a certificate for this track appears or
	// the 15s timeout expires (ROADMAP-ACADEMY 07.05). The backend issues the
	// certificate asynchronously on 100% completion.
	async function pollForCertificate() {
		if (certPolling) return;
		certPolling = true;
		const maxAttempts = 8; // 8 × 2s = 16s max
		const interval = 2000;
		for (let i = 0; i < maxAttempts; i++) {
			await new Promise((r) => setTimeout(r, interval));
			const r = await api.get<{ id: string; certificate_number: string; track_id: string }[]>(
				'/me/certificates'
			);
			if (r.data) {
				const cert = r.data.find((c) => c.track_id === track.id);
				if (cert) {
					issuedCert = cert;
					certPolling = false;
					return;
				}
			}
		}
		certPolling = false;
	}

	// Video progress tracking (browser-guarded init). The backend persists
	// progress_secs → academy_lesson_progress.position_secs and returns it on the
	// lesson payload, so the player resumes where the user left off.
	let videoEl = $state<HTMLVideoElement | null>(null);
	let lastProgressSave = 0;
	let resumeApplied = $state(false);
	function onVideoLoaded() {
		if (resumeApplied || !videoEl) return;
		resumeApplied = true;
		const pos = lesson.progress_secs ?? 0;
		if (pos > 0 && videoEl.duration && pos < videoEl.duration - 2) {
			videoEl.currentTime = pos;
		}
	}
	function onVideoTimeUpdate() {
		if (!videoEl || !data.user) return;
		const now = Math.floor(videoEl.currentTime);
		// Save progress every 15 seconds.
		if (now - lastProgressSave >= 15) {
			lastProgressSave = now;
			api.post(`/academy/lessons/${lesson.id}/progress`, { progress_secs: now });
		}
	}

	// Quiz engine state — answers map, submission, results, retake.
	let quizAnswers = $state<Record<string, string[]>>({});
	let quizTextAnswers = $state<Record<string, string>>({});
	let quizSubmitted = $state(false);
	let quizResult = $state<QuizResult | null>(null);
	let quizSubmitting = $state(false);

	// Reset quiz state when the lesson changes.
	$effect(() => {
		const _ = lesson.id;
		quizAnswers = {};
		quizTextAnswers = {};
		quizSubmitted = false;
		quizResult = null;
		quizSubmitting = false;
	});

	// All questions answered? Submit guard (called reactively in template).
	function quizCanSubmit(): boolean {
		const q = lesson.quiz;
		if (!q || quizSubmitting) return false;
		return q.questions.every((question: QuizQuestion) => {
			if (question.type === 'text') {
				return (quizTextAnswers[question.id] ?? '').trim().length > 0;
			}
			return (quizAnswers[question.id] ?? []).length > 0;
		});
	}

	function toggleOption(question: QuizQuestion, optionId: string) {
		if (quizSubmitted) return;
		if (question.type === 'single') {
			quizAnswers = { ...quizAnswers, [question.id]: [optionId] };
		} else {
			const current = quizAnswers[question.id] ?? [];
			quizAnswers = {
				...quizAnswers,
				[question.id]: current.includes(optionId)
					? current.filter((id) => id !== optionId)
					: [...current, optionId]
			};
		}
	}

	async function submitQuiz() {
		// A guest can reach a *preview* lesson with the quiz engine rendered.
		// Guard against firing an authed mutation without a session.
		if (!data.user) {
			window.location.href = loginUrl(`/tracks/${page.params.slug}/${page.params.module}/${page.params.lesson}`);
			return;
		}
		const q = lesson.quiz;
		if (!q || !quizCanSubmit()) return;
		quizSubmitting = true;
		const payload: QuizSubmission[] = q.questions.map((question: QuizQuestion) => ({
			question_id: question.id,
			answer:
				question.type === 'text'
					? (quizTextAnswers[question.id] ?? '')
					: (quizAnswers[question.id] ?? [])
		}));
		const r = await api.post<QuizResult>(`/academy/quizzes/${q.id}/submit`, { answers: payload });
		quizSubmitting = false;
		if (r.data) {
			quizResult = r.data;
			quizSubmitted = true;
		}
	}

	function retakeQuiz() {
		quizAnswers = {};
		quizTextAnswers = {};
		quizSubmitted = false;
		quizResult = null;
	}

	// Look up per-question result after submission.
	function questionResult(questionId: string) {
		return quizResult?.results.find((r) => r.question_id === questionId) ?? null;
	}
</script>

<svelte:head>
	<title>{lesson.title} · {track.title} · {t('app.name')}</title>
	<meta name="description" content={excerptFromHtml(lesson.content_html) ?? lesson.title} />
	{#if lesson.is_preview}
		<meta name="robots" content="index, follow" />
	{:else}
		<meta name="robots" content="noindex, nofollow" />
	{/if}
	<link rel="canonical" href={`https://academy.japaconnect.com/tracks/${track.slug}/${page.params.module}/${lesson.slug}`} />
	<meta property="og:title" content={`${lesson.title} · ${track.title}`} />
	<meta property="og:description" content={excerptFromHtml(lesson.content_html) ?? lesson.title} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={`https://academy.japaconnect.com/tracks/${track.slug}/${page.params.module}/${lesson.slug}`} />
	{#if track.thumbnail_url}
		<meta property="og:image" content={track.thumbnail_url} />
	{/if}
	<meta name="twitter:card" content={track.thumbnail_url ? 'summary_large_image' : 'summary'} />
	<meta name="twitter:title" content={`${lesson.title} · ${track.title}`} />
	<meta name="twitter:description" content={excerptFromHtml(lesson.content_html) ?? lesson.title} />
	{#if track.thumbnail_url}
		<meta name="twitter:image" content={track.thumbnail_url} />
	{/if}
	{@html safeJsonLd({
			'@context': 'https://schema.org',
			'@type': 'LearningResource',
			name: lesson.title,
			description: excerptFromHtml(lesson.content_html) ?? undefined,
			isPartOf: { '@type': 'Course', name: track.title, url: `https://academy.japaconnect.com/tracks/${track.slug}` },
			teaches: lesson.title
		})}
</svelte:head>

<!-- Right sidebar: lesson meta (progress ring, next lesson, notes).
     SetSidebar registers its children as the AppShell right-sidebar snippet. -->
<SetSidebar>
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('academy.progress')}</h2>
		<div class="flex items-center gap-3">
			<ProgressRing value={progressPct} />
			<div class="text-sm">
				<p class="text-[var(--color-text)]">{enrollment?.lessons_completed ?? 0}/{enrollment?.lessons_total ?? totalLessons}</p>
				<p class="text-[var(--color-muted)]">{t('academy.lessons')}</p>
			</div>
		</div>
	</section>

	{#if lesson.next_lesson}
		<a
			href="/tracks/{track.slug}/{lesson.next_lesson.module_slug}/{lesson.next_lesson.slug}"
			class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 hover:bg-[var(--color-surface-subtle)]"
		>
			<p class="mb-1 text-xs text-[var(--color-muted)]">{t('academy.up_next')}</p>
			<p class="flex items-center gap-1 text-sm font-medium text-[var(--color-primary)]">
				{lesson.next_lesson.title} <ArrowRight size={14} />
			</p>
		</a>
	{/if}

	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<NotebookPen size={14} /> {t('academy.notes')}
		</h2>
		<textarea
			placeholder={t('academy.notes_placeholder', 'Write your notes…')}
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
					<div class="prose">{@html sanitizeContentHtml(lesson.content_html)}</div>
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
						onloadedmetadata={onVideoLoaded}
					></video>
				{:else}
					<p class="text-[var(--color-muted)]">{t('common.empty')}</p>
				{/if}
			{:else if lesson.lesson_type === 'quiz'}
				<!-- Quiz engine — GET on load, POST /academy/quizzes/:id/submit -->
				{#if lesson.quiz}
					{@const quiz = lesson.quiz as Quiz}
					<section class="quiz-engine">
						<header class="mb-4">
							<h2 class="text-xl font-bold text-[var(--color-text)]">{quiz.title}</h2>
							{#if quiz.description}
								<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{quiz.description}</p>
							{/if}
							{#if quiz.pass_score_pct != null}
								<p class="mt-1 text-xs text-[var(--color-muted)]">
									{t('academy.pass_score', 'Pass score')}: {quiz.pass_score_pct}%
								</p>
							{/if}
						</header>

						{#if quizSubmitted && quizResult}
							<!-- Overall result banner -->
							<div
								class="mb-6 rounded-[var(--radius-card)] border p-4 {quizResult.passed
									? 'border-[var(--color-progress)] bg-[var(--color-surface)]'
									: 'border-[var(--color-destructive)] bg-[var(--color-surface)]'}"
							>
								<p class="text-lg font-bold text-[var(--color-text)]">
									{Math.round(quizResult.score_pct)}% — {quizResult.passed
										? t('academy.quiz_passed', 'Passed!')
										: t('academy.quiz_failed', 'Not passed')}
								</p>
								<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
									{quizResult.earned_points}/{quizResult.total_points} {t('academy.points', 'points')}
								</p>
							</div>
						{/if}

						<ol class="space-y-6">
							{#each quiz.questions as question, qi (question.id)}
								{@const qr = questionResult(question.id)}
								<li
									class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
								>
									<div class="mb-3 flex items-start gap-2">
										{#if quizSubmitted && qr}
											<span
												class="mt-0.5 font-bold {qr.correct
													? 'text-[var(--color-progress)]'
													: 'text-[var(--color-destructive)]'}"
											>
												{qr.correct ? '✓' : '✗'}
											</span>
										{/if}
										<p class="text-sm font-medium text-[var(--color-text)]">
											{qi + 1}. {question.prompt}
										</p>
									</div>

									<!-- Single: radio buttons -->
									{#if question.type === 'single' && question.options}
										<ul class="space-y-2">
											{#each question.options as option (option.id)}
												{@const selected = (quizAnswers[question.id] ?? []).includes(option.id)}
												<li>
													<label
														class="flex cursor-pointer items-center gap-2 rounded-[var(--radius)] border p-2 text-sm transition-colors {selected
															? 'border-[var(--color-primary)] bg-[var(--color-surface)]'
															: 'border-[var(--color-border)] hover:bg-[var(--color-surface)]'} {quizSubmitted
															? 'cursor-default'
															: ''}"
													>
														<input
															type="radio"
															name={question.id}
															checked={selected}
															disabled={quizSubmitted}
															onchange={() => toggleOption(question, option.id)}
														/>
														<span class="text-[var(--color-text)]">{option.label}</span>
													</label>
												</li>
											{/each}
										</ul>
									{:else if question.type === 'multiple' && question.options}
										<!-- Multiple: checkboxes -->
										<ul class="space-y-2">
											{#each question.options as option (option.id)}
												{@const selected = (quizAnswers[question.id] ?? []).includes(option.id)}
												<li>
													<label
														class="flex cursor-pointer items-center gap-2 rounded-[var(--radius)] border p-2 text-sm transition-colors {selected
															? 'border-[var(--color-primary)] bg-[var(--color-surface)]'
															: 'border-[var(--color-border)] hover:bg-[var(--color-surface)]'} {quizSubmitted
															? 'cursor-default'
															: ''}"
													>
														<input
															type="checkbox"
															checked={selected}
															disabled={quizSubmitted}
															onchange={() => toggleOption(question, option.id)}
														/>
														<span class="text-[var(--color-text)]">{option.label}</span>
													</label>
												</li>
											{/each}
										</ul>
									{:else if question.type === 'text'}
										<!-- Text: free-text input -->
										<input
											type="text"
											value={quizTextAnswers[question.id] ?? ''}
											disabled={quizSubmitted}
											oninput={(e) => {
												if (quizSubmitted) return;
												quizTextAnswers = {
													...quizTextAnswers,
													[question.id]: e.currentTarget.value
												};
											}}
											placeholder={t('academy.type_answer', 'Type your answer…')}
											class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none disabled:opacity-70"
										/>
									{/if}

									<!-- Per-question feedback after submit -->
									{#if quizSubmitted && qr}
										{#if !qr.correct && qr.correct_answers.length > 0}
											<p class="mt-3 text-xs text-[var(--color-text-subtle)]">
												{t('academy.correct_answers', 'Correct answers')}:
												{#if question.options}
													{#each qr.correct_answers as cid, i}{#if i > 0}, {/if}{question.options?.find((o) => o.id === cid)?.label ?? cid}{/each}
												{:else}
													{qr.correct_answers.join(', ')}
												{/if}
											</p>
										{/if}
										{#if qr.explanation}
											<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{qr.explanation}</p>
										{:else if question.explanation}
											<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{question.explanation}</p>
										{/if}
									{/if}
								</li>
							{/each}
						</ol>

						<!-- Submit / Retake controls -->
						<div class="mt-6 flex items-center gap-3">
							{#if !quizSubmitted}
								<button
									type="button"
									onclick={submitQuiz}
									disabled={!quizCanSubmit()}
									class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
								>
									{#if quizSubmitting}
										{t('academy.submitting', 'Submitting…')}
									{:else}
										{t('lesson.submit_quiz', 'Submit quiz')}
									{/if}
								</button>
							{:else}
								<button
									type="button"
									onclick={retakeQuiz}
									class="inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface)]"
								>
									{t('academy.retake_quiz', 'Retake quiz')}
								</button>
							{/if}
						</div>
					</section>
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
					<ArrowLeft size={16} /> {t('academy.prev_lesson')}
				</a>
			{:else}
				<span></span>
			{/if}

			<button
				type="button"
				onclick={markComplete}
				disabled={marking || isCompleted || !canInteract}
				class="inline-flex items-center gap-2 rounded-[var(--radius)] px-4 py-2 text-sm font-medium transition-colors {isCompleted ? 'bg-[var(--color-surface-subtle)] text-[var(--color-progress)]' : 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'} disabled:opacity-60"
			>
				{#if isCompleted}
					<CheckCircle size={16} /> {t('mark.completed')}
				{:else if marking}
					{t('academy.marking')}
				{:else}
					{t('academy.mark_complete')}
				{/if}
			</button>

			{#if lesson.next_lesson}
				<a
					href="/tracks/{track.slug}/{lesson.next_lesson.module_slug}/{lesson.next_lesson.slug}"
					class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline"
				>
					{t('academy.next_lesson')} <ArrowRight size={16} />
				</a>
			{:else}
				<span></span>
			{/if}
		</div>

		<!-- Track completion celebration + certificate link (ROADMAP-ACADEMY 07.05) -->
		{#if trackCompleted}
			<div class="mt-6 max-w-3xl rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-primary)_6%,transparent)] p-6 text-center">
				{#if issuedCert}
					<p class="text-lg font-semibold text-[var(--color-text)]">{t('academy.track_complete', 'Track Complete!')}</p>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('academy.certificate_issued', 'Your certificate is ready.')}</p>
					<a
						href="/certificates/{issuedCert.id}"
						class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
					>
						{t('academy.view_certificate', 'View certificate')}
					</a>
				{:else if certPolling}
					<p class="text-sm text-[var(--color-text-subtle)]">{t('academy.generating_certificate', 'Generating your certificate…')}</p>
				{:else}
					<p class="text-lg font-semibold text-[var(--color-text)]">{t('academy.track_complete', 'Track Complete!')}</p>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('academy.certificate_pending', 'Your certificate will appear in My Certificates shortly.')}</p>
				{/if}
			</div>
		{/if}
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
