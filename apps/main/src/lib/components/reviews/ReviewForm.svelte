<script lang="ts">
	// Ref: japaconnect-engineering/sections/29-ratings-reviews.md (create form)
	// Inline create-review form. The parent passes entityType + entityId; the
	// form posts to /reviews/{entity_type}/{entity_id} via the browser API
	// client and emits `onCreated` with the new review so the parent can
	// prepend it to the list and refresh the aggregate.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Star, Send } from 'lucide-svelte';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import type { Review } from '$lib/types';

	interface Props {
		entityType: string;
		entityId: string;
		onCreated?: (review: Review) => void;
	}
	let { entityType, entityId, onCreated }: Props = $props();

	let rating = $state(0);
	let hoverRating = $state(0);
	let title = $state('');
	let body = $state('');
	let submitting = $state(false);
	let errorKey = $state('');

	function setRating(n: number) {
		rating = n;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (rating < 1 || rating > 5) {
			errorKey = 'reviews.errors.rating_required';
			return;
		}
		// Guests can't submit reviews — redirect to login instead of a 401.
		if (!(await requireAuthOrRedirect())) return;
		submitting = true;
		try {
			const res = await api.post<Review>(
				`/reviews/${entityType}/${entityId}`,
				{ rating, title: title || undefined, body: body || undefined }
			);
			if (res.data) {
				onCreated?.(res.data);
				rating = 0;
				title = '';
				body = '';
			} else if (res.error?.code === 'self_review') {
				errorKey = 'reviews.errors.self_review';
			} else {
				errorKey = 'reviews.errors.submit_failed';
			}
		} catch {
			errorKey = 'reviews.errors.submit_failed';
		} finally {
			submitting = false;
		}
	}
</script>

<form onsubmit={submit} class="space-y-3 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4" novalidate>
	<h3 class="text-sm font-medium text-[var(--color-text)]">{t('reviews.write_title')}</h3>

	<div class="flex items-center gap-1">
		<span class="me-2 text-xs text-[var(--color-muted)]">{t('reviews.your_rating')}</span>
		{#each Array(5) as _, i (i)}
			{@const n = i + 1}
			<button
				type="button"
				onclick={() => setRating(n)}
				onmouseenter={() => (hoverRating = n)}
				onmouseleave={() => (hoverRating = 0)}
				class="p-0.5"
				aria-label={`${t('reviews.set_rating')}: ${n}`}
			>
				<Star
					class="h-5 w-5 transition {(hoverRating || rating) >= n ? 'fill-[var(--color-warning)] text-[var(--color-warning)]' : 'text-[var(--color-muted)] hover:text-[var(--color-warning)]'}"
					aria-hidden="true"
				/>
			</button>
		{/each}
	</div>

	<input
		value={title}
		oninput={(e) => (title = e.currentTarget.value)}
		placeholder={t('reviews.title_placeholder')}
		class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
	/>

	<textarea
		value={body}
		oninput={(e) => (body = e.currentTarget.value)}
		rows={3}
		placeholder={t('reviews.body_placeholder')}
		class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
	></textarea>

	{#if errorKey}
		<p class="text-sm text-[var(--color-danger)]" role="alert">{t(errorKey)}</p>
	{/if}

	<div class="flex justify-end">
		<button
			type="submit"
			disabled={submitting || rating < 1}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
		>
			<Send class="h-4 w-4 rtl:-scale-x-100" />
			{submitting ? '...' : t('reviews.submit')}
		</button>
	</div>
</form>
