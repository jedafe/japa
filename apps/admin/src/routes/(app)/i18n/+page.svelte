<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { api, i18nAdminApiExtended } from '$lib/api';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { Globe, Plus, Download, RefreshCw, FileText } from 'lucide-svelte';

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
	let createModalOpen = $state(false);
	let editModalOpen = $state(false);
	let editingString = $state<TranslationEntry | null>(null);
	let deleteConfirmOpen = $state(false);
	let deletingTarget = $state<{ type: 'string'; id: string } | null>(null);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let creatingLocale = $state(false);

	let form = $state({ code: '', name: '', flag: '' });

	let tabs = $derived([
		{ value: 'languages', label: 'Languages', icon: Globe, count: languages.length },
		{ value: 'translations', label: 'Translations', icon: FileText, count: translations.length }
	]);

	const languageColumns = [
		{
			key: 'code',
			label: 'Code',
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-gray-900">${value}</span>`
		},
		{
			key: 'name',
			label: 'Language',
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-2">
					<span class="text-lg">${row.flag || '🌐'}</span>
					<span class="font-medium text-gray-900">${row.name}</span>
				</div>
			`
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			render: (_: any, row: any) => {
				const status = row.isDefault ? 'default' : row.isEnabled ? 'enabled' : 'disabled';
				const cls =
					status === 'default'
						? 'bg-primary-100 text-primary-800 border-primary-200'
						: status === 'enabled'
							? 'bg-success-100 text-success-800 border-success-200'
							: 'bg-gray-100 text-gray-800 border-gray-200';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${status}</span>`;
			}
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__deleteLanguage('${row.code}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">${t("common.delete")}</button>
				</div>
			`
		}
	];

	const translationColumns = [
		{
			key: 'key',
			label: 'Key',
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-gray-900">${value}</span>`
		},
		{
			key: 'value',
			label: 'Value',
			render: (_: any, row: any) => `
				<div class="max-w-md truncate text-sm text-gray-700">${row.value || '—'}</div>
			`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__editTranslation('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">${t("common.edit")}</button>
					<button onclick="window.__deleteTranslation('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">${t("common.delete")}</button>
				</div>
			`
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

	async function submitLanguage() {
		if (!form.code.trim() || !form.name.trim()) return;
		creatingLocale = true;
		errorMessage = '';
		// Provision a new locale by copying all English keys into the new locale.
		// The backend implicitly creates the locale once any translation row exists for it.
		const enResult = await i18nAdminApiExtended.listStrings('locale=en');
		if (!enResult.ok) {
			errorMessage = 'Failed to load source strings';
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
			createModalOpen = false;
			form = { code: '', name: '', flag: '' };
			successMessage = 'Language added successfully';
			loadLanguages();
		} else {
			errorMessage = res.error ?? 'Failed to add language';
		}
		creatingLocale = false;
	}

	function openEditModal(entry: TranslationEntry) {
		editingString = { ...entry };
		editModalOpen = true;
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
			editModalOpen = false;
			editingString = null;
			successMessage = 'Translation updated';
			loadTranslations();
		} else {
			errorMessage = res.error ?? 'Failed to update translation';
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
		const res = await api(`/admin/i18n/strings/${deletingTarget.id}`, {
			method: 'DELETE',
			body: { locale: selectedLocale }
		});
		if (res.ok) {
			successMessage = 'Translation deleted';
			loadTranslations();
		} else {
			errorMessage = res.error ?? 'Failed to delete';
		}
		deleteConfirmOpen = false;
		deletingTarget = null;
		saving = false;
	}

	$effect(() => {
		loadData();
	});

	$effect(() => {
		(window as any).__editTranslation = (id: string) => {
			const entry = translations.find((tr) => tr.id === id);
			if (entry) openEditModal(entry);
		};
		(window as any).__deleteTranslation = (id: string) => {
			openDeleteConfirm(id);
		};
		(window as any).__deleteLanguage = (code: string) => {
			// Deleting a locale is not supported by the backend; languages are derived
			// from existing translation rows. Admins remove a locale by clearing its strings.
			errorMessage = 'Language removal is managed by deleting its translation strings.';
		};
		return () => {
			delete (window as any).__editTranslation;
			delete (window as any).__deleteTranslation;
			delete (window as any).__deleteLanguage;
		};
	});
</script>

<svelte:head>
	<title>{t('app.i18n.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('app.i18n.title')}
	description={t('app.i18n.description')}
	actions={[
		{ label: t('app.i18n.add_language'), icon: Plus, onclick: () => (createModalOpen = true) },
		{ label: t('common.refresh'), icon: RefreshCw, variant: 'ghost', onclick: loadData }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="rounded-lg border border-success-200 bg-success-50 px-4 py-3 text-sm text-success-700">
			{successMessage}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'languages'}
		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-gray-500">Loading...</p>
				{:else}
					<DataTable
						columns={languageColumns}
						data={languages}
						pageSize={20}
						emptyMessage={t('app.i18n.no_languages')}
					/>
				{/if}
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === 'translations'}
		<div class="mb-4 flex items-center gap-3">
			<label for="locale-select" class="text-sm font-medium text-gray-700">Language</label>
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
					<p class="py-8 text-center text-sm text-gray-500">Loading...</p>
				{:else}
					<DataTable
						columns={translationColumns}
						data={translations}
						pageSize={50}
						emptyMessage={t('app.i18n.no_translations')}
					/>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Add Language Modal -->
<Modal bind:open={createModalOpen} title={t('app.i18n.add_language')} description={t('app.i18n.add_language_desc')}>
	<div class="space-y-4">
		<div>
			<label for="lang-code" class="mb-1 block text-sm font-medium text-gray-700">Language Code</label>
			<input
				id="lang-code"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., es"
				bind:value={form.code}
			/>
		</div>
		<div>
			<label for="lang-name" class="mb-1 block text-sm font-medium text-gray-700">Language Name</label>
			<input
				id="lang-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., Spanish"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="lang-flag" class="mb-1 block text-sm font-medium text-gray-700">Flag Emoji (optional)</label>
			<input
				id="lang-flag"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., 🇪🇸"
				bind:value={form.flag}
			/>
		</div>
		<p class="text-xs text-gray-500">{t('app.i18n.add_language_note')}</p>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (createModalOpen = false)}>${t("common.cancel")}</Button>
			<Button onclick={submitLanguage} disabled={creatingLocale || !form.code.trim() || !form.name.trim()}>
				{creatingLocale ? t('common.saving') : t('app.i18n.create')}
			</Button>
		</div>
	</div>
</Modal>

<!-- Edit Translation Modal -->
<Modal bind:open={editModalOpen} title={t('app.i18n.edit_translation')} description={t('app.i18n.edit_translation_desc')}>
	{#if editingString}
		<div class="space-y-4">
			<div>
				<label class="mb-1 block text-sm font-medium text-gray-700">Key</label>
				<p class="rounded-lg bg-gray-50 px-3 py-2 font-mono text-xs text-gray-600">{editingString.key}</p>
			</div>
			<div>
				<label for="edit-value" class="mb-1 block text-sm font-medium text-gray-700">Value ({editingString.locale})</label>
				<textarea
					id="edit-value"
					rows="4"
					class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
					bind:value={editingString.value}
				></textarea>
			</div>
			<div class="flex justify-end gap-3 pt-2">
				<Button variant="outline" onclick={() => (editModalOpen = false)}>${t("common.cancel")}</Button>
				<Button onclick={submitEditString} disabled={saving}>{saving ? t('common.saving') : t('common.save')}</Button>
			</div>
		</div>
	{/if}
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title={t('common.confirmDelete')} description={t('app.i18n.delete_confirm_desc')}>
	<div class="space-y-4">
		<p class="text-sm text-gray-600">
			{t('app.i18n.delete_confirm_body')}
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>${t("common.cancel")}</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? t('common.saving') : t('common.delete')}
			</Button>
		</div>
	</div>
</Modal>
