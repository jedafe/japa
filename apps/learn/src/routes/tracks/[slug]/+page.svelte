<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import type { TrackDetail } from '$lib/api';
	import { api } from '$lib/api';
	import {
		BookOpen,
		Clock,
		Layers,
		CheckCircle2,
		Circle,
		Play,
		ArrowRight,
		Award,
		ChevronDown,
		ChevronRight,
		Loader2,
		Plus
	} from 'lucide-svelte';

	let { data } = $props();

	let track = $derived(data.track as TrackDetail);
	let progress = $derived(data.progress as { progress: number; completed_lessons: string[] } | null);
	let isEnrolled = $derived(data.isEnrolled as boolean);
	let expandedModules = $state<Set<string>>(new Set());
	let mounted = $state(true);
	let enrolling = $state(false);

	$effect(() => {
		if (track && track.modules.length > 0 && expandedModules.size === 0) {
			expandedModules = new Set([track.modules[0].id]);
		}
	});

	function toggleModule(moduleId: string) {
		const next = new Set(expandedModules);
		if (next.has(moduleId)) {
			next.delete(moduleId);
		} else {
			next.add(moduleId);
		}
		expandedModules = next;
	}

	function isLessonCompleted(lessonId: string): boolean {
		return progress?.completed_lessons?.includes(lessonId) ?? false;
	}

	function getFirstIncompleteLessonUrl(): string {
		for (const mod of track.modules) {
			for (const lesson of mod.lessons) {
				if (!isLessonCompleted(lesson.id)) {
					return `/tracks/${track.slug}/modules/${mod.slug}/lessons/${lesson.slug}`;
				}
			}
		}
		if (track.modules.length > 0 && track.modules[0].lessons.length > 0) {
			const firstMod = track.modules[0];
			return `/tracks/${track.slug}/modules/${firstMod.slug}/lessons/${firstMod.lessons[0].slug}`;
		}
		return '#';
	}

	const completedCount = $derived(progress?.completed_lessons?.length ?? 0);
	const totalLessons = $derived(track.lesson_count);
	const progressPercent = $derived(
		totalLessons > 0 ? Math.round((completedCount / totalLessons) * 100) : 0
	);

	const difficultyColor: Record<string, string> = {
		beginner: 'var(--color-success)',
		intermediate: 'var(--color-warning)',
		advanced: 'var(--color-danger)'
	};

	function formatDifficulty(d: string): string {
		return d.charAt(0).toUpperCase() + d.slice(1);
	}

	async function enroll() {
		enrolling = true;
		try {
			await api.enroll(track.id);
			isEnrolled = true;
		} catch {
			// Handle error silently
		} finally {
			enrolling = false;
		}
	}
</script>

<svelte:head>
	<title>{track.title} | JapaConnect Learn</title>
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8 sm:px-6">
	<!-- Track Header -->
	<section class="rounded-xl border p-6 sm:p-8" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
			<div class="flex-1">
				<div class="flex items-center gap-2">
					<span
						class="rounded-full px-2.5 py-0.5 text-xs font-medium"
						style="background-color: color-mix(in srgb, {difficultyColor[track.difficulty] ?? 'var(--color-muted)'} 15%, transparent); color: {difficultyColor[track.difficulty] ?? 'var(--color-muted)'}"
					>
						{formatDifficulty(track.difficulty)}
					</span>
				</div>
				<h1 class="mt-3 text-2xl font-bold tracking-tight sm:text-3xl" style="color: var(--color-foreground)">
					{track.title}
				</h1>
				<p class="mt-2 max-w-2xl" style="color: var(--color-muted)">
					{track.description}
				</p>

				<div class="mt-4 flex flex-wrap items-center gap-4 text-sm" style="color: var(--color-muted)">
					<span class="flex items-center gap-1.5">
						<Layers class="h-4 w-4" />
						{track.module_count} modules
					</span>
					<span class="flex items-center gap-1.5">
						<BookOpen class="h-4 w-4" />
						{track.lesson_count} lessons
					</span>
					<span class="flex items-center gap-1.5">
						<Clock class="h-4 w-4" />
						{track.estimated_hours}h estimated
					</span>
				</div>

				{#if mounted && progress && totalLessons > 0}
					<div class="mt-5">
						<div class="flex items-center justify-between text-sm">
							<span style="color: var(--color-muted)">{t('tracks.slug.progress')}</span>
							<span class="font-medium" style="color: var(--color-foreground)">{progressPercent}%</span>
						</div>
						<div class="mt-1.5 h-2 w-full overflow-hidden rounded-full" style="background-color: var(--color-border)">
							<div
								class="h-full rounded-full transition-all"
								style="width: {progressPercent}%; background-color: var(--color-primary)"
							></div>
						</div>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{completedCount} of {totalLessons} lessons completed
						</p>
					</div>
				{/if}
			</div>

		<!-- Sidebar actions -->
		<div class="flex flex-row gap-3 lg:flex-col lg:items-end">
			{#if !isEnrolled}
				<button
					class="flex items-center gap-2 rounded-lg px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={enrolling}
					onclick={enroll}
				>
					{#if enrolling}
						<Loader2 class="h-4 w-4 animate-spin" />
						Enrolling...
					{:else}
						<Plus class="h-4 w-4" />
						Enroll in Track
					{/if}
				</button>
			{:else}
				<a
					href={getFirstIncompleteLessonUrl()}
					class="flex items-center gap-2 rounded-lg px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
					style="background-color: var(--color-primary)"
				>
					<Play class="h-4 w-4" />
					{mounted && progress ? 'Continue Learning' : 'Start Learning'}
					<ArrowRight class="h-4 w-4" />
				</a>
			{/if}
			{#if progressPercent === 100}
				<a
					href="/certificates"
					class="flex items-center gap-2 rounded-lg border px-5 py-2.5 text-sm font-medium transition-opacity hover:opacity-80"
					style="border-color: var(--color-success); color: var(--color-success)"
				>
					<Award class="h-4 w-4" />
					View Certificate
				</a>
			{/if}
		</div>
		</div>
	</section>

	<!-- Modules -->
	<section class="mt-8">
		<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('tracks.slug.course_content')}</h2>

		<div class="mt-4 space-y-3">
			{#each track.modules as mod, i (mod.id)}
				{@const isExpanded = expandedModules.has(mod.id)}
				{@const modCompleted = mod.lessons.every((l) => isLessonCompleted(l.id))}
				<div class="rounded-lg border" style="border-color: var(--color-border)">
					<button
						class="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-[var(--color-surface)]"
						onclick={() => toggleModule(mod.id)}
					>
						{#if isExpanded}
							<ChevronDown class="h-4 w-4 shrink-0" style="color: var(--color-muted)" />
						{:else}
							<ChevronRight class="h-4 w-4 shrink-0" style="color: var(--color-muted)" />
						{/if}
						<div class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-medium" style="background-color: var(--color-surface); color: var(--color-muted)">
							{i + 1}
						</div>
						<div class="flex-1">
							<span class="text-sm font-medium" style="color: var(--color-foreground)">{mod.title}</span>
							<span class="ml-2 text-xs" style="color: var(--color-muted)">
								{mod.lessons.length} lessons
							</span>
						</div>
						{#if modCompleted}
							<CheckCircle2 class="h-4 w-4 shrink-0" style="color: var(--color-success)" />
						{/if}
					</button>

					{#if isExpanded}
						<div class="border-t px-4 py-2" style="border-color: var(--color-border)">
							{#if mod.description}
								<p class="py-2 text-xs" style="color: var(--color-muted)">{mod.description}</p>
							{/if}
							<ul class="space-y-1">
								{#each mod.lessons as lesson (lesson.id)}
									{@const completed = isLessonCompleted(lesson.id)}
									<li>
										<a
											href="/tracks/{track.slug}/modules/{mod.slug}/lessons/{lesson.slug}"
											class="flex items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors hover:bg-[var(--color-surface)]"
										>
											{#if completed}
												<CheckCircle2 class="h-4 w-4 shrink-0" style="color: var(--color-success)" />
											{:else}
												<Circle class="h-4 w-4 shrink-0" style="color: var(--color-muted)" />
											{/if}
											<span class="flex-1" style="color: var(--color-foreground)">{lesson.title}</span>
											<span class="text-xs" style="color: var(--color-muted)">{lesson.duration_minutes}m</span>
										</a>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	</section>
</main>
