<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type TrackDetail, type Lesson } from '$lib/api';
	import {
		CheckCircle2,
		Circle,
		ArrowLeft,
		ArrowRight,
		BookOpen,
		Clock,
		CheckCheck,
		Loader2
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

	onMount(async () => {
		try {
			const progress = await api.getProgress(track.id);
			completedLessons = new Set(progress.completed_lessons ?? []);
			const total = track.lesson_count;
			const completed = progress.completed_lessons?.length ?? 0;
			progressPercent = total > 0 ? Math.round((completed / total) * 100) : 0;
		} catch {
			// No progress yet
		}
		isCompleted = completedLessons.has(lesson.id);
	});

	async function markComplete() {
		markingComplete = true;
		try {
			await api.updateProgress(lesson.id, { completed: true });
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
			<div
				class="prose max-w-none"
				style="color: var(--color-foreground)"
			>
				{@html lesson.body}
			</div>

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
