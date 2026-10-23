<script lang="ts">
	// One service review row — shared by the detail-page reviews section
	// (ServiceReviews, where the first row renders highlighted) and the
	// standalone /services/[slug]/reviews index. Caller wraps in a <ul>.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { Star } from 'lucide-svelte';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { t } from '$lib/i18n';
	import type { ServiceReview } from '$lib/types';

	interface Props {
		review: ServiceReview;
		highlight?: boolean;
	}
	let { review, highlight = false }: Props = $props();

	function authorName(r: ServiceReview): string {
		return (
			r.author_display_name ?? r.author_username ?? t('services.anonymous_reviewer', 'Anonymous')
		);
	}

	function formatDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString();
		} catch {
			return iso;
		}
	}
</script>

<li
	class="rounded-[var(--radius-card)] border bg-[var(--color-surface)] p-4 {highlight
		? 'border-[var(--color-primary)]/30'
		: 'border-[var(--color-border)]'}"
>
	<div class="flex items-center justify-between gap-2">
		<div class="flex items-center gap-2">
			{#if review.author_username}
				<UserHoverCard
					id={review.author_id}
					slug={review.author_username}
					name={review.author_display_name ?? review.author_username}
					avatar={review.author_avatar_url}
					variant="row"
					class="inline-flex max-w-full items-center text-start text-sm font-medium text-[var(--color-text)] hover:underline"
				>
					{#snippet trigger()}
						<span>{authorName(review)}</span>
					{/snippet}
				</UserHoverCard>
			{:else}
				<span class="text-sm font-medium text-[var(--color-text)]">{authorName(review)}</span>
			{/if}
			<span class="flex items-center gap-0.5 text-[var(--color-warning)]">
				{#each Array(5) as _, i (i)}
					<Star
						size={12}
						fill={i < review.rating ? 'currentColor' : 'none'}
						class={i < review.rating ? '' : 'text-[var(--color-muted)]'}
						aria-hidden="true"
					/>
				{/each}
			</span>
		</div>
		<span class="text-xs text-[var(--color-muted)]">{formatDate(review.created_at)}</span>
	</div>
	{#if review.title}
		<p class="mt-2 text-sm font-medium text-[var(--color-text)]">{review.title}</p>
	{/if}
	{#if review.body}
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{review.body}</p>
	{/if}
</li>
