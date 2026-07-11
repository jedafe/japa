<script lang="ts">
	import type { Enrollment, Track } from '$lib/api';
	import { api } from '$lib/api';
	import { GraduationCap, BookOpen, Clock, ArrowRight, Award } from 'lucide-svelte';
	import { Badge, Button, Spinner } from '$lib/components/ui';

	let { data } = $props();

	let enrollments = $derived(data.enrollments as Enrollment[]);
	let tracks = $state<Track[]>([]);
	let loading = $state(true);

	import { onMount } from 'svelte';

	onMount(async () => {
		try {
			const allTracks = await api.listTracks();
			tracks = allTracks;
		} catch {
			tracks = [];
		}
		loading = false;
	});

	function getTrack(trackId: string): Track | undefined {
		return tracks.find((t) => t.id === trackId);
	}

	const difficultyColor: Record<string, string> = {
		beginner: 'var(--color-success)',
		intermediate: 'var(--color-warning)',
		advanced: 'var(--color-danger)'
	};
</script>

<svelte:head>
	<title>Dashboard | JapaConnect Learn</title>
</svelte:head>

<main class="mx-auto max-w-6xl px-4 py-8 sm:px-6">
	<h1 class="mb-2 text-2xl font-bold tracking-tight">My Learning Dashboard</h1>
	<p class="mb-8 text-sm" style="color: var(--color-muted)">Track your progress across all enrolled learning tracks.</p>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Spinner size="lg" />
		</div>
	{:else if enrollments.length === 0}
		<div class="rounded-2xl border px-6 py-16 text-center" style="border-color: var(--color-border); background: var(--gradient-surface)">
			<div
				class="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl"
				style="background: color-mix(in srgb, var(--color-muted) 10%, transparent)"
			>
				<GraduationCap class="h-8 w-8" style="color: var(--color-muted)" />
			</div>
			<p class="mt-4 text-lg font-semibold" style="color: var(--color-foreground)">No enrollments yet</p>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">Enroll in a learning track to get started.</p>
			<Button variant="primary" class="mt-6" onclick={() => window.location.href = '/'}>
				Browse Tracks
				<ArrowRight class="h-4 w-4" />
			</Button>
		</div>
	{:else}
		<div class="space-y-4">
			{#each enrollments as enrollment (enrollment.id)}
				{@const track = getTrack(enrollment.track_id)}
					{#if track}
					<a
						href="/tracks/{track.slug}"
						class="card-interactive group block rounded-xl border p-5 transition-colors hover:bg-[var(--color-surface)]"
						style="border-color: var(--color-border); background-color: var(--color-background)"
					>
						<div class="flex items-center gap-4">
							{#if track.thumbnail_url}
								<img src={track.thumbnail_url} alt="" class="h-14 w-14 rounded-xl object-cover" />
							{:else}
								<div
									class="flex h-14 w-14 items-center justify-center rounded-xl"
									style="background: var(--gradient-primary)"
								>
									<BookOpen class="h-7 w-7 text-white" />
								</div>
							{/if}
							<div class="flex-1 min-w-0">
								<div class="flex items-center gap-2">
									<h3 class="font-semibold" style="color: var(--color-foreground)">{track.title}</h3>
									<Badge>{track.difficulty}</Badge>
								</div>
								<div class="mt-1 flex items-center gap-3 text-xs" style="color: var(--color-muted)">
									<span class="flex items-center gap-1">
										<BookOpen class="h-3 w-3" />
										{track.lesson_count} lessons
									</span>
									<span class="flex items-center gap-1">
										<Clock class="h-3 w-3" />
										{track.estimated_hours}h
									</span>
									<span>Enrolled {new Date(enrollment.enrolled_at).toLocaleDateString()}</span>
								</div>
								{#if enrollment.progress_pct != null && enrollment.progress_pct > 0}
									<div class="mt-2 flex items-center gap-2">
										<div class="h-1.5 flex-1 overflow-hidden rounded-full" style="background: var(--color-surface)">
											<div
												class="h-full rounded-full transition-all"
												style="width: {Math.min(enrollment.progress_pct, 100)}%; background: var(--color-primary)"
											></div>
										</div>
										<span class="text-[10px] font-medium" style="color: var(--color-muted)">
											{Math.round(enrollment.progress_pct)}%
										</span>
									</div>
								{/if}
							</div>
							<ArrowRight class="h-4 w-4 shrink-0 transition-transform group-hover:translate-x-0.5" style="color: var(--color-muted)" />
						</div>
					</a>
				{/if}
			{/each}
		</div>
	{/if}
</main>
