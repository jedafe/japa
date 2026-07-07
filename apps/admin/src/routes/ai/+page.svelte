<script lang="ts">
	import { onMount } from 'svelte';
	import { aiApi, type HitlReview, type GenerateResult } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { Loader2, Copy, Check, Clock, CheckCircle2, XCircle, Eye, Sparkles } from 'lucide-svelte';

	// ─── Generator State ──────────────────────────────────────────
	let prompt = $state('');
	let category = $state('blog_post');
	let model = $state('');
	let generating = $state(false);
	let generateResult = $state<GenerateResult | null>(null);
	let generateError = $state('');
	let copied = $state(false);

	const categories = [
		{ value: 'blog_post', labelKey: 'ai.category.blog_post' },
		{ value: 'forum_reply', labelKey: 'ai.category.forum_reply' },
		{ value: 'social_media', labelKey: 'ai.category.social_media' },
		{ value: 'email', labelKey: 'ai.category.email' },
		{ value: 'summary', labelKey: 'ai.category.summary' },
		{ value: 'translation', labelKey: 'ai.category.translation' }
	];

	let models = $state<{ value: string; label: string }[]>([
		{ value: '', label: 'Default' }
	]);

	onMount(async () => {
		try {
			const res = await aiApi.listModels();
			if (res.ok && res.data.length > 0) {
				models = res.data.map((m) => ({ value: m.id, label: m.label }));
			}
		} catch {
			models = [
				{ value: '', label: 'Default' },
				{ value: 'gpt-4', label: t('ai.model.gpt4') },
				{ value: 'gpt-3.5-turbo', label: t('ai.model.gpt35') },
				{ value: 'claude-3', label: t('ai.model.claude3') }
			];
		}
	});

	// ─── HITL Queue State ─────────────────────────────────────────
	let reviews = $state<HitlReview[]>([]);
	let totalReviews = $state(0);
	let reviewsLoading = $state(true);
	let reviewsError = $state('');
	let page = $state(0);
	const pageSize = 10;

	// Stats
	let pendingCount = $state(0);
	let approvedToday = $state(0);
	let rejectedToday = $state(0);

	// Review modal
	let showReviewModal = $state(false);
	let selectedReview = $state<HitlReview | null>(null);
	let rejectReason = $state('');
	let showRejectInput = $state(false);
	let actionLoading = $state(false);
	let actionMessage = $state('');
	let actionMessageType = $state<'success' | 'error'>('success');

	let totalPages = $derived(Math.max(1, Math.ceil(totalReviews / pageSize)));

	async function handleGenerate() {
		if (!prompt.trim()) return;
		generating = true;
		generateError = '';
		generateResult = null;
		try {
			const result = await aiApi.generate({
				prompt: prompt.trim(),
				category,
				model: model || undefined
			});
			if (result.ok) {
				generateResult = result.data;
			} else {
				generateError = result.error;
			}
		} catch {
			generateError = t('ai.generate_error');
		} finally {
			generating = false;
		}
	}

	async function copyResult() {
		if (!generateResult) return;
		await navigator.clipboard.writeText(generateResult.generated_text);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	async function loadReviews() {
		reviewsLoading = true;
		reviewsError = '';
		try {
			const result = await aiApi.listPendingHitlReviews({
				limit: pageSize,
				offset: page * pageSize
			});
			if (result.ok) {
				reviews = result.data.reviews;
				totalReviews = result.data.total;
				pendingCount = result.data.total;
			} else {
				reviewsError = result.error;
			}
		} catch {
			reviewsError = t('ai.load_error');
		} finally {
			reviewsLoading = false;
		}
	}

	function openReview(review: HitlReview) {
		selectedReview = review;
		rejectReason = '';
		showRejectInput = false;
		actionMessage = '';
		showReviewModal = true;
	}

	async function approveReview() {
		if (!selectedReview) return;
		actionLoading = true;
		actionMessage = '';
		try {
			const result = await aiApi.approveHitlReview(selectedReview.id);
			if (result.ok) {
				actionMessageType = 'success';
				actionMessage = t('ai.approved_success');
				approvedToday++;
				pendingCount = Math.max(0, pendingCount - 1);
				showReviewModal = false;
				loadReviews();
			} else {
				actionMessageType = 'error';
				actionMessage = result.error;
			}
		} catch {
			actionMessageType = 'error';
			actionMessage = t('ai.approve_error');
		} finally {
			actionLoading = false;
		}
	}

	async function rejectReview() {
		if (!selectedReview || !rejectReason.trim()) return;
		actionLoading = true;
		actionMessage = '';
		try {
			const result = await aiApi.rejectHitlReview(selectedReview.id, rejectReason.trim());
			if (result.ok) {
				actionMessageType = 'success';
				actionMessage = t('ai.rejected_success');
				rejectedToday++;
				pendingCount = Math.max(0, pendingCount - 1);
				showReviewModal = false;
				loadReviews();
			} else {
				actionMessageType = 'error';
				actionMessage = result.error;
			}
		} catch {
			actionMessageType = 'error';
			actionMessage = t('ai.reject_error');
		} finally {
			actionLoading = false;
		}
	}

	function changePage(delta: number) {
		page = Math.max(0, Math.min(totalPages - 1, page + delta));
		loadReviews();
	}

	function truncate(text: string, max: number): string {
		if (text.length <= max) return text;
		return text.slice(0, max) + '...';
	}

	onMount(loadReviews);
</script>

<svelte:head>
	<title>AI Core — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('ai.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('ai.description')}</p>

<!-- ═══ Section A: AI Content Generator ═══ -->
<section class="mb-8 rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold">
		<Sparkles class="h-5 w-5" style="color: var(--color-primary)" />
		{t('ai.generator')}
	</h2>

	<div class="space-y-4">
		<div>
			<label for="ai-prompt" class="mb-1 block text-sm font-medium">{t('ai.prompt')}</label>
			<textarea
				id="ai-prompt"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				rows="4"
				placeholder={t('ai.prompt_placeholder')}
				bind:value={prompt}
			></textarea>
		</div>

		<div class="grid gap-4 sm:grid-cols-2">
			<div>
				<label for="ai-category" class="mb-1 block text-sm font-medium">{t('ai.category')}</label>
				<select
					id="ai-category"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={category}
				>
					{#each categories as cat}
						<option value={cat.value}>{t(cat.labelKey)}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="ai-model" class="mb-1 block text-sm font-medium">{t('ai.model_optional')}</label>
				<select
					id="ai-model"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={model}
				>
					{#each models as m}
						<option value={m.value}>{m.label}</option>
					{/each}
				</select>
			</div>
		</div>

		<div class="flex items-center gap-3 pt-2">
			<button
				class="btn btn-primary disabled:opacity-50"
				disabled={generating || !prompt.trim()}
				onclick={handleGenerate}
			>
				{#if generating}
					<Loader2 class="h-4 w-4 animate-spin" />
					{t('ai.generating')}
				{:else}
					<Sparkles class="h-4 w-4" />
					{t('ai.generate')}
				{/if}
			</button>
		</div>

		{#if generateError}
			<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{generateError}
			</div>
		{/if}

		{#if generateResult}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-background)">
				<div class="mb-3 flex items-center justify-between">
					<h3 class="text-sm font-semibold">{t('ai.result')}</h3>
					<button
						class="flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
						style="border-color: var(--color-border)"
						onclick={copyResult}
					>
						{#if copied}
							<Check class="h-3 w-3" style="color: var(--color-success)" />
							{t('ai.copied')}
						{:else}
							<Copy class="h-3 w-3" />
							{t('ai.copy')}
						{/if}
					</button>
				</div>
				<div class="mb-3 whitespace-pre-wrap rounded-md p-3 text-sm" style="background-color: var(--color-surface)">
					{generateResult.generated_text}
				</div>
				<div class="flex flex-wrap gap-4 text-xs" style="color: var(--color-muted)">
					<span>{t('ai.model_used')}: <strong>{generateResult.model}</strong></span>
					<span>{t('ai.tokens_used')}: <strong>{generateResult.tokens_used}</strong></span>
				</div>
			</div>
		{/if}
	</div>
</section>

<!-- ═══ Section B: HITL Review Queue ═══ -->
<section>
	<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold">
		<Eye class="h-5 w-5" style="color: var(--color-primary)" />
		{t('ai.hitl_queue')}
	</h2>
	<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('ai.hitl_description')}</p>

	<!-- Stats Cards -->
	<div class="mb-6 grid gap-4 sm:grid-cols-3">
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="flex items-center gap-3">
				<div class="rounded-full p-2" style="background-color: color-mix(in srgb, var(--color-warning) 12%, transparent)">
					<Clock class="h-4 w-4" style="color: var(--color-warning)" />
				</div>
				<div>
					<p class="text-xs" style="color: var(--color-muted)">{t('ai.pending')}</p>
					<p class="text-xl font-bold">{pendingCount}</p>
				</div>
			</div>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="flex items-center gap-3">
				<div class="rounded-full p-2" style="background-color: color-mix(in srgb, var(--color-success) 12%, transparent)">
					<CheckCircle2 class="h-4 w-4" style="color: var(--color-success)" />
				</div>
				<div>
					<p class="text-xs" style="color: var(--color-muted)">{t('ai.approved_today')}</p>
					<p class="text-xl font-bold">{approvedToday}</p>
				</div>
			</div>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="flex items-center gap-3">
				<div class="rounded-full p-2" style="background-color: color-mix(in srgb, var(--color-danger) 12%, transparent)">
					<XCircle class="h-4 w-4" style="color: var(--color-danger)" />
				</div>
				<div>
					<p class="text-xs" style="color: var(--color-muted)">{t('ai.rejected_today')}</p>
					<p class="text-xl font-bold">{rejectedToday}</p>
				</div>
			</div>
		</div>
	</div>

	{#if reviewsError}
		<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{reviewsError}
		</div>
	{/if}

	{#if reviewsLoading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else if reviews.length === 0}
		<div class="rounded-lg border py-12 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<Eye class="mx-auto mb-3 h-8 w-8" style="color: var(--color-muted)" />
			<p class="text-sm" style="color: var(--color-muted)">{t('ai.no_pending')}</p>
		</div>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-color: var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.content')}</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.type')}</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.source')}</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.created')}</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.actions')}</th>
					</tr>
				</thead>
				<tbody>
					{#each reviews as review}
						<tr class="border-t" style="border-color: var(--color-border)">
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{truncate(review.content, 80)}</td>
							<td class="px-4 py-3">
								<span class="inline-block rounded-full bg-purple-500/15 px-2 py-0.5 text-xs text-purple-700 dark:text-purple-400">
									{review.content_type}
								</span>
							</td>
							<td class="px-4 py-3 text-xs">{review.source}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(review.created_at).toLocaleDateString()}</td>
							<td class="px-4 py-3">
								<button
									class="rounded px-2 py-1 text-xs font-medium hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openReview(review)}
								>
									{t('ai.review')}
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<!-- Pagination -->
		<div class="mt-3 flex items-center justify-between">
			<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
			<div class="flex gap-1">
				<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page === 0} onclick={() => changePage(-1)}>&larr;</button>
				<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => changePage(1)}>&rarr;</button>
			</div>
		</div>
	{/if}
</section>

<!-- ═══ Review Modal ═══ -->
<Modal bind:open={showReviewModal} title={t('ai.review_title')}>
	{#if selectedReview}
		{#if actionMessage}
			<div class="mb-3 rounded-md border px-3 py-2 text-sm" style="border-color: {actionMessageType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'}; background-color: color-mix(in srgb, {actionMessageType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'} 8%, transparent); color: {actionMessageType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'}">
				{actionMessage}
			</div>
		{/if}

		<div class="space-y-3">
			<div class="flex flex-wrap gap-2 text-xs">
				<span class="inline-block rounded-full bg-purple-500/15 px-2 py-0.5 text-purple-700 dark:text-purple-400">
					{selectedReview.content_type}
				</span>
				<span class="inline-block rounded-full px-2 py-0.5" style="background-color: color-mix(in srgb, var(--color-info) 12%, transparent); color: var(--color-info)">
					{selectedReview.source}
				</span>
				<span style="color: var(--color-muted)">
					{new Date(selectedReview.created_at).toLocaleString()}
				</span>
			</div>

			<div class="max-h-64 overflow-y-auto rounded-md p-3 text-sm" style="background-color: var(--color-surface)">
				{selectedReview.content}
			</div>

			{#if showRejectInput}
				<div>
					<label for="reject-reason" class="mb-1 block text-sm font-medium">{t('ai.reject_reason')}</label>
					<textarea
						id="reject-reason"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						rows="3"
						placeholder={t('ai.reject_reason_placeholder')}
						bind:value={rejectReason}
					></textarea>
				</div>
			{/if}

			<div class="flex gap-2 pt-2">
				<button
					class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-success)"
					disabled={actionLoading}
					onclick={approveReview}
				>
					{#if actionLoading}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{:else}
						<CheckCircle2 class="h-3.5 w-3.5" />
					{/if}
					{t('ai.approve')}
				</button>
				{#if !showRejectInput}
					<button
						class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors"
						style="background-color: var(--color-danger)"
						onclick={() => (showRejectInput = true)}
					>
						<XCircle class="h-3.5 w-3.5" />
						{t('ai.reject')}
					</button>
				{:else}
					<button
						class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors disabled:opacity-50"
						style="background-color: var(--color-danger)"
						disabled={actionLoading || !rejectReason.trim()}
						onclick={rejectReview}
					>
						{#if actionLoading}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{/if}
						{t('ai.reject')}
					</button>
				{/if}
			</div>
		</div>
	{/if}
</Modal>
