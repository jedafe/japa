<script lang="ts">
	// Ref: japaconnect-engineering/sections/29-ratings-reviews.md (list display)
	// Renders a list of reviews for an entity. Each item shows the star fill,
	// title, body, and a relative date. The list is fetched by the parent
	// (server load) and passed in — this component is presentational only.
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Star, MessageSquare } from 'lucide-svelte';
	import type { Review } from '$lib/types';

	interface Props {
		reviews: Review[];
		/** Optional callback when a review is deleted (owner-only). */
		onDelete?: (id: string) => void | Promise<void>;
		/** Show the delete button for reviews by this user ID (owner-only). */
		currentUserId?: string;
	}
	let { reviews, onDelete, currentUserId }: Props = $props();

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}
</script>

{#if reviews.length === 0}
	<EmptyState
		icon={MessageSquare}
		title={t('reviews.no_reviews')}
		body={t('reviews.empty_body')}
	/>
{:else}
	<ul class="space-y-3">
		{#each reviews as r (r.id)}
			<li class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="mb-1.5 flex items-center justify-between gap-2">
					<div class="flex items-center gap-1" aria-label={`${t('reviews.rating_aria')}: ${r.rating}`}>
						{#each Array(5) as _, i (i)}
							<Star
								class="h-3.5 w-3.5 {i < r.rating ? 'fill-amber-400 text-amber-400' : 'text-[var(--color-muted)]'}"
								aria-hidden="true"
							/>
						{/each}
					</div>
					<div class="flex items-center gap-2">
						<span class="text-xs text-[var(--color-muted)]">{formatDate(r.created_at)}</span>
						{#if onDelete && currentUserId && r.reviewer_user_id === currentUserId}
							<button
								type="button"
								onclick={() => onDelete(r.id)}
								class="text-xs text-[var(--color-muted)] hover:text-red-500"
								aria-label={t('reviews.delete')}
							>
								{t('reviews.delete')}
							</button>
						{/if}
					</div>
				</div>
				{#if r.title}
					<p class="text-sm font-medium text-[var(--color-text)]">{r.title}</p>
				{/if}
				{#if r.body}
					<p class="mt-1 whitespace-pre-wrap text-sm text-[var(--color-muted)]">{r.body}</p>
				{/if}
			</li>
		{/each}
	</ul>
{/if}
