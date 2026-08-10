<script lang="ts">
	// AchievementWidget — sidebar widget for Space pages showing the latest
	// 3–5 achievements filed against the space's country. Fetches from
	// GET /api/v1/spaces/:slug/achievements?limit=5 (§89 / ADR-097).
	// Renders only when the achievement feature is enabled and the space
	// has at least one achievement — empty state is hidden to avoid
	// cluttering the sidebar.
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Trophy, Heart, ArrowRight } from 'lucide-svelte';
	import type { Achievement } from '$lib/types';

	interface Props {
		spaceSlug: string;
		/** Hide the widget when there are no achievements (default true). */
		hideWhenEmpty?: boolean;
	}
	let { spaceSlug, hideWhenEmpty = true }: Props = $props();

	let achievements = $state<Achievement[]>([]);
	let loading = $state(true);
	let loadError = $state(false);

	onMount(async () => {
		const res = await api.get<Achievement[]>(`/spaces/${spaceSlug}/achievements?limit=5`);
		if (res.error) {
			// 404 means the feature is disabled — hide the widget silently.
			if (res.error.status !== 404) loadError = true;
		} else if (res.data) {
			achievements = res.data;
		}
		loading = false;
	});

	const milestoneLabel = (m?: string) => {
		if (!m) return '';
		const key = `achievements.type_${m}`;
		return t(key, m.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()));
	};
</script>

{#if !loading && achievements.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<Trophy size={16} class="text-[var(--color-primary)]" />
			{t('achievements.widget_heading', 'Recent Achievements')}
		</h2>
		<ul class="space-y-2.5">
			{#each achievements as a (a.id)}
				<li>
					<a
						href={`/achievements/${a.slug}`}
						class="group block rounded-[var(--radius)] p-2 transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<p class="line-clamp-1 text-sm font-medium text-[var(--color-text)]">{a.title}</p>
						<div class="mt-1 flex items-center gap-2 text-xs text-[var(--color-text-subtle)]">
							{#if a.milestone_type}
								<span class="inline-flex items-center gap-0.5">
									<Trophy size={11} />
									{milestoneLabel(a.milestone_type)}
								</span>
							{/if}
							{#if a.reaction_count > 0}
								<span class="inline-flex items-center gap-0.5">
									<Heart size={11} class="text-[var(--color-primary)]" fill="currentColor" />
									{a.reaction_count}
								</span>
							{/if}
						</div>
					</a>
				</li>
			{/each}
		</ul>
		<a
			href={`/achievements?dest=${encodeURIComponent(spaceSlug)}`}
			class="mt-3 flex items-center justify-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			{t('achievements.widget_view_all', 'View all')}
			<ArrowRight size={12} />
		</a>
	</section>
{:else if !loading && !hideWhenEmpty && !loadError && achievements.length === 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
			<Trophy size={16} class="text-[var(--color-primary)]" />
			{t('achievements.widget_heading', 'Recent Achievements')}
		</h2>
		<p class="text-xs text-[var(--color-muted)]">{t('achievements.widget_empty', 'No achievements yet for this destination.')}</p>
	</section>
{/if}
