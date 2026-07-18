<script lang="ts">
	import { api, i18nAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, RefreshCw, Pencil, Trash2 } from 'lucide-svelte';
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

	// Search term wired into the i18n strings load
	let searchTerm = $state('');

	// Edit (upsert)
	let showEditModal = $state(false);
	let editTarget = $state<I18NString | null>(null);
	let editValue = $state('');
	let editSaving = $state(false);

	// Delete
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<I18NString | null>(null);
	let deleteSaving = $state(false);

	// Backend gates i18n string upsert/delete on i18n.manage
	const canManage = $derived(hasPermission('i18n.manage'));

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

	// Debounced search handler that refetches the strings list
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
			// Backend upserts by key+locale
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
	<!-- Debounced search input wired to the strings load -->
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
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('i18n.key')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('i18n.locale')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('i18n.value')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('i18n.updated')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions', 'Actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each strings as s (s.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-mono text-xs">{s.key}</td>
						<td class="px-4 py-3 text-xs">{s.locale}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{s.value.slice(0, 80)}{s.value.length > 80 ? '...' : ''}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(s.updated_at).toLocaleDateString()}</td>
						<td class="px-4 py-3">
							{#if canManage}
								<div class="flex items-center gap-1">
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-primary)" onclick={() => openEdit(s)}>
										<Pencil class="inline h-3 w-3" />
										{t('common.edit', 'Edit')}
									</button>
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-danger)" onclick={() => confirmDelete(s)}>
										<Trash2 class="inline h-3 w-3" />
										{t('common.delete', 'Delete')}
									</button>
								</div>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('i18n.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
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
