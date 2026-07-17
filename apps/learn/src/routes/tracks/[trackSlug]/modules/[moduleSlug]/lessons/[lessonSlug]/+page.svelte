<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { api, type TrackDetail, type Lesson, type QuizQuestion, type QuizResult } from '$lib/api';
	import DOMPurify from 'dompurify';
	import {
		CheckCircle2,
		Circle,
		ArrowLeft,
		ArrowRight,
		BookOpen,
		Clock,
		CheckCheck,
		Loader2,
		Play,
		Video,
		XCircle,
		CheckCircle
	} from 'lucide-svelte';

	let { data } = $props();

	let lesson = $derived(data.lesson as Lesson);
	let track = $derived(data.track as TrackDetail);
	let completedLessons = $state<Set<string>>(new Set());
	let markingComplete = $state(false);
	let isCompleted = $state(false);
	let progressPercent = $state(0);

	const prevLesson = $derived(data.prevLesson);
	const nextLesson = $derived(data.nextLesson);
	const currentModule = $derived(data.currentModule);
	const allLessons = $derived(data.allLessons);

	const sanitizedBody = $derived(
		browser
			? DOMPurify.sanitize(typeof lesson.body === 'string' ? lesson.body : JSON.stringify(lesson.body))
			: typeof lesson.body === 'string'
				? lesson.body
				: JSON.stringify(lesson.body)
	);

	// Quiz state
	let quizAnswers = $state<Record<string, number>>({});
	let quizSubmitted = $state(false);
	let quizResult = $state<QuizResult | null>(null);
	let quizLoading = $state(false);
	let quizError = $state('');

	onMount(async () => {
		try {
			const progress = await api.getProgress(track.id);
			completedLessons = new Set(progress.completed_lessons ?? []);
			const completedCount = progress.completed_lessons?.length ?? 0;
			const totalLessons = track.lesson_count;
			progressPercent = totalLessons > 0 ? Math.round((completedCount / totalLessons) * 100) : 0;
		} catch {
			// No progress yet
		}
		isCompleted = completedLessons.has(lesson.id);
	});

	async function markComplete() {
		markingComplete = true;
		try {
			await api.completeLesson(lesson.id);
			isCompleted = true;
			completedLessons = new Set([...completedLessons, lesson.id]);
			const total = track.lesson_count;
			const completed = completedLessons.size;
			progressPercent = total > 0 ? Math.round((completed / total) * 100) : 0;
		} catch {
			// Silently handle error
		} finally {
			markingComplete = false;
		}
	}

	function isLessonDone(lessonId: string): boolean {
		return completedLessons.has(lessonId);
	}

	async function submitQuiz() {
		if (!lesson.quiz_id) return;
		quizLoading = true;
		quizError = '';
		try {
			quizResult = await api.submitQuiz(lesson.quiz_id, quizAnswers);
			quizSubmitted = true;
		} catch (e) {
			quizError = e instanceof Error ? e.message : 'Failed to submit quiz';
		} finally {
			quizLoading = false;
		}
	}

	function getVideoEmbedUrl(url: string): string {
		const youtubeMatch = url.match(/(?:youtube\.com\/watch\?v=|youtu\.be\/)([^&]+)/);
		if (youtubeMatch) return `https://www.youtube.com/embed/${youtubeMatch[1]}`;
		const vimeoMatch = url.match(/vimeo\.com\/(\d+)/);
		if (vimeoMatch) return `https://player.vimeo.com/video/${vimeoMatch[1]}`;
		return url;
	}
</script>

<svelte:head>
	<title>{lesson.title} | {track.title} | JapaConnect Learn</title>
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-6 sm:px-6">
	<!-- Progress bar at top -->
	<div class="mb-6">
		<div class="flex items-center justify-between text-sm">
			<a
				href="/tracks/{track.slug}"
				class="flex items-center gap-1 font-medium transition-colors hover:opacity-80"
				style="color: var(--color-primary)"
			>
				<ArrowLeft class="h-4 w-4" />
				{track.title}
			</a>
			<span style="color: var(--color-muted)">{progressPercent}% complete</span>
		</div>
		<div class="mt-2 h-1.5 w-full overflow-hidden rounded-full" style="background-color: var(--color-border)">
			<div
				class="h-full rounded-full transition-all"
				style="width: {progressPercent}%; background-color: var(--color-primary)"
			></div>
		</div>
	</div>

	<div class="flex flex-col gap-8 lg:flex-row">
		<!-- Main content -->
		<article class="min-w-0 flex-1">
			<!-- Lesson header -->
			<div class="mb-6">
				<div class="flex items-center gap-2 text-sm" style="color: var(--color-muted)">
					<span>{currentModule.title}</span>
					<span>·</span>
					<span class="flex items-center gap-1">
						<Clock class="h-3.5 w-3.5" />
						{lesson.duration_minutes}m
					</span>
				</div>
				<h1 class="mt-2 text-2xl font-bold tracking-tight sm:text-3xl" style="color: var(--color-foreground)">
					{lesson.title}
				</h1>
			</div>

			<!-- Lesson body -->
			{#if lesson.video_url}
				<div class="mb-6">
					<div class="aspect-video overflow-hidden rounded-xl" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<iframe
							src={getVideoEmbedUrl(lesson.video_url)}
							class="h-full w-full"
							allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
							allowfullscreen
							title={lesson.title}
						></iframe>
					</div>
				</div>
			{/if}

			<div
				class="prose max-w-none"
				style="color: var(--color-foreground)"
			>
				{@html sanitizedBody}
			</div>

			<!-- Quiz section -->
			{#if lesson.quiz && lesson.quiz.length > 0}
				<div class="mt-8 rounded-xl border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
					<h3 class="mb-4 flex items-center gap-2 text-lg font-semibold">
						<BookOpen class="h-5 w-5" style="color: var(--color-primary)" />
						Knowledge Check
					</h3>
					<div class="space-y-6">
						{#each lesson.quiz as question, qi}
							<div>
								<p class="mb-3 text-sm font-medium">{qi + 1}. {question.question}</p>
								<div class="space-y-2">
									{#each question.options as option, oi}
										<label
											class="flex cursor-pointer items-center gap-3 rounded-lg border px-4 py-2.5 text-sm transition-colors"
											style="border-color: {quizSubmitted
												? oi === question.correct_index
													? 'var(--color-success)'
													: quizAnswers[question.id] === oi
														? 'var(--color-danger)'
														: 'var(--color-border)'
												: quizAnswers[question.id] === oi
													? 'var(--color-primary)'
													: 'var(--color-border)'}; background-color: {quizSubmitted
												? oi === question.correct_index
													? 'color-mix(in srgb, var(--color-success) 8%, transparent)'
													: quizAnswers[question.id] === oi
														? 'color-mix(in srgb, var(--color-danger) 8%, transparent)'
														: 'transparent'
												: 'transparent'}"
										>
											<input
												type="radio"
												name="q-{question.id}"
												value={oi}
												disabled={quizSubmitted}
												class="h-4 w-4"
												onchange={() => { quizAnswers[question.id] = oi; }}
											/>
											{option}
										</label>
									{/each}
								</div>
								{#if quizSubmitted && question.explanation}
									<p class="mt-2 text-xs" style="color: var(--color-muted)">{question.explanation}</p>
								{/if}
							</div>
						{/each}
					</div>
					<div class="mt-6 flex items-center gap-4">
						{#if !quizSubmitted}
							<button
								class="flex items-center gap-2 rounded-lg px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
								style="background-color: var(--color-primary)"
								disabled={Object.keys(quizAnswers).length < (lesson.quiz?.length ?? 0) || quizLoading}
								onclick={submitQuiz}
							>
								{#if quizLoading}
									<Loader2 class="h-4 w-4 animate-spin" />
									Submitting...
								{:else}
									<CheckCheck class="h-4 w-4" />
									Submit Quiz
								{/if}
							</button>
							{#if quizError}
								<p class="text-sm" style="color: var(--color-danger)">{quizError}</p>
							{/if}
						{:else if quizResult}
							<div class="flex items-center gap-2 text-sm font-medium" style="color: {quizResult.passed ? 'var(--color-success)' : 'var(--color-danger)'}">
								{#if quizResult.passed}
									<CheckCircle class="h-5 w-5" />
									{quizResult.score}% — Great job! ({quizResult.correct_answers}/{quizResult.total} correct)
								{:else}
									<XCircle class="h-5 w-5" />
									{quizResult.score}% — Try again ({quizResult.correct_answers}/{quizResult.total} correct)
								{/if}
							</div>
						{/if}
					</div>
				</div>
			{/if}

			<!-- Mark complete + navigation -->
			<div class="mt-8 space-y-4">
				{#if !isCompleted}
					<button
						onclick={markComplete}
						disabled={markingComplete}
						class="flex items-center gap-2 rounded-lg px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-success)"
					>
						{#if markingComplete}
							<Loader2 class="h-4 w-4 animate-spin" />
							Saving...
						{:else}
							<CheckCheck class="h-4 w-4" />
							Mark as Complete
						{/if}
					</button>
				{:else}
					<div class="flex items-center gap-2 text-sm font-medium" style="color: var(--color-success)">
						<CheckCircle2 class="h-5 w-5" />
						Lesson completed
					</div>
				{/if}

				<!-- Prev/Next navigation -->
				<div class="flex items-center justify-between border-t pt-4" style="border-color: var(--color-border)">
					{#if prevLesson}
						<a
							href="/tracks/{track.slug}/modules/{prevLesson.moduleSlug}/lessons/{prevLesson.lessonSlug}"
							class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium transition-colors hover:bg-[var(--color-surface)]"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							<ArrowLeft class="h-4 w-4" />
							<span class="max-w-[200px] truncate">{prevLesson.title}</span>
						</a>
					{:else}
						<div></div>
					{/if}
					{#if nextLesson}
						<a
							href="/tracks/{track.slug}/modules/{nextLesson.moduleSlug}/lessons/{nextLesson.lessonSlug}"
							class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
							style="background-color: var(--color-primary)"
						>
							<span class="max-w-[200px] truncate">{nextLesson.title}</span>
							<ArrowRight class="h-4 w-4" />
						</a>
					{:else}
						<a
							href="/tracks/{track.slug}"
							class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-opacity hover:opacity-80"
							style="color: var(--color-primary)"
						>
							Back to track
							<ArrowRight class="h-4 w-4" />
						</a>
					{/if}
				</div>
			</div>
		</article>

		<!-- Sidebar: module lessons list -->
		<aside class="w-full shrink-0 lg:w-72">
			<div class="sticky top-20 rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<h3 class="mb-3 flex items-center gap-2 text-sm font-semibold" style="color: var(--color-foreground)">
					<BookOpen class="h-4 w-4" style="color: var(--color-primary)" />
					{currentModule.title}
				</h3>
				<ul class="space-y-1">
					{#each currentModule.lessons as sl (sl.id)}
						{@const done = isLessonDone(sl.id)}
						{@const isCurrent = sl.slug === lesson.slug}
						<li>
							<a
								href="/tracks/{track.slug}/modules/{currentModule.slug}/lessons/{sl.slug}"
								class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors"
								style="background-color: {isCurrent ? 'color-mix(in srgb, var(--color-primary) 8%, transparent)' : 'transparent'}; color: {isCurrent ? 'var(--color-primary)' : 'var(--color-foreground)'}"
							>
								{#if done}
									<CheckCircle2 class="h-3.5 w-3.5 shrink-0" style="color: var(--color-success)" />
								{:else}
									<Circle class="h-3.5 w-3.5 shrink-0" style="color: var(--color-muted)" />
								{/if}
								<span class="truncate">{sl.title}</span>
								<span class="ml-auto text-xs" style="color: var(--color-muted)">{sl.duration_minutes}m</span>
							</a>
						</li>
					{/each}
				</ul>
			</div>
		</aside>
	</div>
</main>
