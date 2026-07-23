<script lang="ts">
	import { api, i18nAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, RefreshCw, Pencil, Trash2 } from 'lucide-svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import Modal from '$lib/components/Modal.svelte';

	type I18NString = {
		id: string;
		key: string;
		locale: string;
		value: string;
		updated_at: string;
	};

	let strings = $state<I18NString[]>([]);
	let languages = $state<Array<{ code: string; name: string }>>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let selectedLocale = $state('en');

	let searchTerm = $state('');

	let showEditModal = $state(false);
	let editTarget = $state<I18NString | null>(null);
	let editValue = $state('');
	let editSaving = $state(false);

	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<I18NString | null>(null);
	let deleteSaving = $state(false);

	const canManage = $derived(hasPermission('i18n.manage'));

	const filteredStrings = $derived(
		searchTerm
			? strings.filter(
					(s) =>
						s.key.toLowerCase().includes(searchTerm.toLowerCase()) ||
						s.locale.toLowerCase().includes(searchTerm.toLowerCase()) ||
						s.value.toLowerCase().includes(searchTerm.toLowerCase())
				)
			: strings
	);

	const columns = [
		{ key: 'key', label: t('i18n.key'), sortable: true,
			render: (v: string) => `<span class="font-mono text-xs">${v}</span>`
		},
		{ key: 'locale', label: t('i18n.locale'), sortable: true },
		{ key: 'value', label: t('i18n.value'), sortable: false,
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v.slice(0, 80)}${v.length > 80 ? '...' : ''}</span>`
		},
		{ key: 'updated_at', label: t('i18n.updated'), sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-24', component: TranslationActionsCell }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (searchTerm.trim()) params.set('q', searchTerm.trim());
			const qs = params.toString();
			const [stringsRes, langsRes] = await Promise.all([
				api<I18NString[]>(`/admin/i18n/strings${qs ? `?${qs}` : ''}`),
				api<Array<{ code: string; name: string }>>('/admin/i18n/languages')
			]);
			if (stringsRes.ok) strings = stringsRes.data ?? [];
			if (langsRes.ok) languages = langsRes.data ?? [];
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	function onSearchInput(e: Event) {
		searchTerm = (e.currentTarget as HTMLInputElement).value;
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => load(), 300);
	}

	function openEdit(s: I18NString) {
		editTarget = s;
		editValue = s.value;
		showEditModal = true;
	}

	function confirmDelete(s: I18NString) {
		deleteTarget = s;
		showDeleteConfirm = true;
	}

	async function saveEdit() {
		if (!editTarget) return;
		editSaving = true;
		try {
			const result = await api('/admin/i18n/strings', {
				method: 'POST',
				body: JSON.stringify({ key: editTarget.key, locale: editTarget.locale, value: editValue })
			});
			if (result.ok) {
				showEditModal = false;
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			editSaving = false;
		}
	}

	async function deleteString() {
		if (!deleteTarget) return;
		deleteSaving = true;
		try {
			const result = await api(`/admin/i18n/strings/${deleteTarget.id}`, { method: 'DELETE' });
			if (result.ok) {
				showDeleteConfirm = false;
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			deleteSaving = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('i18n.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('i18n.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('i18n.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center justify-between gap-2">
	<input
		type="search"
		placeholder={t('common.search')}
		oninput={onSearchInput}
		aria-label={t('common.search')}
		class="w-full max-w-xs rounded-lg border p-2 text-sm outline-none focus:ring-2"
		style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
	/>
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		{columns}
		data={filteredStrings}
		pageSize={25}
		emptyMessage={t('i18n.empty')}
	/>
{/if}

<Modal bind:open={showEditModal} title={t('i18n.edit_title', 'Edit Translation')}>
	{#if editTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('i18n.key', 'Key')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{editTarget.key}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('i18n.locale', 'Locale')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{editTarget.locale}</p>
			</div>
			<div>
				<label for="editValue" class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('i18n.value', 'Value')}</label>
				<textarea id="editValue" bind:value={editValue} rows="4" class="mt-1 w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"></textarea>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showEditModal = false)}>{t('common.cancel', 'Cancel')}</button>
		<button class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50" style="background-color: var(--color-primary)" onclick={saveEdit} disabled={editSaving}>
			{editSaving ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
		</button>
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title={t('i18n.delete_title', 'Delete Translation')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">{t('i18n.delete_confirm', 'Are you sure you want to delete this translation string? This action cannot be undone.')}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</button>
		<button class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50" style="background-color: var(--color-danger)" onclick={deleteString} disabled={deleteSaving}>
			{deleteSaving ? t('common.deleting', 'Deleting…') : t('common.delete', 'Delete')}
		</button>
	</div>
</Modal>

{#snippet TranslationActionsCell({ value, row }: { value: any; row: I18NString })}
	{#if canManage}
		<div class="flex items-center gap-1">
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="inline h-3 w-3" />
				{t('common.edit', 'Edit')}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="inline h-3 w-3" />
				{t('common.delete', 'Delete')}
			</Button>
		</div>
	{/if}
{/snippet}
