<script lang="ts">
	import type { Track } from '$lib/api';
	import { GraduationCap, BookOpen, Clock, ArrowRight, Layers } from 'lucide-svelte';

	let { data } = $props();
	let tracks = $derived(data.tracks as Track[]);

	const difficultyColor: Record<string, string> = {
		beginner: 'var(--color-success)',
		intermediate: 'var(--color-warning)',
		advanced: 'var(--color-danger)'
	};

	function formatDifficulty(d: string): string {
		return d.charAt(0).toUpperCase() + d.slice(1);
	}
</script>

<svelte:head>
	<title>Learning Tracks | JapaConnect Learn</title>
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8 sm:px-6">
	<!-- Hero -->
	<section class="relative overflow-hidden rounded-2xl p-6 sm:p-8" style="background: var(--gradient-surface); border: 1px solid var(--color-border)">
		<!-- Background decoration -->
		<div
			class="absolute -right-10 -top-10 h-40 w-40 rounded-full opacity-[0.07]"
			style="background: var(--gradient-primary)"
		></div>

		<div class="relative flex items-center gap-4">
			<div
				class="flex h-14 w-14 items-center justify-center rounded-2xl shadow-sm"
				style="background: var(--gradient-primary)"
			>
				<GraduationCap class="h-7 w-7 text-white" />
			</div>
			<div>
				<h1 class="text-2xl font-bold tracking-tight sm:text-3xl">Learn & Grow</h1>
				<p class="mt-1 max-w-2xl text-sm leading-relaxed sm:text-base" style="color: var(--color-muted)">
					Structured learning tracks for your journey abroad. From first steps to advanced pathways, guided by community experts.
				</p>
			</div>
		</div>
	</section>

	<!-- Tracks -->
	<section class="mt-8">
		<div class="flex items-center gap-2">
			<Layers class="h-5 w-5" style="color: var(--color-primary)" />
			<h2 class="text-xl font-semibold tracking-tight">Learning Tracks</h2>
		</div>

	{#if tracks.length === 0}
			<div class="mt-6 rounded-2xl border px-6 py-16 text-center" style="border-color: var(--color-border); background: var(--gradient-surface)">
				<div
					class="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl"
					style="background: color-mix(in srgb, var(--color-muted) 10%, transparent)"
				>
					<GraduationCap class="h-8 w-8" style="color: var(--color-muted)" />
				</div>
				<p class="mt-4 text-lg font-semibold" style="color: var(--color-foreground)">No tracks available yet</p>
				<p class="mt-1 text-sm" style="color: var(--color-muted)">Check back soon for new learning content.</p>
			</div>
		{:else}
			<div class="mt-6 grid gap-4 sm:grid-cols-2">
				{#each tracks as track, i (track.id)}
					<a
						href="/tracks/{track.slug}"
						class="card-interactive group block rounded-2xl border p-6"
						style="border-color: var(--color-border); background: var(--color-surface); animation: slide-up 0.3s ease-out {i * 75}ms both"
					>
						<div class="flex items-start justify-between">
							<div class="flex items-center gap-3">
								{#if track.thumbnail_url}
									<img
										src={track.thumbnail_url}
										alt=""
										class="h-12 w-12 rounded-xl object-cover shadow-sm"
									/>
								{:else}
									<div
										class="flex h-12 w-12 items-center justify-center rounded-xl shadow-sm"
										style="background: var(--gradient-primary)"
									>
										<BookOpen class="h-6 w-6 text-white" />
									</div>
								{/if}
								<div>
									<h3 class="font-semibold leading-snug" style="color: var(--color-foreground)">{track.title}</h3>
									<span
										class="mt-0.5 inline-block text-xs font-semibold capitalize"
										style="color: {difficultyColor[track.difficulty] ?? 'var(--color-muted)'}"
									>
										{formatDifficulty(track.difficulty)}
									</span>
								</div>
							</div>
						</div>

						<p class="mt-3 text-sm leading-relaxed" style="color: var(--color-muted)">
							{track.description}
						</p>

						<div class="mt-4 flex items-center gap-4 text-xs font-medium" style="color: var(--color-muted)">
							<span class="flex items-center gap-1">
								<Layers class="h-3.5 w-3.5" />
								{track.module_count} modules
							</span>
							<span class="flex items-center gap-1">
								<BookOpen class="h-3.5 w-3.5" />
								{track.lesson_count} lessons
							</span>
							<span class="flex items-center gap-1">
								<Clock class="h-3.5 w-3.5" />
								{track.estimated_hours}h
							</span>
						</div>

						<div
							class="mt-4 flex items-center gap-1 text-sm font-semibold transition-all group-hover:gap-2"
							style="color: var(--color-primary)"
						>
							Start track
							<ArrowRight class="h-4 w-4 transition-transform group-hover:translate-x-0.5" />
						</div>
					</a>
				{/each}
			</div>
		{/if}
	</section>
</main>
