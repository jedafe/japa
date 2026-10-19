<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { KeyRound, Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import type { AiByokProvider } from '$lib/api/types';
	import type { DataTableColumn, DataTableRowAction } from '$lib/api/types';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { onMount } from 'svelte';

	// BYOK provider allowlist — the "storefront" users attach their own API
	// keys against (spec: docs/superpowers/specs/2026-10-19-ai-byok-design.md).
	// Backend routes gated by ai.providers.view / ai.providers.manage.
	let {} = $props();

	const canView = $derived(hasAnyPermission(['ai.providers.view', 'ai.providers.manage']));
	const canManage = $derived(hasAnyPermission(['ai.providers.manage']));

	let providers = $state<AiByokProvider[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	// Create/Edit dialog state
	let createModalOpen = $state(false);
	let editModalOpen = $state(false);
	let editing = $state<AiByokProvider | null>(null);
	let saving = $state(false);
	let formError = $state('');

	let form = $state({
		provider_type: 'openai',
		display_name: '',
		modelsText: '', // one "id|Label" per line
		signup_url: '',
		enabled: true
	});

	// Delete confirmation state
	let confirmOpen = $state(false);
	let deleteTarget = $state<AiByokProvider | null>(null);
	let deleting = $state(false);

	// BYOK provider types — the backend CHECK constraint limits the
	// allowlist to these four (migration 000890).
	const providerTypeOptions = [
		{ value: 'openai', label: 'OpenAI' },
		{ value: 'anthropic', label: 'Anthropic' },
		{ value: 'groq', label: 'Groq' },
		{ value: 'perplexity', label: 'Perplexity' }
	];

	onMount(load);

	async function load() {
		loading = true;
		errorMessage = '';
		const res = await api('/control/ai-byok-providers');
		if (res.ok) {
			providers = (res.data as { data?: AiByokProvider[] })?.data ?? [];
		} else {
			errorMessage = res.error ?? t('common.load_failed', 'Failed to load');
		}
		loading = false;
	}

	function modelsToText(models: { id: string; label: string }[]): string {
		return models.map((m) => `${m.id}|${m.label}`).join('\n');
	}

	function textToModels(text: string): { id: string; label: string }[] | null {
		const out: { id: string; label: string }[] = [];
		for (const raw of text.split('\n')) {
			const line = raw.trim();
			if (!line) continue;
			const idx = line.indexOf('|');
			if (idx < 0) return null; // every line must be "id|Label"
			const id = line.slice(0, idx).trim();
			const label = line.slice(idx + 1).trim();
			if (!id) return null;
			out.push({ id, label: label || id });
		}
		return out;
	}

	function openCreate() {
		form = { provider_type: 'openai', display_name: '', modelsText: '', signup_url: '', enabled: true };
		formError = '';
		createModalOpen = true;
	}

	function openEdit(p: AiByokProvider) {
		editing = p;
		form = {
			provider_type: p.provider_type,
			display_name: p.display_name,
			modelsText: modelsToText(p.models),
			signup_url: p.signup_url,
			enabled: p.enabled
		};
		formError = '';
		editModalOpen = true;
	}

	async function save() {
		const models = textToModels(form.modelsText);
		if (!form.display_name.trim()) {
			formError = t('byok.err_name', 'Display name is required');
			return;
		}
		if (models === null) {
			formError = t('byok.err_models', 'Each model line must be "id|Label"');
			return;
		}
		if (form.signup_url && !form.signup_url.startsWith('https://')) {
			formError = t('byok.err_url', 'Signup URL must be https:// or empty');
			return;
		}

		saving = true;
		formError = '';
		const body = {
			provider_type: form.provider_type,
			display_name: form.display_name.trim(),
			models,
			signup_url: form.signup_url.trim(),
			enabled: form.enabled
		};
		const res = editing
			? await api(`/control/ai-byok-providers/${editing.id}`, { method: 'PATCH', body: JSON.stringify(body) })
			: await api('/control/ai-byok-providers', { method: 'POST', body: JSON.stringify(body) });

		if (res.ok) {
			createModalOpen = false;
			editModalOpen = false;
			editing = null;
			await load();
		} else {
			formError = res.error ?? t('common.save_failed', 'Failed to save');
		}
		saving = false;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		deleting = true;
		const res = await api(`/control/ai-byok-providers/${deleteTarget.id}`, { method: 'DELETE' });
		if (res.ok) {
			confirmOpen = false;
			deleteTarget = null;
			await load();
		} else {
			errorMessage = res.error ?? t('common.delete_failed', 'Failed to delete');
		}
		deleting = false;
	}

	const columns: DataTableColumn[] = [
		{ key: 'display_name', label: t('byok.col_name', 'Provider') },
		{ key: 'provider_type', label: t('byok.col_type', 'Type') },
		{
			key: 'models',
			label: t('byok.col_models', 'Models'),
			render: (_v: unknown, row: Record<string, unknown>) =>
				String(((row.models as { id: string }[] | undefined) ?? []).length)
		},
		{ key: 'signup_url', label: t('byok.col_signup', 'Signup URL') },
		{ key: 'referral_clicks', label: t('byok.col_clicks', 'Clicks') },
		{
			key: 'enabled',
			label: t('byok.col_enabled', 'Enabled'),
			render: (_v: unknown, row: Record<string, unknown>) => (row.enabled ? '✓' : '—')
		}
	];

	const rowActions = $derived<DataTableRowAction[]>(
		canManage
			? [
					{
						icon: Pencil,
						label: t('common.edit', 'Edit'),
						onClick: (row: Record<string, unknown>) => openEdit(row as unknown as AiByokProvider)
					},
					{
						icon: Trash2,
						label: t('common.delete', 'Delete'),
						variant: 'destructive',
						onClick: (row: Record<string, unknown>) => {
							deleteTarget = row as unknown as AiByokProvider;
							confirmOpen = true;
						}
					}
				]
			: []
	);
</script>

{#if !canView}
	<div class="p-8 text-center text-sm text-[var(--color-muted)]">
		{t('common.access_denied', 'Access denied')}
	</div>
{:else}
	<div class="space-y-4 p-4">
		<div class="flex items-start justify-between gap-4">
			<PageHeader
				title={t('nav.ai_byok_providers', 'AI BYOK Providers')}
				description={t('byok.subtitle', 'Allowlist of provider types users may attach their own API keys for')}
			/>
			{#if canManage}
				<button
					type="button"
					onclick={openCreate}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-[var(--color-primary-foreground)]"
				>
					<Plus class="h-4 w-4" />
					{t('byok.new', 'New BYOK provider')}
				</button>
			{/if}
		</div>

		{#if errorMessage}
			<p class="text-sm text-[var(--color-destructive)]">{errorMessage}</p>
		{/if}

		{#if loading}
			<p class="text-sm text-[var(--color-muted)]">{t('common.loading', 'Loading…')}</p>
		{:else}
			<DataTable {columns} rows={providers as unknown as Record<string, unknown>[]} {rowActions} />
		{/if}
	</div>
{/if}

<Modal bind:open={createModalOpen} title={t('byok.new', 'New BYOK provider')} size="lg">
	{@render formFields()}
</Modal>

<Modal bind:open={editModalOpen} title={t('byok.edit', 'Edit BYOK provider')} size="lg">
	{@render formFields()}
</Modal>

{#snippet formFields()}
	<div class="space-y-4">
		<div>
			<label for="bp-type" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('byok.col_type', 'Type')}</label
			>
			<AutocompleteSelect id="bp-type" bind:value={form.provider_type} options={providerTypeOptions} />
		</div>
		<div>
			<label for="bp-name" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('byok.col_name', 'Display name')}</label
			>
			<input
				id="bp-name"
				type="text"
				bind:value={form.display_name}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm"
			/>
		</div>
		<div>
			<label for="bp-models" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('byok.col_models', 'Models')} <span class="text-xs text-[var(--color-muted)]">({t('byok.models_hint', 'one per line: id|Label')})</span></label
			>
			<textarea
				id="bp-models"
				bind:value={form.modelsText}
				rows="5"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 font-mono text-sm"
				placeholder="gpt-4o|GPT-4o&#10;gpt-4o-mini|GPT-4o Mini"
			></textarea>
		</div>
		<div>
			<label for="bp-url" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('byok.col_signup', 'Signup URL')} <span class="text-xs text-[var(--color-muted)]">({t('byok.url_hint', 'affiliate/referral link, https only')})</span></label
			>
			<input
				id="bp-url"
				type="url"
				bind:value={form.signup_url}
				placeholder="https://…"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm"
			/>
		</div>
		<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
			<input type="checkbox" bind:checked={form.enabled} />
			{t('byok.col_enabled', 'Enabled')}
		</label>
		{#if formError}
			<p class="text-sm text-[var(--color-destructive)]">{formError}</p>
		{/if}
		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={() => {
					createModalOpen = false;
					editModalOpen = false;
					editing = null;
				}}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm"
				>{t('common.cancel', 'Cancel')}</button
			>
			<button
				type="button"
				onclick={save}
				disabled={saving}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-foreground)] disabled:opacity-50"
				>{saving ? t('common.saving', 'Saving…') : t('common.save', 'Save')}</button
			>
		</div>
	</div>
{/snippet}

<ConfirmModal
	bind:open={confirmOpen}
	title={t('byok.delete_title', 'Delete BYOK provider')}
	consequence={t('byok.delete_msg', 'Delete this provider? All user keys attached to it are removed too.')}
	confirmText={t('common.delete', 'Delete')}
	onConfirm={confirmDelete}
	onCancel={() => {
		confirmOpen = false;
		deleteTarget = null;
	}}
/>
