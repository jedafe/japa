<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { api, i18nAdminApiExtended } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { Globe, Plus, Download, RefreshCw, FileText, Eye, Pencil, Trash2 } from 'lucide-svelte';

	type Language = {
		code: string;
		name: string;
		flag?: string;
		completion?: number;
		isDefault: boolean;
		isEnabled: boolean;
	};

	type TranslationEntry = {
		id: string;
		key: string;
		locale: string;
		value: string;
	};

	let languages = $state<Language[]>([]);
	let translations = $state<TranslationEntry[]>([]);
	let loading = $state(true);
	let activeTab = $state('languages');
	let selectedLocale = $state('en');
	let createSlideOverOpen = $state(false);
	let editSlideOverOpen = $state(false);
	let editingString = $state<TranslationEntry | null>(null);
	let deleteConfirmOpen = $state(false);
	let deletingTarget = $state<{ type: 'string'; id: string } | null>(null);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let creatingLocale = $state(false);

	let selectedLanguageIds = $state<Set<string>>(new Set());
	let selectedTranslationIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	let showViewSlideOver = $state(false);
	let viewLanguage = $state<Language | null>(null);
	let viewString = $state<TranslationEntry | null>(null);

	const canManageI18n = $derived(hasPermission("i18n.manage"));

	let form = $state({ code: '', name: '', flag: '' });

	let tabs = $derived([
		{ value: 'languages', label: t('app.i18n.tab_languages', 'Languages'), icon: Globe, count: languages.length },
		{ value: 'translations', label: t('app.i18n.tab_translations', 'Translations'), icon: FileText, count: translations.length }
	]);

	const languageColumns = [
		{
			key: 'code',
			label: t('app.i18n.code', 'Code'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-foreground">${value}</span>`
		},
		{
			key: 'name',
			label: t('app.i18n.language', 'Language'),
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-2">
					<span class="text-lg">${row.flag || '🌐'}</span>
					<span class="font-medium text-foreground">${row.name}</span>
				</div>
			`
		},
		{
			key: 'status',
			label: t('common.status', 'Status'),
			sortable: true,
			render: (_: any, row: any) => {
				const rawStatus = row.isDefault ? 'default' : row.isEnabled ? 'enabled' : 'disabled';
				const labels: Record<string, string> = {
					default: t('app.i18n.status_default', 'default'),
					enabled: t('app.i18n.status_enabled', 'enabled'),
					disabled: t('app.i18n.status_disabled', 'disabled')
				};
				const status = labels[rawStatus] ?? rawStatus;
				const cls =
					rawStatus === 'default'
						? 'bg-primary-100 text-primary-800 border-primary-200'
						: rawStatus === 'enabled'
							? 'bg-success/15 text-success border-success'
							: 'bg-surface text-foreground border';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${status}</span>`;
			}
		}
	];

	const languageRowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Language) => openLanguageView(row) }
	];

	const translationColumns = [
		{
			key: 'key',
			label: t('app.i18n.key', 'Key'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-foreground">${value}</span>`
		},
		{
			key: 'value',
			label: t('app.i18n.value', 'Value'),
			render: (_: any, row: any) => `
				<div class="max-w-md truncate text-sm text-foreground-alt">${row.value || '—'}</div>
			`
		}
	];

	const translationRowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: TranslationEntry) => openStringView(row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: TranslationEntry) => openEditModal(row),
			show: () => canManageI18n
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: TranslationEntry) => openDeleteConfirm(row.id),
			show: () => canManageI18n
		}
	];

	async function loadLanguages() {
		const result = await i18nAdminApiExtended.listLanguages();
		if (result.ok) {
			const data = (result.data as Array<{ locale: string; count: number }>) ?? [];
			languages = data.map((l) => ({
				code: l.locale,
				name: l.locale.toUpperCase(),
				isDefault: l.locale === 'en',
				isEnabled: true,
				completion: l.count
			}));
		}
	}

	async function loadTranslations() {
		const result = await i18nAdminApiExtended.listStrings(`locale=${selectedLocale}`);
		if (result.ok) {
			translations = (result.data as TranslationEntry[]) ?? [];
		}
	}

	async function loadData() {
		loading = true;
		await Promise.all([loadLanguages(), loadTranslations()]);
		loading = false;
	}

	function openLanguageView(lang: Language) {
		viewLanguage = lang;
		showViewSlideOver = true;
	}

	function openStringView(entry: TranslationEntry) {
		viewString = entry;
		showViewSlideOver = true;
	}

	async function submitLanguage() {
		if (!form.code.trim() || !form.name.trim()) return;
		creatingLocale = true;
		errorMessage = '';
		// Provision a new locale by copying all English keys into the new locale.
		// The backend implicitly creates the locale once any translation row exists for it.
		const enResult = await i18nAdminApiExtended.listStrings('locale=en');
		if (!enResult.ok) {
			errorMessage = t('app.i18n.load_source_error', 'Failed to load source strings');
			creatingLocale = false;
			return;
		}
		const enStrings = (enResult.data as TranslationEntry[]) ?? [];
		const batch = enStrings.map((s) => ({
			locale: form.code.trim(),
			key: s.key,
			value: s.value
		}));
		const res = await i18nAdminApiExtended.batchCreate({ translations: batch });
		if (res.ok) {
			createSlideOverOpen = false;
			form = { code: '', name: '', flag: '' };
			successMessage = t('app.i18n.language_added', 'Language added successfully');
			loadLanguages();
		} else {
			errorMessage = res.error ?? t('app.i18n.add_language_error', 'Failed to add language');
		}
		creatingLocale = false;
	}

	function openEditModal(entry: TranslationEntry) {
		editingString = { ...entry };
		editSlideOverOpen = true;
	}

	async function submitEditString() {
		if (!editingString) return;
		saving = true;
		errorMessage = '';
		const res = await i18nAdminApiExtended.createString({
			locale: editingString.locale,
			key: editingString.key,
			value: editingString.value
		});
		if (res.ok) {
			editSlideOverOpen = false;
			editingString = null;
			successMessage = t('app.i18n.translation_updated', 'Translation updated');
			loadTranslations();
		} else {
			errorMessage = res.error ?? t('app.i18n.update_error', 'Failed to update translation');
		}
		saving = false;
	}

	function openDeleteConfirm(id: string) {
		deletingTarget = { type: 'string', id };
		deleteConfirmOpen = true;
	}

	async function confirmDelete() {
		if (!deletingTarget) return;
		saving = true;
		const res = await api(`/admin/i18n/strings/${deletingTarget.id}?locale=${encodeURIComponent(selectedLocale)}`, {
			method: 'DELETE'
		});
		if (res.ok) {
			successMessage = t('app.i18n.translation_deleted', 'Translation deleted');
			loadTranslations();
		} else {
			errorMessage = res.error ?? t('app.i18n.delete_error', 'Failed to delete');
		}
		deleteConfirmOpen = false;
		deletingTarget = null;
		saving = false;
	}

	async function bulkDeleteLanguages() {
		if (selectedLanguageIds.size === 0) return;
		bulkDeleting = true;
		errorMessage = '';
		try {
			for (const code of selectedLanguageIds) {
				await api(`/admin/i18n/languages/${code}`, { method: 'DELETE' });
			}
			selectedLanguageIds = new Set();
			loadLanguages();
		} catch {
			errorMessage = t('app.i18n.delete_error', 'Failed to delete');
		}
		bulkDeleting = false;
	}

	async function bulkDeleteTranslations() {
		if (selectedTranslationIds.size === 0) return;
		bulkDeleting = true;
		errorMessage = '';
		try {
			for (const id of selectedTranslationIds) {
				await api(`/admin/i18n/strings/${id}?locale=${encodeURIComponent(selectedLocale)}`, { method: 'DELETE' });
			}
			selectedTranslationIds = new Set();
			loadTranslations();
		} catch {
			errorMessage = t('app.i18n.delete_error', 'Failed to delete');
		}
		bulkDeleting = false;
	}

	$effect(() => {
		loadData();
	});

	$effect(() => {
		if (successMessage) {
			const msg = successMessage;
			setTimeout(() => { if (successMessage === msg) successMessage = ''; }, 4000);
		}
		if (errorMessage) {
			const msg = errorMessage;
			setTimeout(() => { if (errorMessage === msg) errorMessage = ''; }, 6000);
		}
	});
</script>

<svelte:head>
	<title>{t('app.i18n.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('app.i18n.title')}
	description={t('app.i18n.description')}
	actions={[
		...(canManageI18n
			? [{ label: t('app.i18n.add_language'), icon: Plus, onclick: () => (createSlideOverOpen = true) }]
			: []),
		{ label: t('common.refresh'), icon: RefreshCw, variant: 'ghost', onclick: loadData }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="rounded-lg border border-success bg-success-50 px-4 py-3 text-sm text-success-700">
			{successMessage}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'languages'}
		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-muted">{t('common.loading', 'Loading...')}</p>
				{:else}
					<DataTable
						columns={languageColumns}
						data={languages}
						actions={languageRowActions}
						pageSize={20}
						emptyMessage={t('app.i18n.no_languages')}
						selectable
						bind:selectedIds={selectedLanguageIds}
					>
						{#snippet bulkActions()}
							<Button variant="destructive" size="sm" onclick={bulkDeleteLanguages} disabled={bulkDeleting || selectedLanguageIds.size === 0}>
								<Trash2 class="h-3.5 w-3.5 mr-1" />
								{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.deleteSelected', 'Delete Selected')} ({selectedLanguageIds.size})
							</Button>
						{/snippet}
					</DataTable>
				{/if}
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === 'translations'}
		<div class="mb-4 flex items-center gap-3">
			<label for="locale-select" class="text-sm font-medium text-foreground-alt">{t('app.i18n.language', 'Language')}</label>
			<Select
				options={languages.map((l) => ({ value: l.code, label: l.name }))}
				value={selectedLocale}
				onchange={(val: string) => {
					selectedLocale = val;
					loadTranslations();
				}}
			/>
		</div>

		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-muted">{t('common.loading', 'Loading...')}</p>
				{:else}
					<DataTable
						columns={translationColumns}
						data={translations}
						actions={translationRowActions}
						pageSize={50}
						emptyMessage={t('app.i18n.no_translations')}
						selectable
						bind:selectedIds={selectedTranslationIds}
					>
						{#snippet bulkActions()}
							<Button variant="destructive" size="sm" onclick={bulkDeleteTranslations} disabled={bulkDeleting || selectedTranslationIds.size === 0}>
								<Trash2 class="h-3.5 w-3.5 mr-1" />
								{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.deleteSelected', 'Delete Selected')} ({selectedTranslationIds.size})
							</Button>
						{/snippet}
					</DataTable>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Add Language SlideOver -->
<SlideOver bind:open={createSlideOverOpen} title={t('app.i18n.add_language')}>
	<div class="space-y-4">
		<div>
			<label for="lang-code" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.i18n.language_code', 'Language Code')}</label>
			<input
				id="lang-code"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.i18n.code_placeholder', 'e.g., es')}
				bind:value={form.code}
			/>
		</div>
		<div>
			<label for="lang-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.i18n.language_name', 'Language Name')}</label>
			<input
				id="lang-name"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder={t('app.i18n.name_placeholder', 'e.g., Spanish')}
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="lang-flag" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.i18n.flag_emoji', 'Flag Emoji (optional)')}</label>
			<input
				id="lang-flag"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="e.g., 🇪🇸"
				bind:value={form.flag}
			/>
		</div>
		<p class="text-xs text-muted">{t('app.i18n.add_language_note')}</p>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (createSlideOverOpen = false)}>{t("common.cancel")}</Button>
			<Button onclick={submitLanguage} disabled={creatingLocale || !form.code.trim() || !form.name.trim()}>
				{creatingLocale ? t('common.saving') : t('app.i18n.create')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Edit Translation SlideOver -->
<SlideOver bind:open={editSlideOverOpen} title={t('app.i18n.edit_translation')}>
	{#if editingString}
		<div class="space-y-4">
			<div>
				<label for="edit-key" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.i18n.key', 'Key')}</label>
				<p id="edit-key" class="rounded-lg bg-surface px-3 py-2 font-mono text-xs text-foreground-alt">{editingString.key}</p>
			</div>
			<div>
				<label for="edit-value" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.i18n.value', 'Value')} ({editingString.locale})</label>
				<textarea
					id="edit-value"
					rows="4"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={editingString.value}
				></textarea>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (editSlideOverOpen = false)}>{t("common.cancel")}</Button>
			<Button onclick={submitEditString} disabled={saving}>{saving ? t('common.saving') : t('common.save')}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title={t('common.confirmDelete')} description={t('app.i18n.delete_confirm_desc')}>
	<div class="space-y-4">
		<p class="text-sm text-foreground-alt">
			{t('app.i18n.delete_confirm_body')}
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>{t("common.cancel")}</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? t('common.saving') : t('common.delete')}
			</Button>
		</div>
	</div>
</Modal>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t('common.details', 'Details')}>
	{#if viewLanguage}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.i18n.code', 'Code')}</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewLanguage.code}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.i18n.language', 'Language')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewLanguage.flag || '🌐'} {viewLanguage.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.i18n.status', 'Status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewLanguage.isDefault ? t('app.i18n.status_default', 'Default') : viewLanguage.isEnabled ? t('app.i18n.status_enabled', 'Enabled') : t('app.i18n.status_disabled', 'Disabled')}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.i18n.strings', 'Strings')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewLanguage.completion ?? 0).toLocaleString()}</p>
				</div>
			</div>
		</div>
	{:else if viewString}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.i18n.key', 'Key')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewString.key}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.i18n.value', 'Value')} ({viewString.locale})</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewString.value || '—'}</p>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (showViewSlideOver = false)}>{t('common.close', 'Close')}</Button>
		</div>
	{/snippet}
</SlideOver>
