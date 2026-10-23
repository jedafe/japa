<script lang="ts">
	// Ref: japaconnect-engineering/adr/ADR-064a-geo-awareness-extension.md §5
	// Dashboard widget displaying geo-aware tool, feature, content, and space
	// recommendations. Data is fetched server-side by the overview dashboard
	// loader (Rule 16 — no hardcoded/mock data). Renders nothing when the
	// recommendations endpoint is unavailable (null) or returns empty arrays.
	import { t } from '$lib/i18n';
	import { Wrench, Sparkles, FileText, Users } from 'lucide-svelte';
	import type {
		RecommendationsResponse,
		RecommendationItem,
		RecommendationContentItem,
		RecommendationSpaceItem
	} from '$lib/types';

	interface Props {
		data: RecommendationsResponse | null;
	}
	let { data }: Props = $props();

	const hasTools = $derived(!!data && data.tools.length > 0);
	const hasFeatures = $derived(!!data && data.features.length > 0);
	const hasContent = $derived(!!data && data.content.length > 0);
	const hasSpaces = $derived(!!data && data.spaces.length > 0);
	const hasAny = $derived(hasTools || hasFeatures || hasContent || hasSpaces);
</script>

{#if data && hasAny}
	<section class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<header class="mb-3">
			<h2 class="text-sm font-medium text-[var(--color-text)]">{t('dashboard.overview.recommendations')}</h2>
			{#if data.context.archetype}
				<p class="mt-0.5 text-xs text-[var(--color-muted)]">
					{data.context.archetype.replace(/_/g, ' ')}
					{#if data.context.japa_stage}
						· {data.context.japa_stage.replace(/_/g, ' ')}
					{/if}
				</p>
			{/if}
		</header>

		<div class="space-y-4">
			{#if hasTools}
				<div>
					<div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-[var(--color-muted)]">
						<Wrench size={12} />
						{t('dashboard.overview.recommendations_tools')}
					</div>
					<ul class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
						{#each data.tools as tool (tool.slug)}
							<li>
								<a
									href={tool.url}
									class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface)]
									border border-transparent"
								>
									<span class="min-w-0 flex-1 truncate">{tool.label}</span>
									<span class="ms-2 shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-muted)]">{tool.reason}</span>
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			{#if hasFeatures}
				<div>
					<div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-[var(--color-muted)]">
						<Sparkles size={12} />
						{t('dashboard.overview.recommendations_features')}
					</div>
					<ul class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
						{#each data.features as feature (feature.slug)}
							<li>
								<a
									href={feature.url}
									class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface)]
									border border-transparent"
								>
									<span class="min-w-0 flex-1 truncate">{feature.label}</span>
									<span class="ms-2 shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-muted)]">{feature.reason}</span>
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			{#if hasContent}
				<div>
					<div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-[var(--color-muted)]">
						<FileText size={12} />
						{t('dashboard.overview.recommendations_content')}
					</div>
					<ul class="space-y-1.5">
						{#each data.content as item (item.id)}
							<li>
								<a
									href={item.url}
									class="block truncate rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface)]"
								>
									{item.title}
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			{#if hasSpaces}
				<div>
					<div class="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-[var(--color-muted)]">
						<Users size={12} />
						{t('dashboard.overview.recommendations_spaces')}
					</div>
					<ul class="space-y-1.5">
						{#each data.spaces as space (space.id)}
							<li>
								<a
									href="/spaces/{space.slug}"
									class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface)]"
								>
									<span class="min-w-0 flex-1 truncate">{space.name}</span>
									{#if space.level}
										<span class="ms-2 shrink-0 text-[10px] uppercase tracking-wide text-[var(--color-muted)]">{space.level}</span>
									{/if}
								</a>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
	</section>
{:else if data !== null}
	<!-- Recommendations endpoint returned but empty — show nothing (not an error state) -->
{:else}
	<!-- Recommendations endpoint unavailable — silently skip (best-effort) -->
{/if}
