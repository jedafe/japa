<script lang="ts">
	import { GraduationCap, BookOpen, Clock, ArrowRight, Users } from 'lucide-svelte';

	type Track = {
		id: string;
		title: string;
		description: string;
		lessons: number;
		duration: string;
		enrolled: number;
		level: 'Beginner' | 'Intermediate' | 'Advanced';
	};

	const tracks: Track[] = [
		{
			id: 'relocation-basics',
			title: 'Relocation Basics',
			description: 'Everything you need to plan your move abroad — visas, budgeting, and timelines.',
			lessons: 12,
			duration: '3h 20m',
			enrolled: 1240,
			level: 'Beginner'
		},
		{
			id: 'student-pathway',
			title: 'Student Pathway',
			description: 'Admissions, funding, and settling in as an international student.',
			lessons: 18,
			duration: '5h 10m',
			enrolled: 890,
			level: 'Beginner'
		},
		{
			id: 'skilled-worker',
			title: 'Skilled Worker Track',
			description: 'Job search strategies, credential evaluation, and work visa sponsorship.',
			lessons: 15,
			duration: '4h 05m',
			enrolled: 1560,
			level: 'Intermediate'
		},
		{
			id: 'entrepreneur-route',
			title: 'Entrepreneur Route',
			description: 'Business visas, startup ecosystems, and building abroad as a founder.',
			lessons: 10,
			duration: '2h 45m',
			enrolled: 320,
			level: 'Advanced'
		}
	];

	const levelColor: Record<Track['level'], string> = {
		Beginner: 'var(--color-success)',
		Intermediate: 'var(--color-warning)',
		Advanced: 'var(--color-danger)'
	};
</script>

<svelte:head>
	<title>JapaConnect Learn</title>
</svelte:head>

<main class="mx-auto max-w-6xl px-6 py-8">
	<section class="rounded-xl p-8" style="background-color: var(--color-surface)">
		<h1 class="text-3xl font-bold tracking-tight">JapaConnect Learn</h1>
		<p class="mt-2 max-w-2xl" style="color: var(--color-muted)">
			Structured learning tracks for your journey abroad. From first steps to advanced pathways,
			guided by community experts.
		</p>
	</section>

	<section id="tracks" class="mt-10">
		<div class="flex items-center gap-2">
			<GraduationCap class="h-5 w-5" style="color: var(--color-primary)" />
			<h2 class="text-xl font-semibold">Learning Tracks</h2>
		</div>

		<div class="mt-6 grid gap-4 sm:grid-cols-2">
			{#each tracks as track (track.id)}
				<article
					class="group rounded-lg border p-6 transition-all hover:shadow-md"
					style="border-color: var(--color-border)"
				>
					<div class="flex items-start justify-between">
						<div class="flex items-center gap-3">
							<div
								class="rounded-md p-2"
								style="background-color: var(--color-surface)"
							>
								<BookOpen class="h-5 w-5" style="color: var(--color-primary)" />
							</div>
							<div>
								<h3 class="font-semibold">{track.title}</h3>
								<span
									class="text-xs font-medium"
									style="color: {levelColor[track.level]}"
								>
									{track.level}
								</span>
							</div>
						</div>
					</div>

					<p class="mt-3 text-sm" style="color: var(--color-muted)">
						{track.description}
					</p>

					<div class="mt-4 flex items-center gap-4 text-xs" style="color: var(--color-muted)">
						<span class="flex items-center gap-1">
							<BookOpen class="h-3.5 w-3.5" />
							{track.lessons} lessons
						</span>
						<span class="flex items-center gap-1">
							<Clock class="h-3.5 w-3.5" />
							{track.duration}
						</span>
						<span class="flex items-center gap-1">
							<Users class="h-3.5 w-3.5" />
							{track.enrolled.toLocaleString()} enrolled
						</span>
					</div>

					<a
						href="/{track.id}"
						class="mt-4 flex items-center gap-1 text-sm font-medium transition-all group-hover:gap-2"
						style="color: var(--color-primary)"
					>
						Start track
						<ArrowRight class="h-4 w-4" />
					</a>
				</article>
			{/each}
		</div>
	</section>
</main>
