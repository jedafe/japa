<script lang="ts">
	// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	// One card on the public board: vote pill + title link to the detail page,
	// summary, category chip, target window, flagship pin. Moderators
	// (roadmap.moderate) get an inline status select that hits the admin
	// status endpoint — there is no separate public mutation (§113).
	import { Pin } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import VoteButton from './VoteButton.svelte';
	import StatusSelect from './StatusSelect.svelte';
	import type { RoadmapBoardItem } from '$lib/types';

	interface Props {
		item: RoadmapBoardItem;
		authed: boolean;
		canModerate?: boolean;
	}
	let { item, authed, canModerate = false }: Props = $props();
</script>

<article
	class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40"
>
	<div class="flex gap-3">
		<VoteButton {item} {authed} />

		<div class="min-w-0 flex-1">
			<div class="flex items-start justify-between gap-2">
				<a
					href="/roadmap/{item.slug}"
					class="text-sm font-semibold leading-snug text-[var(--color-text)] hover:text-[var(--color-primary)]"
				>
					{item.title}
				</a>
				{#if item.is_flagship}
					<span
						class="shrink-0 text-[var(--color-primary)]"
						title={t('roadmap.flagship', 'Flagship')}
						aria-label={t('roadmap.flagship', 'Flagship')}
					>
						<Pin size={14} />
					</span>
				{/if}
			</div>

			{#if item.summary}
				<p class="mt-1 line-clamp-2 text-xs text-[var(--color-text-subtle)]">{item.summary}</p>
			{/if}

			<div class="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-[var(--color-muted)]">
				{#if item.category_name}
					<span class="inline-flex items-center gap-1">
						{#if item.category_color}
							<span
								class="inline-block h-2 w-2 rounded-full"
								style:background-color={item.category_color}
							></span>
						{/if}
						{item.category_name}
					</span>
				{/if}
				{#if item.target_window}
					<span
						class="rounded-full border border-[var(--color-border)] px-1.5 py-px"
						title={t('roadmap.target_window', 'Target')}
					>
						{item.target_window}
					</span>
				{/if}
			</div>
		</div>
	</div>

	{#if canModerate}
		<div class="mt-2 flex justify-end border-t border-[var(--color-border)]/60 pt-2">
			<StatusSelect itemId={item.id} status={item.status} />
		</div>
	{/if}
</article>
