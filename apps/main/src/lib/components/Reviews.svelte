<script lang="ts">
	import { Star, Loader2, Flag, ChevronLeft, ChevronRight, MessageSquare } from 'lucide-svelte';
	import { api, type Review } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let { entityType, entityId }: { entityType: string; entityId: string } = $props();

	let aggregate = $state<{ average: number; count: number; distribution: number[] } | null>(null);
	let reviews = $state<Review[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state('');
	let sort = $state<'newest' | 'highest' | 'lowest'>('newest');
	let currentPage = $state(0);
	const pageSize = 10;

	// Form state
	let showForm = $state(false);
	let formRating = $state(0);
	let formTitle = $state('');
	let formBody = $state('');
	let formLoading = $state(false);
	let formError = $state('');

	// Flag state
	let flaggingId = $state<string | null>(null);
	let flagReason = $state('');
	let flagLoading = $state(false);

	$effect(() => {
		loadAggregate();
		loadReviews();
	});

	async function loadAggregate() {
		try {
			aggregate = await api.getReviewAggregate({ entity_type: entityType, entity_id: entityId });
		} catch {
			// aggregate is optional
		}
	}

	async function loadReviews() {
		loading = true;
		error = '';
		try {
			const result = await api.listReviews({
				entity_type: entityType,
				entity_id: entityId,
				limit: pageSize,
				offset: currentPage * pageSize
			});
			reviews = result.reviews;
			total = result.total;
		} catch {
			error = 'Failed to load reviews.';
		} finally {
			loading = false;
		}
	}

	async function submitReview(e: Event) {
		e.preventDefault();
		if (formRating === 0) {
			formError = 'Please select a rating.';
			return;
		}
		formLoading = true;
		formError = '';
		try {
			await api.createReview({
				entity_type: entityType,
				entity_id: entityId,
				rating: formRating,
				title: formTitle.trim() || undefined,
				body: formBody.trim() || undefined
			});
			showForm = false;
			formRating = 0;
			formTitle = '';
			formBody = '';
			await loadReviews();
			await loadAggregate();
		} catch (err) {
			formError = err instanceof Error ? err.message : 'Failed to submit review.';
		} finally {
			formLoading = false;
		}
	}

	async function submitFlag(reviewId: string) {
		if (!flagReason.trim()) return;
		flagLoading = true;
		try {
			await api.flagReview(reviewId, { reason: flagReason.trim() });
			flaggingId = null;
			flagReason = '';
		} catch {
			// ignore flag errors silently
		} finally {
			flagLoading = false;
		}
	}

	function prevPage() {
		if (currentPage > 0) {
			currentPage--;
			loadReviews();
		}
	}

	function nextPage() {
		if ((currentPage + 1) * pageSize < total) {
			currentPage++;
			loadReviews();
		}
	}

	const totalPages = $derived(Math.ceil(total / pageSize));

	function renderStars(rating: number): string {
		return '★'.repeat(rating) + '☆'.repeat(5 - rating);
	}

	function formatDate(dateStr: string): string {
		const d = new Date(dateStr);
		return d.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
	}
</script>

<section class="mt-6 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<h2 class="flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
			<MessageSquare class="h-5 w-5" style="color: var(--color-primary)" />
			{t('reviews.title')}
		</h2>
		{#if !showForm}
			<button
				onclick={() => (showForm = true)}
				class="rounded-lg px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
				style="background-color: var(--color-primary)"
			>
				{t('reviews.write')}
			</button>
		{/if}
	</div>

	<!-- Aggregate Rating -->
	{#if aggregate && aggregate.count > 0}
		<div class="mt-4 flex items-center gap-4 rounded-lg p-4" style="background-color: var(--color-background);">
			<div class="text-center">
				<div class="text-3xl font-bold" style="color: var(--color-foreground)">
					{aggregate.average.toFixed(1)}
				</div>
				<div class="mt-1 text-sm" style="color: var(--color-muted)">
					{aggregate.count} {aggregate.count === 1 ? 'review' : 'reviews'}
				</div>
			</div>
			<div class="flex-1">
				{#each aggregate.distribution as count, i}
					{@const pct = aggregate.count > 0 ? (count / aggregate.count) * 100 : 0}
					<div class="flex items-center gap-2 text-xs">
						<span class="w-6 text-right" style="color: var(--color-muted)">{5 - i}★</span>
						<div class="h-2 flex-1 overflow-hidden rounded-full" style="background-color: var(--color-border);">
							<div
								class="h-full rounded-full transition-all"
								style="width: {pct}%; background-color: var(--color-primary);"
							></div>
						</div>
						<span class="w-8" style="color: var(--color-muted)">{count}</span>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Review Form -->
	{#if showForm}
		<form onsubmit={submitReview} class="mt-4 space-y-3 rounded-lg p-4" style="background-color: var(--color-background);">
			<fieldset>
				<legend class="mb-1 text-sm font-medium" style="color: var(--color-foreground)">{t('reviews.rating')}</legend>
				<div class="flex gap-1" role="radiogroup" aria-label="Rating">
					{#each [1, 2, 3, 4, 5] as star}
						<button
							type="button"
							onclick={() => (formRating = star)}
							class="text-2xl transition-transform hover:scale-110"
							style="color: {star <= formRating ? 'var(--color-warning)' : 'var(--color-border)'}"
							aria-label="{star} star{star !== 1 ? 's' : ''}"
							role="radio"
							aria-checked={star === formRating}
						>
							★
						</button>
					{/each}
				</div>
			</fieldset>
			<div>
				<label for="review_title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('reviews.title_optional')}</label>
				<input
					id="review_title"
					type="text"
					bind:value={formTitle}
					placeholder={t('reviews.summarize')}
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				/>
			</div>
			<div>
				<label for="review_body" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('reviews.review_optional')}</label>
				<textarea
					id="review_body"
					bind:value={formBody}
					rows="3"
					placeholder={t('reviews.share_details')}
					class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
				></textarea>
			</div>
			{#if formError}
				<p class="text-sm" style="color: var(--color-danger)">{formError}</p>
			{/if}
			<div class="flex justify-end gap-2">
				<button
					type="button"
					onclick={() => { showForm = false; formError = ''; }}
					class="rounded-lg px-3 py-1.5 text-sm font-medium"
					style="color: var(--color-muted)"
				>
					{t('common.cancel')}
				</button>
				<button
					type="submit"
					disabled={formLoading || formRating === 0}
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
				>
					{#if formLoading}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{/if}
					{t('reviews.submit')}
				</button>
			</div>
		</form>
	{/if}

	<!-- Sort Controls -->
	{#if total > 0}
		<div class="mt-4 flex items-center gap-2 text-sm">
			<span style="color: var(--color-muted)">{t('reviews.sort_by')}</span>
			{#each [['newest', t('reviews.newest')], ['highest', t('reviews.highest')], ['lowest', t('reviews.lowest')]] as [value, label]}
				<button
					onclick={() => { sort = value as 'newest' | 'highest' | 'lowest'; loadReviews(); }}
					class="rounded-full px-2.5 py-0.5 text-xs font-medium transition-colors"
					style="background-color: {sort === value ? 'var(--color-primary)' : 'var(--color-background)'}; color: {sort === value ? 'white' : 'var(--color-muted)'}"
				>
					{label}
				</button>
			{/each}
		</div>
	{/if}

	<!-- Reviews List -->
	{#if loading}
		<div class="mt-4 space-y-3">
			{#each [1, 2, 3] as _}
				<div class="h-24 animate-pulse rounded-lg" style="background-color: var(--color-background)"></div>
			{/each}
		</div>
	{:else if error}
		<div class="mt-4 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{:else if reviews.length === 0}
		<div class="mt-4 py-8 text-center">
			<Star class="mx-auto mb-2 h-8 w-8" style="color: var(--color-muted)" />
			<p class="text-sm" style="color: var(--color-muted)">{t('reviews.none')} {t('reviews.none_msg')}</p>
		</div>
	{:else}
		<div class="mt-4 space-y-3">
			{#each reviews as review (review.id)}
				<div class="rounded-lg p-4" style="background-color: var(--color-background);">
					<div class="flex items-start justify-between">
						<div class="flex items-center gap-3">
							{#if review.author_avatar}
								<img src={review.author_avatar} alt="" class="h-8 w-8 rounded-full object-cover" />
							{:else}
								<div
									class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold text-white"
									style="background-color: var(--color-primary)"
								>
									{review.author_name.charAt(0).toUpperCase()}
								</div>
							{/if}
							<div>
								<span class="text-sm font-medium" style="color: var(--color-foreground)">
									{review.author_name}
								</span>
								<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
									<span style="color: var(--color-warning)">{renderStars(review.rating)}</span>
									<span>&middot;</span>
									<time datetime={review.created_at}>{formatDate(review.created_at)}</time>
								</div>
							</div>
						</div>
						<button
							onclick={() => (flaggingId = flaggingId === review.id ? null : review.id)}
							class="rounded p-1 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="color: var(--color-muted)"
							aria-label="Flag review"
						>
							<Flag class="h-3.5 w-3.5" />
						</button>
					</div>
					{#if review.title}
						<p class="mt-2 text-sm font-medium" style="color: var(--color-foreground)">{review.title}</p>
					{/if}
					{#if review.body}
						<p class="mt-1 text-sm" style="color: var(--color-foreground)">{review.body}</p>
					{/if}

					{#if flaggingId === review.id}
						<div class="mt-3 space-y-2 rounded-lg p-3" style="background-color: var(--color-surface);">
							<input
								type="text"
								bind:value={flagReason}
								placeholder={t('reviews.flag_reason')}
								class="w-full rounded-lg border px-3 py-1.5 text-sm outline-none focus:border-[var(--color-primary)]"
								style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
							/>
							<div class="flex justify-end gap-2">
								<button
									onclick={() => { flaggingId = null; flagReason = ''; }}
									class="rounded px-2 py-1 text-xs font-medium"
									style="color: var(--color-muted)"
								>
									{t('common.cancel')}
								</button>
								<button
									onclick={() => submitFlag(review.id)}
									disabled={flagLoading || !flagReason.trim()}
									class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
									style="background-color: var(--color-danger)"
								>
									{#if flagLoading}
										<Loader2 class="h-3 w-3 animate-spin" />
									{/if}
									{t('reviews.flag')}
								</button>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>

		<!-- Pagination -->
		{#if totalPages > 1}
			<div class="mt-4 flex items-center justify-center gap-2">
				<button
					onclick={prevPage}
					disabled={currentPage === 0}
					class="flex items-center gap-1 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-50"
					style="border-color: var(--color-border); color: var(--color-foreground)"
				>
					<ChevronLeft class="h-4 w-4" />
					{t('reviews.prev')}
				</button>
				<span class="text-sm" style="color: var(--color-muted)">
					{currentPage + 1} of {totalPages}
				</span>
				<button
					onclick={nextPage}
					disabled={(currentPage + 1) >= totalPages}
					class="flex items-center gap-1 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-50"
					style="border-color: var(--color-border); color: var(--color-foreground)"
				>
					{t('common.next')}
					<ChevronRight class="h-4 w-4" />
				</button>
			</div>
		{/if}
	{/if}
</section>
