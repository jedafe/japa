<script lang="ts">
	import { onMount } from 'svelte';
	import { aiApi, aiAdminApi, type HitlReview, type GenerateResult } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { Loader2, Copy, Check, Clock, CheckCircle2, XCircle, Eye, Sparkles, AlertTriangle, Wrench, BarChart3, Activity, Settings } from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { page as pageState } from '$app/state';

	const canGenerate = $derived(hasPermission('ai.context.generate'));
	const canReviewHitl = $derived(hasPermission('ai.context.review'));

	// ─── Generator State ──────────────────────────────────────────
	let prompt = $state('');
	let category = $state('blog_post');
	let model = $state('');
	let generating = $state(false);
	let generateResult = $state<GenerateResult | null>(null);
	let generateError = $state('');
	let copied = $state(false);

	let categories = $state<{ value: string; label: string; description: string }[]>([
		{ value: 'blog_post', label: 'Blog Post', description: 'Generate blog post content' },
		{ value: 'forum_reply', label: 'Forum Reply', description: 'Generate forum reply content' },
		{ value: 'social_media', label: 'Social Media', description: 'Generate social media content' },
		{ value: 'email', label: 'Email', description: 'Generate email content' },
		{ value: 'summary', label: 'Summary', description: 'Generate summary content' },
		{ value: 'translation', label: 'Translation', description: 'Generate translation content' }
	]);

	let models = $state<{ value: string; label: string }[]>([
		{ value: '', label: 'Default' }
	]);

	// Tab navigation (usage is in-memory only, generate/tools follow URL)
	let displayTab = $state<'generate' | 'tools' | 'usage'>('generate');
	let activeTab = $derived.by(() => {
		const path = pageState.url.pathname;
		if (path === '/ai') return displayTab === 'usage' ? 'usage' : 'generate';
		if (path.startsWith('/ai/tools')) return displayTab === 'usage' ? 'usage' : 'tools';
		return displayTab;
	});

	// ─── AI Tools State ──────────────────────────────────────────
	type AiTool = {
		id: string;
		name: string;
		description?: string;
		is_enabled: boolean;
		config?: Record<string, unknown>;
		created_at: string;
	};

	let tools = $state<AiTool[]>([]);
	let toolsLoading = $state(false);
	let toolsError = $state('');

	// Tool edit modal
	let showToolModal = $state(false);
	let editingTool = $state<AiTool | null>(null);
	let toolName = $state('');
	let toolDescription = $state('');
	let toolEnabled = $state(true);
	let toolSaving = $state(false);

	// ─── AI Usage State ──────────────────────────────────────────
	type AiUsage = {
		total_requests: number;
		total_tokens: number;
		total_cost: number;
		models_used: Array<{ model: string; requests: number; tokens: number; cost: number }>;
	};

	type ToolUsage = {
		tool_id: string;
		tool_name: string;
		invocations: number;
		avg_latency_ms: number;
		errors: number;
	};

	let aiUsage = $state<AiUsage | null>(null);
	let toolUsage = $state<ToolUsage[]>([]);
	let usageLoading = $state(false);

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

	// ─── AI Tools Functions ──────────────────────────────────────
	async function loadTools() {
		toolsLoading = true;
		toolsError = '';
		try {
			const result = await aiAdminApi.listTools();
			if (result.ok) {
				tools = (result.data as AiTool[]) ?? [];
			} else {
				toolsError = result.error;
			}
		} catch {
			toolsError = t('common.error');
		} finally {
			toolsLoading = false;
		}
	}

	function openEditTool(tool: AiTool) {
		editingTool = tool;
		toolName = tool.name;
		toolDescription = tool.description || '';
		toolEnabled = tool.is_enabled;
		showToolModal = true;
	}

	async function saveTool() {
		if (!editingTool) return;
		toolSaving = true;
		toolsError = '';
		try {
			const result = await aiAdminApi.updateTool(editingTool.id, {
				name: toolName.trim(),
				description: toolDescription.trim() || undefined,
				is_enabled: toolEnabled
			});
			if (result.ok) {
				showToolModal = false;
				loadTools();
			} else {
				toolsError = result.error;
			}
		} catch {
			toolsError = t('common.error');
		} finally {
			toolSaving = false;
		}
	}

	// ─── AI Usage Functions ──────────────────────────────────────
	async function loadUsage() {
		usageLoading = true;
		try {
			const [usageResult, toolsResult] = await Promise.all([
				aiAdminApi.getAiUsage(),
				aiAdminApi.getToolsUsage()
			]);
			if (usageResult.ok) {
				aiUsage = usageResult.data as AiUsage;
			}
			if (toolsResult.ok) {
				toolUsage = (toolsResult.data as ToolUsage[]) ?? [];
			}
		} catch {
			// silent
		} finally {
			usageLoading = false;
		}
	}

	onMount(async () => {
		try {
			const cats = await aiApi.listCategories();
			if (Array.isArray(cats) && cats.length > 0) {
				categories = cats;
			}
		} catch {
			// Keep default categories
		}

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

		loadReviews();
	});
</script>

<svelte:head>
	<title>AI Core — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('ai.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('ai.description')}</p>

<!-- Sub-navigation tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<a
		href="/ai"
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'generate' ? 'background-color: var(--color-primary); color: white' : ''}
	>
		<Sparkles class="h-4 w-4" />
		{t('ai.generate.title')}
	</a>
	<a
		href="/ai/tools"
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'tools' ? 'background-color: var(--color-primary); color: white' : ''}
	>
		<Wrench class="h-4 w-4" />
		{t('ai.tools.title')}
	</a>
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		onclick={() => { displayTab = 'usage'; loadUsage(); }}
		style={activeTab === 'usage' ? 'background-color: var(--color-primary); color: white' : ''}
	>
		<Activity class="h-4 w-4" />
		{t('ai.usage.title')}
	</button>
</div>

<!-- ═══ Section A: AI Content Generator ═══ -->
{#if activeTab === 'generate'}
	{#if canGenerate}
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
							<option value={cat.value}>{cat.label}</option>
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
	{:else}
		<section class="mb-8 rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="flex items-center gap-3 text-sm" style="color: var(--color-muted)">
				<AlertTriangle class="h-5 w-5" />
				You don't have permission to use AI generation.
			</div>
		</section>
	{/if}

	<!-- ═══ Section B: HITL Review Queue ═══ -->
	{#if canReviewHitl}
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
	{/if}

<!-- ═══ AI Tools Management ═══ -->
{:else if activeTab === 'tools'}
	<section>
		<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold">
			<Wrench class="h-5 w-5" style="color: var(--color-primary)" />
			{t('ai.tools.management')}
		</h2>
		<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('ai.tools.management_description')}</p>

		{#if toolsError}
			<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
				{toolsError}
			</div>
		{/if}

		{#if toolsLoading}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
		{:else}
			<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr style="border-bottom: 1px solid var(--color-border)">
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.tools.name')}</th>
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.tools.description')}</th>
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.tools.status')}</th>
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.tools.created')}</th>
							<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
						</tr>
					</thead>
					<tbody>
						{#each tools as tool (tool.id)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="px-4 py-3 font-medium">{tool.name}</td>
								<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{tool.description || '—'}</td>
								<td class="px-4 py-3">
									<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {tool.is_enabled ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
										{tool.is_enabled ? t('ai.tools.enabled') : t('ai.tools.disabled')}
									</span>
								</td>
								<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(tool.created_at).toLocaleDateString()}</td>
								<td class="px-4 py-3 text-right">
									<button
										class="flex items-center gap-1 rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-primary)"
										onclick={() => openEditTool(tool)}
									>
										<Settings class="h-3 w-3" />
										{t('common.configure')}
									</button>
								</td>
							</tr>
						{:else}
							<tr>
								<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
									{t('ai.tools.empty')}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</section>

<!-- ═══ AI Usage Stats ═══ -->
{:else if activeTab === 'usage'}
	<section>
		<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold">
			<BarChart3 class="h-5 w-5" style="color: var(--color-primary)" />
			{t('ai.usage.title')}
		</h2>

		{#if usageLoading}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
		{:else}
			<!-- AI Usage Overview -->
			{#if aiUsage}
				<div class="mb-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
					<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<p class="text-xs" style="color: var(--color-muted)">{t('ai.usage.total_requests')}</p>
						<p class="mt-1 text-2xl font-bold">{aiUsage.total_requests.toLocaleString()}</p>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<p class="text-xs" style="color: var(--color-muted)">{t('ai.usage.total_tokens')}</p>
						<p class="mt-1 text-2xl font-bold">{aiUsage.total_tokens.toLocaleString()}</p>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<p class="text-xs" style="color: var(--color-muted)">{t('ai.usage.total_cost')}</p>
						<p class="mt-1 text-2xl font-bold">${aiUsage.total_cost.toFixed(2)}</p>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<p class="text-xs" style="color: var(--color-muted)">{t('ai.usage.models_used')}</p>
						<p class="mt-1 text-2xl font-bold">{aiUsage.models_used?.length ?? 0}</p>
					</div>
				</div>

				{#if aiUsage.models_used?.length}
					<h3 class="mb-3 text-sm font-medium">{t('ai.usage.by_model')}</h3>
					<div class="mb-6 overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
						<table class="w-full text-sm">
							<thead>
								<tr style="border-bottom: 1px solid var(--color-border)">
									<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.usage.model')}</th>
									<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.usage.requests')}</th>
									<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.usage.tokens')}</th>
									<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.usage.cost')}</th>
								</tr>
							</thead>
							<tbody>
								{#each aiUsage.models_used as m}
									<tr class="border-t" style="border-color: var(--color-border)">
										<td class="px-4 py-3 font-medium">{m.model}</td>
										<td class="px-4 py-3 text-right">{m.requests.toLocaleString()}</td>
										<td class="px-4 py-3 text-right">{m.tokens.toLocaleString()}</td>
										<td class="px-4 py-3 text-right">${m.cost.toFixed(2)}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			{/if}

			<!-- Tool Usage -->
			<h3 class="mb-3 text-sm font-medium">{t('ai.tools.usage')}</h3>
			<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr style="border-bottom: 1px solid var(--color-border)">
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.tools.name')}</th>
							<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.tools.invocations')}</th>
							<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.tools.avg_latency')}</th>
							<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('ai.tools.errors')}</th>
						</tr>
					</thead>
					<tbody>
						{#each toolUsage as tu (tu.tool_id)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="px-4 py-3 font-medium">{tu.tool_name}</td>
								<td class="px-4 py-3 text-right">{tu.invocations.toLocaleString()}</td>
								<td class="px-4 py-3 text-right">{tu.avg_latency_ms.toFixed(0)}ms</td>
								<td class="px-4 py-3 text-right">
									<span class={tu.errors > 0 ? 'text-red-600 dark:text-red-400' : ''}>{tu.errors}</span>
								</td>
							</tr>
						{:else}
							<tr>
								<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
									{t('ai.tools.no_usage')}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</section>
{/if}

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

<!-- ═══ Tool Edit Modal ═══ -->
<Modal bind:open={showToolModal} title={t('ai.tools.configure')} onConfirm={saveTool} confirmLabel={toolSaving ? t('common.saving') : t('common.save')}>
	<div class="space-y-4">
		<div>
			<label for="tool-name" class="mb-1 block text-sm font-medium">{t('ai.tools.name')}</label>
			<input id="tool-name" type="text" bind:value={toolName} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="tool-desc" class="mb-1 block text-sm font-medium">{t('ai.tools.description')}</label>
			<textarea id="tool-desc" bind:value={toolDescription} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"></textarea>
		</div>
		<div class="flex items-center gap-3">
			<label class="relative inline-flex cursor-pointer items-center">
				<input type="checkbox" bind:checked={toolEnabled} class="peer sr-only" />
				<div class="peer h-5 w-9 rounded-full bg-gray-200 after:absolute after:left-[2px] after:top-[2px] after:h-4 after:w-4 after:rounded-full after:border after:border-gray-300 after:bg-white after:transition-all peer-checked:bg-[var(--color-primary)] peer-checked:after:translate-x-full peer-checked:after:border-white dark:bg-gray-700"></div>
			</label>
			<span class="text-sm font-medium">{t('ai.tools.enabled')}</span>
		</div>
	</div>
</Modal>
