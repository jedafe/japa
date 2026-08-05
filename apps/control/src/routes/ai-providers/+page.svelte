<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Cpu, Plus, Pencil, Trash2, Zap, Check, X } from 'lucide-svelte';
	import { Dialog, Select } from 'bits-ui';
	import { t } from '$lib/stores/i18n.svelte';
	import type { AiProvider } from '$lib/api/types';
	import type { DataTableColumn, DataTableRowAction } from '$lib/api/types';

	let {} = $props();

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

	// Model fetching state — when the admin enters an API key and clicks
	// "Fetch Models", we call the provider's /v1/models endpoint and
	// populate a dropdown instead of requiring manual model name entry.
	let availableModels = $state<{ id: string; label: string }[]>([]);
	let fetchingModels = $state(false);
	let fetchModelsError = $state('');

	const providerTypes = $derived([
		{ value: 'xiaomi', label: t('ai_providers.type_xiaomi', 'Xiaomi MiMo'), baseUrlHint: 'Auto-selected from key prefix (tp- or sk-)' },
		{ value: 'openai', label: t('ai_providers.type_openai', 'OpenAI-compatible'), baseUrlHint: 'https://api.openai.com/v1 — or OpenRouter: https://openrouter.ai/api/v1 — or Command Code: https://api.commandcode.ai/provider/v1' },
		{ value: 'ollama', label: t('ai_providers.type_ollama', 'Ollama'), baseUrlHint: 'http://localhost:11434/v1' },
		{ value: 'anthropic', label: t('ai_providers.type_anthropic', 'Anthropic (native)'), baseUrlHint: 'https://api.anthropic.com/v1 — or Command Code: https://api.commandcode.ai/provider/v1' },
		{ value: 'devin', label: t('ai_providers.type_devin', 'Devin (Cognition)'), baseUrlHint: 'https://api.devin.ai/v3/organizations/org-xxxxx (include org ID!) — model field = devin_mode (normal/fast/lite/ultra/fusion)' },
		{ value: 'cursor', label: t('ai_providers.type_cursor', 'Cursor (Anysphere)'), baseUrlHint: 'https://api.cursor.com/v1 (Basic auth, key from Cursor Dashboard)' },
		{ value: 'custom', label: t('ai_providers.type_custom', 'Custom'), baseUrlHint: 'Any OpenAI-compatible /v1 endpoint' }
	]);

	const categoryOptions = $derived([
		{ value: 'content_generation', label: t('ai_providers.cat_content_generation', 'Content Generation') },
		{ value: 'chat', label: t('ai_providers.cat_chat', 'Chat (JapaBot)') },
		{ value: 'embedding', label: t('ai_providers.cat_embedding', 'Embedding') },
		{ value: 'moderation', label: t('ai_providers.cat_moderation', 'Moderation') }
	]);

	$effect(() => {
		load();
	});

	async function load() {
		loading = true;
		errorMessage = '';
		const result = await api<AiProvider[]>('/control/ai-providers');
		if (result.ok) {
			providers = Array.isArray(result.data) ? result.data : [];
		} else {
			errorMessage = result.error;
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
			onClick: (row) => openEdit(row as unknown as AiProvider)
		},
		{
			label: t('ai_providers.test', 'Test Connection'),
			icon: Zap,
			onClick: (row) => testProvider(row as unknown as AiProvider)
		},
		{
			label: t('ai_providers.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
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

	async function fetchModels() {
		if (!form.api_key.trim()) {
			fetchModelsError = t('ai_providers.fetch_models_need_key', 'Enter an API key first to fetch available models.');
			return;
		}
		fetchingModels = true;
		fetchModelsError = '';
		availableModels = [];

		const result = await api<{ id: string; label: string }[]>('/control/ai-providers/fetch-models', {
			method: 'POST',
			body: {
				provider_type: form.provider_type,
				base_url: form.base_url,
				api_key: form.api_key
			}
		});

		if (result.ok && Array.isArray(result.data) && result.data.length > 0) {
			availableModels = result.data;
			// Auto-select the first model if none is set yet.
			if (!form.model && availableModels.length > 0) {
				form.model = availableModels[0].id;
			}
		} else {
			fetchModelsError = result.error || t('ai_providers.fetch_models_empty', 'No models returned — you can still type the model name manually.');
		}
		fetchingModels = false;
	}

	function resetModelFetch() {
		availableModels = [];
		fetchModelsError = '';
	}
</script>

<PageHeader title={t('ai_providers.title', 'AI Providers')} description={t('ai_providers.description', 'Manage AI provider configurations, API keys, and categories')} />

{#if errorMessage}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-xl border px-4 py-3 text-sm">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between gap-3">
	<SearchInput bind:value={filter} placeholder={t('common.search', 'Search...')} />
	<button
		class="bg-foreground text-background hover:bg-foreground/90 rounded-9px inline-flex items-center gap-1.5 px-4 py-2 text-sm font-medium transition-colors"
		onclick={openCreate}
	>
		<Plus class="size-4" />
		{t('ai_providers.create', 'Add Provider')}
	</button>
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
		<button class="ml-auto text-xs underline" onclick={() => (testResult = null)}>Dismiss</button>
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
<Dialog.Root bind:open={createModalOpen}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 fixed inset-0 z-40" />
		<Dialog.Content class="bg-background border-border-card fixed top-1/2 left-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 shadow-lg max-h-[90vh] overflow-y-auto">
			<Dialog.Title class="text-foreground text-lg font-semibold">{t('ai_providers.create_title', 'Add AI Provider')}</Dialog.Title>
			<Dialog.Description class="text-muted-foreground mt-1 text-sm">{t('ai_providers.create_description', 'Configure a new AI backend for the dispatcher.')}</Dialog.Description>

			<div class="mt-4 space-y-4">
				<!-- Name -->
				<div>
					<label for="create-name" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.name', 'Name')}</label>
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
					<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.type', 'Type')}</label>
					<Select.Root type="single" bind:value={form.provider_type}>
						<Select.Trigger class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm">
							<Select.Value />
						</Select.Trigger>
						<Select.Portal>
							<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-lg border">
								<Select.Viewport class="p-1">
									{#each providerTypes as pt}
										<Select.Item class="data-highlighted:bg-accent rounded-lg px-3 py-1.5 text-sm outline-none" value={pt.value} label={pt.label}>
											{pt.label}
										</Select.Item>
									{/each}
								</Select.Viewport>
							</Select.Content>
						</Select.Portal>
					</Select.Root>
				</div>

				<!-- API Key -->
				<div>
					<label for="create-api-key" class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.api_key', 'API Key')}</label>
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
						{#if availableModels.length > 0}
							<Select.Root type="single" bind:value={form.model}>
								<Select.Trigger class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono">
									<Select.Value placeholder={t('ai_providers.model_placeholder', 'Select a model')} />
								</Select.Trigger>
								<Select.Portal>
									<Select.Content class="border-border-input bg-background shadow-popover z-50 max-h-[300px] rounded-lg border overflow-y-auto">
										<Select.Viewport class="p-1">
											{#each availableModels as m}
												<Select.Item class="data-highlighted:bg-accent rounded-lg px-3 py-1.5 text-sm outline-none font-mono" value={m.id} label={m.label}>
													{m.label}
												</Select.Item>
											{/each}
										</Select.Viewport>
									</Select.Content>
								</Select.Portal>
							</Select.Root>
						{:else}
							<input
								id="create-model"
								type="text"
								bind:value={form.model}
								class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
								placeholder={t('ai_providers.model_placeholder', 'e.g., mimo-v2-flash')}
							/>
						{/if}
						<button
							type="button"
							class="text-muted-foreground hover:text-foreground mt-1 text-[10px] underline transition-colors disabled:opacity-50"
							disabled={fetchingModels}
							onclick={fetchModels}
						>
							{fetchingModels
								? t('ai_providers.fetching_models', 'Fetching models...')
								: t('ai_providers.fetch_models', 'Fetch available models from provider')}
						</button>
						{#if fetchModelsError}
							<p class="text-destructive mt-1 text-[10px]">{fetchModelsError}</p>
						{/if}
					</div>
				</div>

				<!-- Categories -->
				<div>
					<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.categories', 'Categories')}</label>
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
						<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.enabled', 'Enabled')}</label>
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
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Edit Provider Dialog -->
<Dialog.Root bind:open={editModalOpen}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 fixed inset-0 z-40" />
		<Dialog.Content class="bg-background border-border-card fixed top-1/2 left-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 shadow-lg max-h-[90vh] overflow-y-auto">
			<Dialog.Title class="text-foreground text-lg font-semibold">{t('ai_providers.edit_title', 'Edit AI Provider')}</Dialog.Title>
			<Dialog.Description class="text-muted-foreground mt-1 text-sm">
				{editingProvider?.name} · <span class="font-mono">{getProviderTypeLabel(editingProvider?.provider_type ?? '')}</span>
			</Dialog.Description>

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
					<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.type', 'Type')}</label>
					<Select.Root type="single" bind:value={form.provider_type}>
						<Select.Trigger class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm">
							<Select.Value />
						</Select.Trigger>
						<Select.Portal>
							<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-lg border">
								<Select.Viewport class="p-1">
									{#each providerTypes as pt}
										<Select.Item class="data-highlighted:bg-accent rounded-lg px-3 py-1.5 text-sm outline-none" value={pt.value} label={pt.label}>
											{pt.label}
										</Select.Item>
									{/each}
								</Select.Viewport>
							</Select.Content>
						</Select.Portal>
					</Select.Root>
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
						<p class="text-muted-foreground mt-1 text-[10px]">Current: <span class="font-mono">{editingProvider.api_key_masked}</span></p>
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
						{#if availableModels.length > 0}
							<Select.Root type="single" bind:value={form.model}>
								<Select.Trigger class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono">
									<Select.Value placeholder={t('ai_providers.model_placeholder', 'Select a model')} />
								</Select.Trigger>
								<Select.Portal>
									<Select.Content class="border-border-input bg-background shadow-popover z-50 max-h-[300px] rounded-lg border overflow-y-auto">
										<Select.Viewport class="p-1">
											{#each availableModels as m}
												<Select.Item class="data-highlighted:bg-accent rounded-lg px-3 py-1.5 text-sm outline-none font-mono" value={m.id} label={m.label}>
													{m.label}
												</Select.Item>
											{/each}
										</Select.Viewport>
									</Select.Content>
								</Select.Portal>
							</Select.Root>
						{:else}
							<input
								id="edit-model"
								type="text"
								bind:value={form.model}
								class="border-border-input bg-background-alt text-foreground w-full rounded-9px border px-3 py-2 text-sm font-mono"
								placeholder={t('ai_providers.model_placeholder', 'e.g., mimo-v2-flash')}
							/>
						{/if}
						<button
							type="button"
							class="text-muted-foreground hover:text-foreground mt-1 text-[10px] underline transition-colors disabled:opacity-50"
							disabled={fetchingModels}
							onclick={fetchModels}
						>
							{fetchingModels
								? t('ai_providers.fetching_models', 'Fetching models...')
								: t('ai_providers.fetch_models', 'Fetch available models from provider')}
						</button>
						{#if fetchModelsError}
							<p class="text-destructive mt-1 text-[10px]">{fetchModelsError}</p>
						{/if}
					</div>
				</div>

				<!-- Categories -->
				<div>
					<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.categories', 'Categories')}</label>
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
						<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider">{t('ai_providers.enabled', 'Enabled')}</label>
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
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<!-- Delete Confirmation -->
<ConfirmModal
	bind:open={confirmOpen}
	title={t('ai_providers.delete_title', 'Delete AI Provider')}
	consequence={deleteTarget ? t('ai_providers.delete_confirm_detail', 'Permanently delete provider \'{name}\'. The dispatcher will be hot-reloaded and this provider will stop serving requests immediately.').replace('{name}', deleteTarget.name) : ''}
	onConfirm={confirmDelete}
/>
