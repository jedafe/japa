<script lang="ts">
	// Paths — curated learning paths (collections of tracks).
	// CSR-only; the backend paths endpoint is /learn/paths (if available).
	// Degrades gracefully if the endpoint is not yet wired.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { LoadingSkeleton, EmptyState } from '$lib/components/ui';
	import { Layers } from 'lucide-svelte';
	import type { TrackSummary } from '$lib/types';

	interface LearningPath {
		id: string;
		slug: string;
		title: string;
		description?: string;
		tracks: TrackSummary[];
	}

	let paths = $state<LearningPath[]>([]);
	let loading = $state(true);

	$effect(() => {
		(async () => {
			const r = await api.get<LearningPath[]>('/learn/paths');
			loading = false;
			if (r.data) paths = r.data;
		})();
	});
</script>

<svelte:head>
	<title>{t('learn.paths')} · {t('app.name')}</title>
</svelte:head>

<div class="px-4 py-6 md:px-6 lg:px-8">
	<header class="mb-6">
		<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
			<Layers size={24} class="text-[var(--color-primary)]" />
			{t('learn.paths')}
		</h1>
	</header>

	{#if loading}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
			{#each Array(2) as _}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<LoadingSkeleton class="h-6 w-3/4" />
					<LoadingSkeleton class="mt-2 h-4 w-full" />
					<LoadingSkeleton class="mt-3 h-20 w-full" />
				</div>
			{/each}
		</div>
	{:else if paths.length === 0}
		<EmptyState title={t('common.empty')} description="No learning paths published yet." />
	{:else}
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
			{#each paths as path (path.id)}
				<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
					<h2 class="text-lg font-semibold text-[var(--color-text)]">{path.title}</h2>
					{#if path.description}
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{path.description}</p>
					{/if}
					<ul class="mt-3 space-y-2">
						{#each path.tracks as track (track.id)}
							<li>
								<a
									href="/tracks/{track.slug}"
									class="flex items-center gap-2 rounded-[var(--radius)] p-2 hover:bg-[var(--color-surface-subtle)]"
								>
									{#if track.thumbnail_url}
										<img src={track.thumbnail_url} alt="" class="h-10 w-14 rounded object-cover" />
									{/if}
									<span class="flex-1 truncate text-sm font-medium text-[var(--color-text)]">{track.title}</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/each}
		</div>
	{/if}
</div>
