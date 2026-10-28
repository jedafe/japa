<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { aiApi, aiAdminApi, aiModelsAdminApi, type HitlReview, type HitlVerifyResult, type GenerateResult, type AiTool, type AiUsageSummary, type AiToolUsageStats } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Loader2, Copy, Check, Clock, CheckCircle2, XCircle, Eye, Sparkles, AlertTriangle, Wrench, BarChart3, Activity, Settings, Trash2, Search } from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Select } from '$lib/components/ui/select';
	import { page as pageState } from '$app/state';
	import { goto } from '$app/navigation';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';
	import { Switch } from '$lib/components/ui/switch';

	const canGenerate = $derived(hasPermission('ai.generate'));
	const canReviewHitl = $derived(hasPermission('ai.hitl.review'));

	// ─── Generator State ──────────────────────────────────────────
	let prompt = $state('');
	let category = $state('blog_post');
	let model = $state('');
	let generating = $state(false);
	let generateResult = $state<GenerateResult | null>(null);
	let generateError = $state('');
	let copied = $state(false);

	let categories = $state<{ value: string; label: string; description: string }[]>([]);
	let categoriesLoading = $state(false);

	let models = $state<{ value: string; label: string }[]>([
		{ value: '', label: t('ai.model.default', 'Default') }
	]);

	// Tab navigation (generate/tools are path-scoped routes — /ai and
	// /ai/tools — while usage/models are in-memory tabs on /ai). The in-memory
	// tabs are deep-linkable via ?tab=; ?tab=tools redirects to /ai/tools.
	const AI_TABS = ['generate', 'tools', 'usage', 'models'] as const;
	let displayTab = $state<'generate' | 'tools' | 'usage' | 'models'>(initTabParam('generate', { valid: AI_TABS }));
	bindTabParam(() => displayTab, (v) => (displayTab = v), { valid: AI_TABS });
	$effect(() => {
		if (displayTab === 'tools' && pageState.url.pathname === '/ai') void goto('/ai/tools');
	});
	let activeTab = $derived.by(() => {
		const path = pageState.url.pathname;
		if (path === '/ai') {
			if (displayTab === 'usage') return 'usage';
			if (displayTab === 'models') return 'models';
			return 'generate';
		}
		if (path.startsWith('/ai/tools')) {
			if (displayTab === 'usage') return 'usage';
			if (displayTab === 'models') return 'models';
			return 'tools';
		}
		return displayTab;
	});

	// ─── AI Tools State ──────────────────────────────────────────
	let tools = $state<AiTool[]>([]);
	let toolsLoading = $state(false);
	let toolsError = $state('');

	// Tool edit slide-over
	let showToolSlideOver = $state(false);
	let editingTool = $state<AiTool | null>(null);
	let toolName = $state('');
	let toolDescription = $state('');
	let toolEnabled = $state(true);
	let toolSaving = $state(false);

	// ─── AI Usage State ──────────────────────────────────────────
	let aiUsage = $state<AiUsageSummary | null>(null);
	let toolUsage = $state<AiToolUsageStats[]>([]);
	let usageLoading = $state(false);

	// ─── HITL Queue State ─────────────────────────────────────────
	let reviews = $state<HitlReview[]>([]);
	let totalReviews = $state(0);
	let reviewsLoading = $state(true);
	let reviewsError = $state('');
	let page = $state(0);
	let pageSize = $state(10);

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

	// HITL verify state
	let verifyLoading = $state(false);
	let verifyResult = $state<HitlVerifyResult | null>(null);
	let verifyError = $state('');

	let hitlSearchQuery = $state('');
	let toolsSearchQuery = $state('');
	let usageSearchQuery = $state('');
	let modelsSearchQuery = $state('');

	const hitlColumns = [
		{ key: 'content', label: t('ai.content'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${truncate(v, 80)}</span>` },
		{ key: 'content_type', label: t('ai.type'), render: (v: string) => `<span class="inline-block rounded-full bg-info/15 px-2 py-0.5 text-xs text-info dark:text-info">${v}</span>` },
		{ key: 'source', label: t('ai.source'), render: (v: string) => `<span class="text-xs">${v}</span>` },
		{ key: 'created_at', label: t('ai.created'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` }
	];

	const hitlRowActions: ActionItem[] = [
		{ label: t('ai.review'), icon: Eye, onclick: (row: HitlReview) => openReview(row) }
	];

	const toolsMgmtColumns = [
		{ key: 'name', label: t('ai.tools.name') },
		{ key: 'description', label: t('ai.tools.description'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'is_enabled', label: t('ai.tools.status'), render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('ai.tools.enabled') : t('ai.tools.disabled')}</span>` },
		{ key: 'created_at', label: t('ai.tools.created'), render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${new Date(v).toLocaleDateString()}</span>` }
	];

	const toolsMgmtRowActions: ActionItem[] = [
		{ label: t('common.configure', 'Configure'), icon: Settings, onclick: (row: AiTool) => openEditTool(row) },
		{ label: t('common.delete', 'Delete'), icon: Trash2, variant: 'destructive', onclick: (row: AiTool) => confirmDeleteTool(row) }
	];

	const modelUsageColumns = [
		{ key: 'task_type', label: t('ai.usage.model', 'Task Type'), render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: 'calls', label: t('ai.usage.requests'), render: (v: number) => v.toLocaleString() },
		{ key: 'tokens_used', label: t('ai.usage.tokens'), render: (v: number) => v.toLocaleString() },
		{ key: 'total_cost_usd', label: t('ai.usage.cost'), render: (v: number) => `$${(v ?? 0).toFixed(2)}` }
	];

	const toolUsageColumns = [
		{ key: 'tool_name', label: t('ai.tools.name'), render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: 'invocations', label: t('ai.tools.invocations'), render: (v: number) => v.toLocaleString() },
		{ key: 'tokens_used', label: t('ai.usage.tokens'), render: (v: number) => v.toLocaleString() },
		{ key: 'total_cost_usd', label: t('ai.usage.cost'), render: (v: number) => `$${(v ?? 0).toFixed(2)}` }
	];

	const modelsColumns = [
		{ key: 'label', label: t('ai.models.name', 'Name'), render: (v: string) => `<span class="font-medium">${v}</span>` },
		{ key: 'id', label: t('ai.models.id', 'ID'), render: (v: string) => v ? `<span class="font-mono text-xs">${v}</span>` : `<span class="text-xs" style="color: var(--color-muted)">auto</span>` }
	];

	const filteredHitlReviews = $derived(
		hitlSearchQuery
			? reviews.filter((r) => r.content.toLowerCase().includes(hitlSearchQuery.toLowerCase()) || r.source.toLowerCase().includes(hitlSearchQuery.toLowerCase()))
			: reviews
	);

	const filteredTools = $derived(
		toolsSearchQuery
			? tools.filter((t) => t.name.toLowerCase().includes(toolsSearchQuery.toLowerCase()))
			: tools
	);

	const filteredModelUsage = $derived(
		usageSearchQuery
			? (aiUsage?.by_task_type ?? []).filter((m) => m.task_type.toLowerCase().includes(usageSearchQuery.toLowerCase()))
			: (aiUsage?.by_task_type ?? [])
	);

	const filteredToolUsage = $derived(
		usageSearchQuery
			? toolUsage.filter((t) => t.tool_name.toLowerCase().includes(usageSearchQuery.toLowerCase()))
			: toolUsage
	);

	let aiModels = $state<{ id: string; label: string }[]>([]);

	const filteredModels = $derived(
		modelsSearchQuery
			? aiModels.filter((m) => (m.label || m.id).toLowerCase().includes(modelsSearchQuery.toLowerCase()))
			: aiModels
	);

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
		verifyResult = null;
		verifyError = '';
		verifyLoading = false;
		showReviewModal = true;
	}

	async function verifyReview() {
		if (!selectedReview) return;
		verifyLoading = true;
		verifyResult = null;
		verifyError = '';
		try {
			const result = await aiApi.verifyHitlClaim({
				claim: selectedReview.content,
				context: `content_type=${selectedReview.content_type}, source=${selectedReview.source}`
			});
			if (result.ok) {
				verifyResult = result.data;
			} else {
				verifyError = result.error;
			}
		} catch (e) {
			verifyError = e instanceof Error ? e.message : 'Verification failed';
		} finally {
			verifyLoading = false;
		}
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
				tools = result.data ?? [];
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
		showToolSlideOver = true;
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
				showToolSlideOver = false;
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

	// Delete tool state
	let showDeleteConfirm = $state(false);
	let deletingTool = $state<AiTool | null>(null);
	let toolDeleting = $state(false);

	function confirmDeleteTool(tool: AiTool) {
		deletingTool = tool;
		showDeleteConfirm = true;
	}

	async function deleteTool() {
		if (!deletingTool) return;
		toolDeleting = true;
		toolsError = '';
		try {
			const result = await aiAdminApi.deleteTool(deletingTool.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deletingTool = null;
				loadTools();
			} else {
				toolsError = result.error;
			}
		} catch {
			toolsError = t('common.error');
		} finally {
			toolDeleting = false;
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
				aiUsage = usageResult.data;
			}
			if (toolsResult.ok) {
				toolUsage = toolsResult.data ?? [];
			}
		} catch {
			// silent
		} finally {
			usageLoading = false;
		}
	}

	let modelsLoading = $state(false);
	async function loadModels() {
		modelsLoading = true;
		try {
			const res = await aiModelsAdminApi.list();
			if (res.ok) {
				aiModels = res.data ?? [];
			}
		} catch {
			// silent
		} finally {
			modelsLoading = false;
		}
	}

	// Lazy-load the in-memory tabs on activation — covers clicks, ?tab=
	// deep-links, and back/forward navigation.
	let usageLoaded = false;
	let modelsLoaded = false;
	$effect(() => {
		if (displayTab === 'usage' && !usageLoaded) {
			usageLoaded = true;
			void loadUsage();
		} else if (displayTab === 'models' && !modelsLoaded) {
			modelsLoaded = true;
			void loadModels();
		}
	});

	$effect(() => {
		(async () => {
			categoriesLoading = true;
			try {
				const cats = await aiApi.listCategories();
				if (cats.ok && Array.isArray(cats.data)) {
					categories = cats.data;
				}
			} catch {
				// Rule 16: no hardcoded category fallback. If the API fails,
				// the categories selector stays empty.
			} finally {
				categoriesLoading = false;
			}

			try {
				const res = await aiApi.listModels();
				if (res.ok && res.data.length > 0) {
					models = res.data.map((m) => ({ value: m.id, label: m.label }));
				}
			} catch {
				// Rule 16: no hardcoded model fallback. If the API fails,
				// keep the initial "Default"-only entry (set at init).
			}

			loadReviews();
		})();
	});
</script>

<svelte:head>
	<title>AI Core — {siteName()} Admin</title>
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
		onclick={() => { loadTools(); }}
	>
		<Wrench class="h-4 w-4" />
		{t('ai.tools.title')}
	</a>
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		onclick={() => { displayTab = 'usage'; }}
		style={activeTab === 'usage' ? 'background-color: var(--color-primary); color: white' : ''}
	>
		<Activity class="h-4 w-4" />
		{t('ai.usage.title')}
	</button>
	<button
		class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		onclick={() => { displayTab = 'models'; }}
		style={activeTab === 'models' ? 'background-color: var(--color-primary); color: white' : ''}
	>
		<Settings class="h-4 w-4" />
		{t('ai.models.title', 'Models')}
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
				<p class="mt-1 block text-xs" style="color: var(--color-muted)">{t('ai.ai_prompt_hint', 'The instruction sent to the AI.')}</p>
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
					<p class="mt-1 block text-xs" style="color: var(--color-muted)">{t('ai.ai_category_hint', 'The category — drives grouping and filters.')}</p>
					<Select options={categories} bind:value={category} />
				</div>
				<div>
					<label for="ai-model" class="mb-1 block text-sm font-medium">{t('ai.model_optional')}</label>
					<p class="mt-1 block text-xs" style="color: var(--color-muted)">{t('ai.ai_model_hint', 'The AI model to use.')}</p>
					<Select options={models} bind:value={model} />
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
							class="flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
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
				{t('ai.no_permission', "You don't have permission to use AI generation.")}
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
		{:else}
			<DataTable
				columns={hitlColumns}
				data={filteredHitlReviews}
				searchable={true}
				onsearch={(q: string) => { hitlSearchQuery = q; }}
				pageSize={10}
				actions={hitlRowActions}
				emptyMessage={t('ai.no_pending')}
			/>

			<!-- Pagination -->
			<div class="mt-3 flex items-center justify-between">
				<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
				<div class="flex gap-1">
					<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-surface/10 disabled:opacity-40" disabled={page === 0} onclick={() => changePage(-1)}>&larr;</button>
					<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-surface/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => changePage(1)}>&rarr;</button>
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
			<DataTable
				columns={toolsMgmtColumns}
				data={filteredTools}
				searchable={true}
				onsearch={(q: string) => { toolsSearchQuery = q; }}
				pageSize={25}
				actions={toolsMgmtRowActions}
				emptyMessage={t('ai.tools.empty')}
			/>
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
						<p class="mt-1 text-2xl font-bold">{(aiUsage.by_task_type ?? []).reduce((s, r) => s + (r.calls ?? 0), 0).toLocaleString()}</p>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<p class="text-xs" style="color: var(--color-muted)">{t('ai.usage.total_tokens')}</p>
						<p class="mt-1 text-2xl font-bold">{(aiUsage.tokens_used ?? 0).toLocaleString()}</p>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<p class="text-xs" style="color: var(--color-muted)">{t('ai.usage.total_cost')}</p>
						<p class="mt-1 text-2xl font-bold">{(aiUsage.total_cost_usd ?? 0).toFixed(2)}</p>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
						<p class="text-xs" style="color: var(--color-muted)">{t('ai.usage.models_used')}</p>
						<p class="mt-1 text-2xl font-bold">{(aiUsage.by_task_type ?? []).length}</p>
					</div>
				</div>

				{#if aiUsage.by_task_type?.length}
					<h3 class="mb-3 text-sm font-medium">{t('ai.usage.by_model')}</h3>
					<div class="mb-6">
						<DataTable
							columns={modelUsageColumns}
							data={filteredModelUsage}
							searchable={true}
							onsearch={(q: string) => { usageSearchQuery = q; }}
							pageSize={25}
							emptyMessage={t('ai.no_usage_data')}
							showPagination={false}
						/>
					</div>
				{/if}
			{/if}

			<!-- Tool Usage -->
			<h3 class="mb-3 text-sm font-medium">{t('ai.tools.usage')}</h3>
			<DataTable
				columns={toolUsageColumns}
				data={filteredToolUsage}
				pageSize={25}
				emptyMessage={t('ai.tools.no_usage')}
				showPagination={false}
			/>
		{/if}
	</section>
{/if}

<!-- ═══ Section D: AI Models (read-only) ═══ -->
{#if activeTab === 'models'}
	<section class="mb-8">
		{#if modelsLoading}
			<div class="flex items-center justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-muted)" />
			</div>
		{:else}
			<DataTable
				columns={modelsColumns}
				data={filteredModels}
				searchable={true}
				onsearch={(q: string) => { modelsSearchQuery = q; }}
				pageSize={25}
				emptyMessage={t('ai.models.empty', 'No AI models configured')}
				showPagination={false}
			/>
		{/if}
	</section>
{/if}

<!-- ═══ Review Modal ═══ -->
<Modal bind:open={showReviewModal} title={t('ai.review_title')}>
	{#if selectedReview}
		{#if actionMessage}
			<div class="mb-3 rounded-md border px-3 py-2 text-sm {actionMessageType === 'success' ? 'border-success bg-success-50 text-success-700' : 'border-danger bg-danger-50 text-danger-700'}">
				{actionMessage}
			</div>
		{/if}

		<div class="space-y-3">
			<div class="flex flex-wrap gap-2 text-xs">
				<span class="inline-block rounded-full bg-info/15 px-2 py-0.5 text-info dark:text-info">
					{selectedReview.content_type}
				</span>
				<span class="inline-block rounded-full bg-info-100 px-2 py-0.5 text-info-700">
					{selectedReview.source}
				</span>
				<span class="text-muted">
					{new Date(selectedReview.created_at).toLocaleString()}
				</span>
			</div>

			<div class="max-h-64 overflow-y-auto rounded-md bg-surface p-3 text-sm text-foreground-alt">
				{selectedReview.content}
			</div>

			{#if verifyError}
				<div class="rounded-md border border-danger bg-danger-50 px-3 py-2 text-sm text-danger-700">
					{verifyError}
				</div>
			{/if}

			{#if verifyResult}
				<div class="space-y-2 rounded-md border border-info bg-info-50 p-3 text-sm">
					<div class="font-medium text-info-900">{t('ai.verified_with', 'Verified with')} {verifyResult.provider}</div>
					<div class="max-h-48 overflow-y-auto text-info-800">{verifyResult.answer}</div>
					{#if verifyResult.citations?.length > 0}
						<div class="pt-1">
							<div class="text-xs font-medium text-info-900">{t('ai.sources', 'Sources')}:</div>
							<ul class="mt-1 list-inside list-disc space-y-1 text-xs text-info-800">
								{#each verifyResult.citations as citation}
									<li>
										<a href={citation} target="_blank" rel="noopener noreferrer" class="hover:underline">{citation}</a>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>
			{/if}

			{#if showRejectInput}
				<div>
					<label for="reject-reason" class="mb-1 block text-sm font-medium text-foreground-alt">{t('ai.reject_reason')}</label>
					<p class="mt-1 block text-xs" style="color: var(--color-muted)">{t('ai.reject_reason_hint', 'The reason recorded — shown to the user where noted.')}</p>
					<textarea
						id="reject-reason"
						class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						rows="3"
						placeholder={t('ai.reject_reason_placeholder')}
						bind:value={rejectReason}
					></textarea>
				</div>
			{/if}

			<div class="flex flex-wrap gap-2 pt-2">
				<Button
					variant="default"
					disabled={actionLoading}
					onclick={approveReview}
				>
					{#if actionLoading}
						<Loader2 class="h-3.5 w-3.5 animate-spin" />
					{:else}
						<CheckCircle2 class="h-3.5 w-3.5" />
					{/if}
					{t('ai.approve')}
				</Button>
				{#if !showRejectInput}
					<Button
						variant="outline"
						disabled={verifyLoading}
						onclick={verifyReview}
					>
						{#if verifyLoading}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<Search class="h-3.5 w-3.5" />
						{/if}
						{t('ai.verify_perplexity', 'Verify with Perplexity')}
					</Button>
					<Button
						variant="destructive"
						onclick={() => (showRejectInput = true)}
					>
						<XCircle class="h-3.5 w-3.5" />
						{t('ai.reject')}
					</Button>
				{:else}
					<Button
						variant="destructive"
						disabled={actionLoading || !rejectReason.trim()}
						onclick={rejectReview}
					>
						{#if actionLoading}
							<Loader2 class="h-3.5 w-3.5 animate-spin" />
						{/if}
						{t('ai.reject')}
					</Button>
				{/if}
			</div>
		</div>
	{/if}
</Modal>

<!-- ═══ Tool Edit SlideOver ═══ -->
<SlideOver bind:open={showToolSlideOver} title={t('ai.tools.configure')}>
	<div class="space-y-4">
		<div>
			<label for="tool-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('ai.tools.name')}</label>
			<p class="mt-1 block text-xs" style="color: var(--color-muted)">{t('ai.tool_name_hint', 'The tool name shown to the AI.')}</p>
			<input id="tool-name" type="text" bind:value={toolName} class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" />
		</div>
		<div>
			<label for="tool-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('ai.tools.description')}</label>
			<p class="mt-1 block text-xs" style="color: var(--color-muted)">{t('ai.tool_desc_hint', 'What the tool does — the AI uses this to decide when to call it.')}</p>
			<textarea id="tool-desc" bind:value={toolDescription} rows="3" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"></textarea>
		</div>
		<div class="flex items-center gap-3">
			<Switch
				bind:checked={toolEnabled}
				ariaLabel={t('ai.tools.enabled')}
			/>
			<span class="text-sm font-medium text-foreground-alt">{t('ai.tools.enabled')}</span>
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-3 pt-4">
			<Button variant="outline" onclick={() => showToolSlideOver = false}>{t('ai.cancel')}</Button>
			<Button onclick={saveTool} disabled={toolSaving}>
				{toolSaving ? 'Saving...' : 'Save Changes'}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- ═══ Tool Delete Confirmation Modal ═══ -->
<Modal bind:open={showDeleteConfirm} title={t('ai.tools.delete_title', 'Delete Tool')}>
	{#if deletingTool}
		<p class="text-sm" style="color: var(--color-foreground-alt)">
			{t('ai.tools.delete_confirm', 'Are you sure you want to delete')} <strong>{deletingTool.name}</strong>?
		</p>
		<p class="mt-2 text-xs" style="color: var(--color-muted)">
			{t('ai.tools.delete_warning', 'This action cannot be undone. All usage history for this tool will also be removed.')}
		</p>
		{#if toolsError}
			<div class="mt-3 rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
				{toolsError}
			</div>
		{/if}
		<div class="mt-6 flex justify-end gap-3">
			<Button variant="outline" onclick={() => { showDeleteConfirm = false; deletingTool = null; }}>
				{t('common.cancel', 'Cancel')}
			</Button>
			<Button variant="destructive" onclick={deleteTool} disabled={toolDeleting}>
				{#if toolDeleting}
					<Loader2 class="h-3.5 w-3.5 animate-spin" />
				{:else}
					<Trash2 class="h-3.5 w-3.5" />
				{/if}
				{t('common.delete', 'Delete')}
			</Button>
		</div>
	{/if}
</Modal>
