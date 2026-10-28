<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Cpu, Plus, Pencil, Trash2, Zap, Check, X } from 'lucide-svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import type { AiProvider } from '$lib/api/types';
	import type { DataTableColumn, DataTableRowAction } from '$lib/api/types';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	let {} = $props();

	// Backend routes gated by ai.providers.view (list/get/types) and
	// ai.providers.manage (create/update/delete/test/fetch-models) —
	// migration 000452, server.go:6096-6111. The frontend must match so
	// users without view access see an access-denied message instead of
	// triggering 403 API calls.
	const canView = $derived(hasAnyPermission(['ai.providers.view', 'ai.providers.manage']));
	const canManage = $derived(hasAnyPermission(['ai.providers.manage']));

	let providers = $state<AiProvider[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let filter = $state('');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('priority');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());

	// Create/Edit dialog state
	let createModalOpen = $state(false);
	let editModalOpen = $state(false);
	let editingProvider = $state<AiProvider | null>(null);
	let saving = $state(false);
	let formError = $state('');

	let form = $state({
		name: '',
		provider_type: 'xiaomi',
		api_key: '',
		base_url: '',
		model: '',
		categories: ['content_generation'] as string[],
		priority: 0,
		enabled: true
	});

	// Delete confirmation state
	let confirmOpen = $state(false);
	let deleteTarget = $state<AiProvider | null>(null);
	let deleting = $state(false);

	// Test result state
	let testResult = $state<{ id: string; status: string; message: string } | null>(null);
	let testing = $state<string | null>(null);

	// Selected preset in the create dialog (one-way; selecting a preset fills the form).
	let selectedPreset = $state('');

	// Model fetching state — the Autocomplete's initialFetcher calls the
	// provider's /v1/models endpoint via the backend proxy when the model
	// field is focused. Results are cached in availableModels so subsequent
	// keystrokes filter client-side without another network call. The cache
	// is invalidated when provider_type, base_url, or api_key changes.
	let availableModels = $state<{ id: string; label: string }[]>([]);
	let fetchingModels = $state(false);
	let fetchModelsError = $state('');
	let modelsFetchedForKey = $state('');

	// Provider type UI metadata — i18n labels and base URL hints keyed by
	// provider_type. The list of valid types is fetched from the API
	// (GET /control/ai-providers/types, Rule 16); this map supplies the
	// human-readable label and hint for each known type.
	const providerTypeMeta: Record<string, { label: string; baseUrlHint: string; modelHint?: string }> = {
		xiaomi: { label: t('ai_providers.type_xiaomi', 'Xiaomi MiMo'), baseUrlHint: 'Auto-selected from key prefix (tp- or sk-)', modelHint: 'mimo-v2-flash, mimo-v2.5, mimo-v2.5-pro' },
		openai: { label: t('ai_providers.type_openai', 'OpenAI-compatible'), baseUrlHint: 'https://api.openai.com/v1 — or OpenRouter: https://openrouter.ai/api/v1 — or Command Code: https://api.commandcode.ai/provider/v1', modelHint: 'gpt-5.4, gpt-4o, gpt-4o-mini, gpt-4.1' },
		groq: { label: t('ai_providers.type_groq', 'Groq'), baseUrlHint: 'https://api.groq.com/openai/v1 (OpenAI-compatible)', modelHint: 'llama-4-scout-17b-16e-instruct, qwen-2.5-32b-instruct, openai/gpt-oss-20b' },
		ollama: { label: t('ai_providers.type_ollama', 'Ollama'), baseUrlHint: 'Local: http://localhost:11434/v1 — Cloud: https://ollama.com/v1 (set Ollama API key)', modelHint: 'llama3.2, mistral, qwen2.5, deepseek-r1' },
		anthropic: { label: t('ai_providers.type_anthropic', 'Anthropic (native)'), baseUrlHint: 'https://api.anthropic.com/v1 — or Command Code: https://api.commandcode.ai/provider/v1', modelHint: 'claude-sonnet-5, claude-opus-5, claude-sonnet-4-6' },
		devin: { label: t('ai_providers.type_devin', 'Devin (Cognition)'), baseUrlHint: 'https://api.devin.ai/v3/organizations/org-xxxxx (include org ID!) — model field = devin_mode (normal/fast/lite/ultra/fusion)', modelHint: 'normal, fast, lite, ultra, fusion' },
		cursor: { label: t('ai_providers.type_cursor', 'Cursor (Anysphere)'), baseUrlHint: 'https://api.cursor.com/v1 (Basic auth, key from Cursor Dashboard)', modelHint: 'composer-2.5, composer-2, auto-smart, grok-4.5' },
		perplexity: { label: t('ai_providers.type_perplexity', 'Perplexity (Agent)'), baseUrlHint: 'https://api.perplexity.ai — use provider-prefixed models (perplexity/sonar, openai/gpt-5.6-sol) or presets (low, medium, high, fast, xhigh)', modelHint: 'perplexity/sonar, low, medium, high, openai/gpt-5.6-sol' },
		custom: { label: t('ai_providers.type_custom', 'Custom'), baseUrlHint: 'Any OpenAI-compatible /v1 endpoint' }
	};

interface ProviderPreset {
	label: string;
	name: string;
	provider_type: string;
	base_url: string;
	model: string;
	categories: string[];
}

// Common provider presets to speed up Control Panel configuration.
// Selecting a preset does not fill the API key — the admin must still enter it.
const providerPresets: ProviderPreset[] = [
	{
		label: 'OpenAI',
		name: 'OpenAI',
		provider_type: 'openai',
		base_url: 'https://api.openai.com/v1',
		model: 'gpt-4o-mini',
		categories: ['content_generation', 'chat', 'extraction']
	},
	{
		label: 'OpenRouter',
		name: 'OpenRouter',
		provider_type: 'openai',
		base_url: 'https://openrouter.ai/api/v1',
		model: 'openai/gpt-4o-mini',
		categories: ['content_generation', 'chat', 'extraction']
	},
	{
		label: 'Groq',
		name: 'Groq',
		provider_type: 'groq',
		base_url: 'https://api.groq.com/openai/v1',
		model: 'llama-4-scout-17b-16e-instruct',
		categories: ['content_generation', 'chat', 'extraction']
	},
	{
		label: 'Together AI',
		name: 'Together AI',
		provider_type: 'openai',
		base_url: 'https://api.together.xyz/v1',
		model: 'meta-llama/Llama-3.3-70B-Instruct-Turbo',
		categories: ['content_generation', 'chat']
	},
	{
		label: 'Anthropic',
		name: 'Anthropic',
		provider_type: 'anthropic',
		base_url: 'https://api.anthropic.com/v1',
		model: 'claude-sonnet-5',
		categories: ['content_generation', 'chat', 'extraction']
	},
	{
		label: 'Perplexity',
		name: 'Perplexity',
		provider_type: 'perplexity',
		base_url: 'https://api.perplexity.ai',
		model: 'low',
		categories: ['content_generation', 'extraction']
	},
	{
		label: 'Xiaomi MiMo',
		name: 'Xiaomi MiMo',
		provider_type: 'xiaomi',
		base_url: '',
		model: 'mimo-v2-flash',
		categories: ['content_generation', 'chat', 'extraction']
	},
	{
		label: 'Ollama Cloud',
		name: 'Ollama Cloud',
		provider_type: 'ollama',
		base_url: 'https://ollama.com/v1',
		model: 'llama3.2',
		categories: ['content_generation', 'chat', 'extraction']
	},
	{
		label: 'Local Ollama',
		name: 'Local Ollama',
		provider_type: 'ollama',
		base_url: 'http://localhost:11434/v1',
		model: 'llama3.2',
		categories: ['content_generation', 'chat', 'extraction']
	}
];

	const categoryMeta: Record<string, string> = {
		content_generation: t('ai_providers.cat_content_generation', 'Content Generation'),
		chat: t('ai_providers.cat_chat', 'Chat (JapaBot)'),
		embedding: t('ai_providers.cat_embedding', 'Embedding'),
		moderation: t('ai_providers.cat_moderation', 'Moderation'),
		extraction: t('ai_providers.cat_extraction', 'Extraction')
	};

	// Fetch valid provider types and categories from the API (Rule 16).
	let apiProviderTypes = $state<string[]>([]);
	let apiCategories = $state<string[]>([]);

	let providerTypes = $derived(
		apiProviderTypes.map((v) => ({
			value: v,
			label: providerTypeMeta[v]?.label ?? v,
			baseUrlHint: providerTypeMeta[v]?.baseUrlHint ?? '',
			modelHint: providerTypeMeta[v]?.modelHint ?? ''
		}))
	);

	let categoryOptions = $derived(
		apiCategories.map((v) => ({
			value: v,
			label: categoryMeta[v] ?? v
		}))
	);

	$effect(() => {
		load();
	});

	// Invalidate the model cache when provider config fields change so
	// the next focus of the model Autocomplete re-fetches from the API.
	$effect(() => {
		// Track these reactively
		form.provider_type;
		form.base_url;
		form.api_key;
		if (modelsFetchedForKey && modelsFetchedForKey !== modelsCacheKey()) {
			availableModels = [];
			modelsFetchedForKey = '';
		}
	});

	async function load() {
		loading = true;
		errorMessage = '';
		if (!canView) {
			loading = false;
			return;
		}
		const result = await api<AiProvider[]>('/control/ai-providers');
		if (result.ok) {
			providers = Array.isArray(result.data) ? result.data : [];
		} else {
			errorMessage = result.error;
		}

		// Fetch valid provider types and categories from the API (Rule 16).
		const typesResult = await api<{ provider_types: string[]; categories: string[] }>('/control/ai-providers/types');
		if (typesResult.ok && typesResult.data) {
			apiProviderTypes = typesResult.data.provider_types ?? [];
			apiCategories = typesResult.data.categories ?? [];
		} else if (result.ok) {
			errorMessage = (!typesResult.ok ? typesResult.error : null) || t('ai_providers.types_error', 'Could not load provider types; category selection may be limited.');
		}
		loading = false;
	}

	let filtered = $derived(
		providers.filter((p) => {
			if (!filter) return true;
			const q = filter.toLowerCase();
			return (
				p.name?.toLowerCase().includes(q) ||
				p.provider_type?.toLowerCase().includes(q) ||
				p.model?.toLowerCase().includes(q) ||
				p.categories?.some((c) => c.toLowerCase().includes(q))
			);
		})
	);

	let sorted = $derived(
		[...filtered].sort((a, b) => {
			if (!sortKey) return 0;
			const dir = sortDirection === 'asc' ? 1 : -1;
			const av = (a as unknown as Record<string, unknown>)[sortKey];
			const bv = (b as unknown as Record<string, unknown>)[sortKey];
			if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir;
			return String(av ?? '').localeCompare(String(bv ?? '')) * dir;
		})
	);

	let paged = $derived(sorted.slice(page * pageSize, (page + 1) * pageSize));

	const columns: DataTableColumn[] = [
		{
			key: 'name',
			label: t('ai_providers.name', 'Name'),
			sortable: true,
			render: (val) => `<span class="font-medium text-foreground">${val}</span>`
		},
		{
			key: 'provider_type',
			label: t('ai_providers.type', 'Type'),
			sortable: true,
			render: (val) => {
				const label = providerTypes.find((p) => p.value === val)?.label ?? val;
				return `<span class="font-mono text-xs text-muted-foreground">${label}</span>`;
			}
		},
		{
			key: 'model',
			label: t('ai_providers.model', 'Default Model'),
			sortable: true,
			render: (val) => val ? `<span class="font-mono text-xs text-muted-foreground">${val}</span>` : '—'
		},
		{
			key: 'categories',
			label: t('ai_providers.categories', 'Categories'),
			sortable: false,
			render: (val) => {
				const cats = val as string[];
				if (!cats || cats.length === 0) return '—';
				return cats
					.map((c) => `<span class="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-[10px] font-medium">${c}</span>`)
					.join(' ');
			}
		},
		{
			key: 'api_key_masked',
			label: t('ai_providers.api_key_masked', 'API Key'),
			sortable: false,
			render: (val) => {
				if (!val) return `<span class="text-muted-foreground text-xs italic">${t('ai_providers.no_key', 'No key set')}</span>`;
				return `<span class="font-mono text-xs text-muted-foreground">${val}</span>`;
			}
		},
		{
			key: 'priority',
			label: t('ai_providers.priority', 'Priority'),
			sortable: true,
			render: (val) => `<span class="font-mono text-xs text-muted-foreground">${val}</span>`
		},
		{
			key: 'enabled',
			label: t('ai_providers.status', 'Status'),
			sortable: true,
			render: (val) => {
				if (val) return `<span class="bg-emerald-500/15 text-emerald-600 rounded-full px-2 py-0.5 text-[10px] font-medium">${t('ai_providers.active', 'Active')}</span>`;
				return `<span class="bg-muted text-muted-foreground rounded-full px-2 py-0.5 text-[10px] font-medium">${t('ai_providers.disabled', 'Disabled')}</span>`;
			}
		}
	];

	const rowActions: DataTableRowAction[] = [
		{
			label: t('ai_providers.edit', 'Edit'),
			icon: Pencil,
			hidden: () => !canManage,
			onClick: (row) => openEdit(row as unknown as AiProvider)
		},
		{
			label: t('ai_providers.test', 'Test Connection'),
			icon: Zap,
			hidden: () => !canManage,
			onClick: (row) => testProvider(row as unknown as AiProvider)
		},
		{
			label: t('ai_providers.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			hidden: () => !canManage,
			onClick: (row) => openDeleteConfirm(row as unknown as AiProvider)
		}
	];

	function openCreate() {
		form = {
			name: '',
			provider_type: 'xiaomi',
			api_key: '',
			base_url: '',
			model: '',
			categories: ['content_generation'],
			priority: 0,
			enabled: true
		};
		formError = '';
		editingProvider = null;
		resetModelFetch();
		createModalOpen = true;
	}

	function applyPreset(preset: ProviderPreset) {
		form.provider_type = preset.provider_type;
		form.base_url = preset.base_url;
		form.model = preset.model;
		if (!form.name) form.name = preset.name;
		form.categories = preset.categories.filter((c) => apiCategories.includes(c));
		resetModelFetch();
	}

	function openEdit(provider: AiProvider) {
		editingProvider = provider;
		form = {
			name: provider.name,
			provider_type: provider.provider_type,
			api_key: '',
			base_url: provider.base_url,
			model: provider.model,
			categories: provider.categories?.length ? [...provider.categories] : ['content_generation'],
			priority: provider.priority,
			enabled: provider.enabled
		};
		formError = '';
		resetModelFetch();
		editModalOpen = true;
	}

	function openDeleteConfirm(provider: AiProvider) {
		deleteTarget = provider;
		confirmOpen = true;
	}

	async function saveProvider() {
		if (!form.name.trim()) {
			formError = t('ai_providers.name_required', 'Name is required.');
			return;
		}
		saving = true;
		formError = '';

		if (editingProvider) {
			// Update existing — only send fields that changed. API key is
			// only sent if the user entered a new value (empty = preserve).
			const body: Record<string, unknown> = {
				name: form.name,
				provider_type: form.provider_type,
				base_url: form.base_url,
				model: form.model,
				categories: form.categories,
				priority: form.priority,
				enabled: form.enabled
			};
			if (form.api_key.trim()) {
				body.api_key = form.api_key;
			}
			const result = await api<AiProvider>(`/control/ai-providers/${editingProvider.id}`, {
				method: 'PATCH',
				body
			});
			if (result.ok) {
				editModalOpen = false;
				await load();
			} else {
				formError = result.error || t('ai_providers.save_error', 'Failed to save provider.');
			}
		} else {
			// Create new
			const result = await api<AiProvider>('/control/ai-providers', {
				method: 'POST',
				body: {
					name: form.name,
					provider_type: form.provider_type,
					api_key: form.api_key,
					base_url: form.base_url,
					model: form.model,
					categories: form.categories,
					priority: form.priority,
					enabled: form.enabled
				}
			});
			if (result.ok) {
				createModalOpen = false;
				await load();
			} else {
				formError = result.error || t('ai_providers.save_error', 'Failed to save provider.');
			}
		}
		saving = false;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		deleting = true;
		const result = await api(`/control/ai-providers/${deleteTarget.id}`, { method: 'DELETE' });
		if (result.ok) {
			confirmOpen = false;
			deleteTarget = null;
			await load();
		} else {
			errorMessage = result.error || t('ai_providers.delete_error', 'Failed to delete provider.');
		}
		deleting = false;
	}

	async function testProvider(provider: AiProvider) {
		testing = provider.id;
		testResult = null;
		const result = await api<{ status: string; message: string }>(`/control/ai-providers/${provider.id}/test`, {
			method: 'POST'
		});
		if (result.ok) {
			testResult = { id: provider.id, status: result.data.status, message: result.data.message };
		} else {
			testResult = { id: provider.id, status: 'failed', message: result.error || 'Request failed' };
		}
		testing = null;
	}

	function toggleCategory(cat: string) {
		if (form.categories.includes(cat)) {
			form.categories = form.categories.filter((c) => c !== cat);
		} else {
			form.categories = [...form.categories, cat];
		}
	}

	function getProviderTypeLabel(value: string): string {
		return providerTypes.find((p) => p.value === value)?.label ?? value;
	}

	function getBaseUrlHint(value: string): string {
		return providerTypes.find((p) => p.value === value)?.baseUrlHint ?? '';
	}

	function getModelHint(value: string): string {
		return providerTypes.find((p) => p.value === value)?.modelHint ?? '';
	}

	// modelsCacheKey returns a composite key that identifies the current
	// provider configuration. When any of these fields change, the cached
	// model list is stale and must be re-fetched.
	function modelsCacheKey(): string {
		return `${form.provider_type}|${form.base_url}|${form.api_key}`;
	}

	// fetchModelsFromProvider calls the backend proxy to fetch the full
	// model list from the provider's /v1/models endpoint. Results are
	// cached in availableModels keyed by modelsCacheKey() so subsequent
	// Autocomplete focus events skip the network call.
	async function fetchModelsFromProvider(): Promise<{ id: string; label: string }[]> {
		if (!form.api_key.trim() || !form.provider_type) {
			return [];
		}
		const result = await api<{ id: string; label: string }[]>('/control/ai-providers/fetch-models', {
			method: 'POST',
			body: {
				provider_type: form.provider_type,
				base_url: form.base_url,
				api_key: form.api_key
			}
		});
		if (!result.ok) {
			throw new Error(result.error || 'fetch failed');
		}
		if (!Array.isArray(result.data)) {
			throw new Error('fetch failed');
		}
		return result.data;
	}

	function resetModelFetch() {
		availableModels = [];
		fetchModelsError = '';
		modelsFetchedForKey = '';
	}

	// --- Autocomplete fetchers ---

	// Provider Type fetcher — filters the server-fetched providerTypes list.
	async function fetchProviderTypes(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return providerTypes
			.filter((pt) => pt.label.toLowerCase().includes(query) || pt.value.toLowerCase().includes(query))
			.map((pt) => ({ id: pt.value, label: pt.label }));
	}

	async function fetchProviderTypesInitial(): Promise<AutocompleteItem[]> {
		return providerTypes.map((pt) => ({ id: pt.value, label: pt.label }));
	}

	function getProviderTypeSelectedLabel(value: string): string | null {
		if (!value) return null;
		return providerTypes.find((pt) => pt.value === value)?.label ?? value;
	}

	// Model fetcher — filters the cached availableModels list client-side.
	// If the cache is empty (e.g. initial fetch hasn't run yet), triggers
	// a server-side fetch first so typing always shows results.
	async function fetchModelsOptions(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		// If cache is empty or stale, try to populate it before filtering.
		if (availableModels.length === 0 && form.api_key.trim() && form.provider_type) {
			await fetchModelsOptionsInitial();
		}
		return availableModels
			.filter((m) => m.label.toLowerCase().includes(query) || m.id.toLowerCase().includes(query))
			.map((m) => ({ id: m.id, label: m.label }));
	}

	// Model initial fetcher — called by Autocomplete when the model field
	// gains focus with no query. Fetches the full model list from the
	// provider's API (via the backend proxy) and caches it. If the cache
	// is already valid for the current provider config, returns immediately.
	async function fetchModelsOptionsInitial(): Promise<AutocompleteItem[]> {
		const key = modelsCacheKey();
		if (modelsFetchedForKey === key && availableModels.length > 0) {
			return availableModels.map((m) => ({ id: m.id, label: m.label }));
		}
		if (!form.api_key.trim() || !form.provider_type) {
			return [];
		}
		fetchingModels = true;
		fetchModelsError = '';
		try {
			availableModels = await fetchModelsFromProvider();
			modelsFetchedForKey = key;
		} catch (err) {
			availableModels = [];
			fetchModelsError = err instanceof Error
				? err.message
				: t('ai_providers.fetch_models_empty', 'No models returned — you can still type the model name manually.');
		} finally {
			fetchingModels = false;
		}
		return availableModels.map((m) => ({ id: m.id, label: m.label }));
	}

	function getModelSelectedLabel(value: string): string | null {
		if (!value) return null;
		return availableModels.find((m) => m.id === value)?.label ?? value;
	}
</script>

<PageHeader title={t('ai_providers.title', 'AI Providers')} description={t('ai_providers.description', 'Manage AI provider configurations, API keys, and categories')} />

{#if !canView}
	<div class="flex items-center gap-2 rounded-lg border p-4 text-sm" style="border-color: var(--color-danger); color: var(--color-danger)">
		{t('ai_providers.no_permission', 'You do not have permission to view AI providers.')}
	</div>
{:else}
{#if errorMessage}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-xl border px-4 py-3 text-sm">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between gap-3">
	<SearchInput bind:value={filter} placeholder={t('common.search', 'Search...')} />
	{#if canManage}
		<button
			class="bg-foreground text-background hover:bg-foreground/90 rounded-9px inline-flex items-center gap-1.5 px-4 py-2 text-sm font-medium transition-colors"
			onclick={openCreate}
		>
			<Plus class="size-4" />
			{t('ai_providers.create', 'Add Provider')}
		</button>
	{/if}
</div>

{#if testResult}
	<div
		class="mb-4 flex items-center gap-2 rounded-xl border px-4 py-3 text-sm {testResult.status === 'success'
			? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600'
			: 'border-destructive bg-destructive/10 text-destructive'}"
	>
		{#if testResult.status === 'success'}
			<Check class="size-4" />
			{t('ai_providers.test_success', 'Provider responded successfully.')}
		{:else}
			<X class="size-4" />
			{t('ai_providers.test_failed', 'Provider test failed: {message}').replace('{message}', testResult.message)}
		{/if}
		<button class="ms-auto text-xs underline" onclick={() => (testResult = null)}>Dismiss</button>
	</div>
{/if}

<DataTable
	{columns}
	rows={paged}
	totalRows={filtered.length}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	bind:selectedIds
	{rowActions}
	{loading}
	emptyMessage={t('ai_providers.empty', 'No AI providers configured. Create one to get started.')}
	emptyIcon={Cpu}
/>

<!-- Create Provider Dialog -->
<Modal bind:open={createModalOpen} title={t('ai_providers.create_title', 'Add AI Provider')} description={t('ai_providers.create_description', 'Configure a new AI backend for the dispatcher.')} size="lg">
			<div class="mt-4 space-y-4">
				<!-- Preset -->
				<div>
					<label for="create-preset" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.preset', 'Start from preset')}
					<HintIcon text={t('ai_providers.preset_hint', 'Prefills settings for a known provider.')}  />
</label>
					<AutocompleteSelect
						id="create-preset"
						bind:value={selectedPreset}
						onValueChange={(v) => {
							const preset = providerPresets.find((p) => p.name === v);
							if (preset) applyPreset(preset);
						}}
						options={[
							{ value: '', label: t('ai_providers.preset_none', '— No preset —') },
							...providerPresets.map((p) => ({ value: p.name, label: p.label }))
						]}
						class="w-full"
					/>
					<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.preset_help', 'Select a common provider to auto-fill the fields. API key is still required.')}</p>
				</div>

				<!-- Name -->
				<div>
					<label for="create-name" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.name', 'Name')}
					<HintIcon text={t('ai_providers.name_hint', 'The display name for this provider.')}  />
</label>
					<input
						id="create-name"
						type="text"
						bind:value={form.name}
						class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm"
						placeholder={t('ai_providers.name_placeholder', 'e.g., Xiaomi MiMo Production')}
					/>
				</div>

				<!-- Provider Type -->
				<div>
					<label for="provider-type" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.type', 'Type')}
					<HintIcon text={t('ai_providers.type_hint', 'The provider category — controls how it is called.')}  />
</label>
					<Autocomplete
						id="provider-type"
						placeholder={t('ai_providers.type', 'Type')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchProviderTypes}
						initialFetcher={fetchProviderTypesInitial}
						selectedId={form.provider_type || null}
						selectedLabel={getProviderTypeSelectedLabel(form.provider_type)}
						onSelect={(item) => (form.provider_type = item.id)}
						onClear={() => (form.provider_type = '')}
					/>
				</div>

				<!-- API Key -->
				<div>
					<label for="create-api-key" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.api_key', 'API Key')}
					<HintIcon text={t('ai_providers.api_key_hint', 'Stored encrypted — used only for your AI features.')}  />
</label>
					<input
						id="create-api-key"
						type="password"
						bind:value={form.api_key}
						class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
						placeholder="sk-..."
					/>
					<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.api_key_help', 'The key is encrypted at rest and never displayed after saving.')}</p>
				</div>

				<!-- Base URL + Model -->
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="create-base-url" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.base_url', 'Base URL')}</label>
						<input
							id="create-base-url"
							type="text"
							bind:value={form.base_url}
							class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
							placeholder={t('ai_providers.base_url_placeholder', 'Leave empty for provider default')}
						/>
						<p class="text-muted-foreground mt-1 text-[10px]">{getBaseUrlHint(form.provider_type)}</p>
					</div>
					<div>
						<label for="create-model" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.model', 'Default Model')}</label>
						<Autocomplete
							id="create-model"
							placeholder={t('ai_providers.model_placeholder', 'Type to search or enter model name')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchModelsOptions}
							initialFetcher={fetchModelsOptionsInitial}
							selectedId={form.model || null}
							selectedLabel={getModelSelectedLabel(form.model)}
							onSelect={(item) => (form.model = item.id)}
							onClear={() => (form.model = '')}
							onFreeTextBlur={(q) => (form.model = q)}
							freeText
							inputClass="font-mono"
						/>
						<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.model_help', 'Focus to load available models from the provider, or type a model name manually.')}</p>
						{#if getModelHint(form.provider_type)}
							<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.model_hint', 'Examples:')} {getModelHint(form.provider_type)}</p>
						{/if}
						{#if fetchModelsError}
							<p class="text-destructive mt-1 text-[10px]">{fetchModelsError}</p>
						{/if}
					</div>
				</div>

				<!-- Categories -->
				<div>
					<span class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.categories', 'Categories')}</span>
					<div class="flex flex-wrap gap-2">
						{#each categoryOptions as cat}
							<button
								type="button"
								class="rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors {form.categories.includes(cat.value)
									? 'bg-accent text-accent-foreground border-accent'
									: 'border-border-input bg-background-alt text-foreground hover:bg-muted'}"
								onclick={() => toggleCategory(cat.value)}
							>
								{cat.label}
							</button>
						{/each}
					</div>
				</div>

				<!-- Priority + Enabled -->
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="create-priority" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.priority', 'Priority')}</label>
						<input
							id="create-priority"
							type="number"
							min="0"
							bind:value={form.priority}
							class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
						/>
						<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.priority_help', 'Lower number = tried first during dispatch.')}</p>
					</div>
					<div>
						<span class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.enabled', 'Enabled')}</span>
						<button
							type="button"
							class="rounded-9px border px-3 py-2 text-sm font-medium transition-colors {form.enabled
								? 'bg-emerald-500/15 text-emerald-600 border-emerald-500/30'
								: 'border-border-input bg-background-alt text-foreground'}"
							onclick={() => (form.enabled = !form.enabled)}
						>
							{form.enabled ? t('ai_providers.active', 'Active') : t('ai_providers.disabled', 'Disabled')}
						</button>
					</div>
				</div>

				{#if formError}
					<p class="text-destructive text-xs">{formError}</p>
				{/if}
			</div>

			<div class="mt-6 flex justify-end gap-2">
				<button
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={() => (createModalOpen = false)}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					class="bg-foreground text-background hover:bg-foreground/90 rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={saving}
					onclick={saveProvider}
				>
					{saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
				</button>
			</div>
</Modal>

<!-- Edit Provider Dialog -->
<Modal bind:open={editModalOpen} title={t('ai_providers.edit_title', 'Edit AI Provider')} size="lg">
	{#snippet description()}
		{editingProvider?.name} · <span class="font-mono">{getProviderTypeLabel(editingProvider?.provider_type ?? '')}</span>
	{/snippet}

			<div class="mt-4 space-y-4">
				<!-- Name -->
				<div>
					<label for="edit-name" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.name', 'Name')}</label>
					<input
						id="edit-name"
						type="text"
						bind:value={form.name}
						class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm"
						placeholder={t('ai_providers.name_placeholder', 'e.g., Xiaomi MiMo Production')}
					/>
				</div>

				<!-- Provider Type -->
				<div>
					<label for="provider-type" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.type', 'Type')}</label>
					<Autocomplete
						id="provider-type"
						placeholder={t('ai_providers.type', 'Type')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchProviderTypes}
						initialFetcher={fetchProviderTypesInitial}
						selectedId={form.provider_type || null}
						selectedLabel={getProviderTypeSelectedLabel(form.provider_type)}
						onSelect={(item) => (form.provider_type = item.id)}
						onClear={() => (form.provider_type = '')}
					/>
				</div>

				<!-- API Key (edit mode — empty = keep existing) -->
				<div>
					<label for="edit-api-key" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.api_key', 'API Key')}</label>
					<input
						id="edit-api-key"
						type="password"
						bind:value={form.api_key}
						class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
						placeholder={t('ai_providers.api_key_placeholder', 'Enter API key (leave empty to keep existing)')}
					/>
					{#if editingProvider?.api_key_masked}
						<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.current_key', 'Current:')} <span class="font-mono">{editingProvider.api_key_masked}</span></p>
					{/if}
				</div>

				<!-- Base URL + Model -->
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="edit-base-url" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.base_url', 'Base URL')}</label>
						<input
							id="edit-base-url"
							type="text"
							bind:value={form.base_url}
							class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
							placeholder={t('ai_providers.base_url_placeholder', 'Leave empty for provider default')}
						/>
						<p class="text-muted-foreground mt-1 text-[10px]">{getBaseUrlHint(form.provider_type)}</p>
					</div>
					<div>
						<label for="edit-model" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.model', 'Default Model')}</label>
						<Autocomplete
							id="edit-model"
							placeholder={t('ai_providers.model_placeholder', 'Type to search or enter model name')}
							emptyMessage={t('common.no_results', 'No results found')}
							fetcher={fetchModelsOptions}
							initialFetcher={fetchModelsOptionsInitial}
							selectedId={form.model || null}
							selectedLabel={getModelSelectedLabel(form.model)}
							onSelect={(item) => (form.model = item.id)}
							onClear={() => (form.model = '')}
							onFreeTextBlur={(q) => (form.model = q)}
							freeText
							inputClass="font-mono"
						/>
						<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.model_help', 'Focus to load available models from the provider, or type a model name manually.')}</p>
						{#if getModelHint(form.provider_type)}
							<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.model_hint', 'Examples:')} {getModelHint(form.provider_type)}</p>
						{/if}
						{#if fetchModelsError}
							<p class="text-destructive mt-1 text-[10px]">{fetchModelsError}</p>
						{/if}
					</div>
				</div>

				<!-- Categories -->
				<div>
					<span class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.categories', 'Categories')}</span>
					<div class="flex flex-wrap gap-2">
						{#each categoryOptions as cat}
							<button
								type="button"
								class="rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors {form.categories.includes(cat.value)
									? 'bg-accent text-accent-foreground border-accent'
									: 'border-border-input bg-background-alt text-foreground hover:bg-muted'}"
								onclick={() => toggleCategory(cat.value)}
							>
								{cat.label}
							</button>
						{/each}
					</div>
				</div>

				<!-- Priority + Enabled -->
				<div class="grid grid-cols-2 gap-3">
					<div>
						<label for="edit-priority" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.priority', 'Priority')}</label>
						<input
							id="edit-priority"
							type="number"
							min="0"
							bind:value={form.priority}
							class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
						/>
						<p class="text-muted-foreground mt-1 text-[10px]">{t('ai_providers.priority_help', 'Lower number = tried first during dispatch.')}</p>
					</div>
					<div>
						<span class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.enabled', 'Enabled')}</span>
						<button
							type="button"
							class="rounded-9px border px-3 py-2 text-sm font-medium transition-colors {form.enabled
								? 'bg-emerald-500/15 text-emerald-600 border-emerald-500/30'
								: 'border-border-input bg-background-alt text-foreground'}"
							onclick={() => (form.enabled = !form.enabled)}
						>
							{form.enabled ? t('ai_providers.active', 'Active') : t('ai_providers.disabled', 'Disabled')}
						</button>
					</div>
				</div>

				{#if formError}
					<p class="text-destructive text-xs">{formError}</p>
				{/if}
			</div>

			<div class="mt-6 flex justify-end gap-2">
				<button
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={() => (editModalOpen = false)}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					class="bg-foreground text-background hover:bg-foreground/90 rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
					disabled={saving}
					onclick={saveProvider}
				>
					{saving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
				</button>
			</div>
</Modal>

<!-- Delete Confirmation -->
<ConfirmModal
	bind:open={confirmOpen}
	title={t('ai_providers.delete_title', 'Delete AI Provider')}
	consequence={deleteTarget ? t('ai_providers.delete_confirm_detail', 'Permanently delete provider \'{name}\'. The dispatcher will be hot-reloaded and this provider will stop serving requests immediately.').replace('{name}', deleteTarget.name) : ''}
	onConfirm={confirmDelete}
/>
{/if}
