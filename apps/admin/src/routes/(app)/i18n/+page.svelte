<script lang="ts">
	import { api } from "$lib/api";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Select } from "$lib/components/ui/select";
	import { Globe, Plus, Download, RefreshCw, FileText } from "lucide-svelte";

	type Language = {
		code: string;
		name: string;
		flag?: string;
		completion?: number;
		isDefault: boolean;
		isEnabled: boolean;
	};

	type TranslationEntry = {
		key: string;
		translations: Record<string, string>;
	};

	let languages = $state<Language[]>([]);
	let translations = $state<TranslationEntry[]>([]);
	let loading = $state(true);
	let activeTab = $state("languages");
	let filters = $state<Record<string, string>>({});
	let createModalOpen = $state(false);
	let editModalOpen = $state(false);
	let editingLanguage = $state<Language | null>(null);
	let deleteConfirmOpen = $state(false);
	let deletingTarget = $state<{ type: "language" | "translation"; key: string } | null>(null);
	let saving = $state(false);
	let errorMessage = $state("");
	let successMessage = $state("");

	let form = $state({ code: "", name: "", flag: "" });

	let tabs = $derived([
		{ value: "languages", label: "Languages", icon: Globe, count: languages.length },
		{ value: "translations", label: "Translations", icon: FileText, count: translations.length }
	]);

	const languageColumns = [
		{
			key: "code",
			label: "Code",
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-gray-900">${value}</span>`
		},
		{
			key: "name",
			label: "Language",
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-2">
					<span class="text-lg">${row.flag || "🌐"}</span>
					<span class="font-medium text-gray-900">${row.name}</span>
				</div>
			`
		},
		{
			key: "completion",
			label: "Completion",
			sortable: true,
			render: (value: number) => `
				<div class="flex items-center gap-2">
					<div class="h-2 w-24 rounded-full bg-gray-200">
						<div class="h-2 rounded-full bg-primary-600" style="width: ${value ?? 0}%"></div>
					</div>
					<span class="text-sm text-gray-600">${value ?? 0}%</span>
				</div>
			`
		},
		{
			key: "status",
			label: "Status",
			sortable: true,
			render: (_: any, row: any) => {
				const status = row.isDefault ? "default" : row.isEnabled ? "enabled" : "disabled";
				const cls =
					status === "default"
						? "bg-primary-100 text-primary-800 border-primary-200"
						: status === "enabled"
							? "bg-success-100 text-success-800 border-success-200"
							: "bg-gray-100 text-gray-800 border-gray-200";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${status}</span>`;
			}
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__editLanguage('${row.code}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
					<button onclick="window.__deleteLanguage('${row.code}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
				</div>
			`
		}
	];

	const translationColumns = [
		{
			key: "key",
			label: "Key",
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-gray-900">${value}</span>`
		},
		{
			key: "translations",
			label: "Translations",
			render: (_: any, row: any) => `
				<div class="flex flex-wrap gap-1">
					${Object.entries(row.translations || {})
						.map(
							([lang, val]) =>
								`<span class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-700">${lang}: ${val || "—"}</span>`
						)
						.join("")}
				</div>
			`
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__editTranslation('${row.key}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
					<button onclick="window.__deleteTranslation('${row.key}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
				</div>
			`
		}
	];

	async function loadLanguages() {
		const result = await api<Language[]>("/i18n/languages");
		if (result.ok) {
			languages = Array.isArray(result.data) ? result.data : [];
		}
	}

	async function loadTranslations() {
		const result = await api<TranslationEntry[]>("/i18n/translations");
		if (result.ok) {
			translations = Array.isArray(result.data) ? result.data : [];
		}
	}

	async function loadData() {
		loading = true;
		await Promise.all([loadLanguages(), loadTranslations()]);
		loading = false;
	}

	async function submitLanguage() {
		if (!form.code.trim() || !form.name.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await api("/i18n/languages", {
			method: "POST",
			body: {
				code: form.code.trim(),
				name: form.name.trim(),
				flag: form.flag.trim() || undefined
			}
		});
		if (result.ok) {
			createModalOpen = false;
			form = { code: "", name: "", flag: "" };
			successMessage = "Language added successfully";
			loadLanguages();
		} else {
			errorMessage = result.error ?? "Failed to add language";
		}
		saving = false;
	}

	function openEditModal(code: string) {
		const lang = languages.find((l) => l.code === code);
		if (lang) {
			editingLanguage = lang;
			form = { code: lang.code, name: lang.name, flag: lang.flag ?? "" };
			editModalOpen = true;
		}
	}

	async function submitEditLanguage() {
		if (!editingLanguage || !form.name.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await api(`/i18n/languages/${editingLanguage.code}`, {
			method: "PUT",
			body: { name: form.name.trim(), flag: form.flag.trim() || undefined }
		});
		if (result.ok) {
			editModalOpen = false;
			editingLanguage = null;
			form = { code: "", name: "", flag: "" };
			successMessage = "Language updated successfully";
			loadLanguages();
		} else {
			errorMessage = result.error ?? "Failed to update language";
		}
		saving = false;
	}

	function openDeleteConfirm(code: string) {
		deletingTarget = { type: "language", key: code };
		deleteConfirmOpen = true;
	}

	async function confirmDelete() {
		if (!deletingTarget) return;
		saving = true;
		const endpoint =
			deletingTarget.type === "language"
				? `/i18n/languages/${deletingTarget.key}`
				: `/i18n/translations/${encodeURIComponent(deletingTarget.key)}`;
		const result = await api(endpoint, { method: "DELETE" });
		if (result.ok) {
			successMessage = `${deletingTarget.type === "language" ? "Language" : "Translation"} deleted`;
			if (deletingTarget.type === "language") loadLanguages();
			else loadTranslations();
		} else {
			errorMessage = result.error ?? "Failed to delete";
		}
		deleteConfirmOpen = false;
		deletingTarget = null;
		saving = false;
	}

	$effect(() => {
		loadData();
	});

	$effect(() => {
		(window as any).__editLanguage = openEditModal;
		(window as any).__deleteLanguage = openDeleteConfirm;
		(window as any).__editTranslation = (key: string) => {
			/* TODO: open translation edit modal */
		};
		(window as any).__deleteTranslation = (key: string) => {
			deletingTarget = { type: "translation", key };
			deleteConfirmOpen = true;
		};
		return () => {
			delete (window as any).__editLanguage;
			delete (window as any).__deleteLanguage;
			delete (window as any).__editTranslation;
			delete (window as any).__deleteTranslation;
		};
	});
</script>

<svelte:head>
	<title>Translations — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Translations"
	description="Manage languages and translation keys"
	actions={[
		{ label: "Add Language", icon: Plus, onclick: () => (createModalOpen = true) },
		{ label: "Export", icon: Download, variant: "outline" },
		{ label: "Refresh", icon: RefreshCw, variant: "ghost", onclick: loadData }
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

	{#if activeTab === "languages"}
		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-gray-500">Loading...</p>
				{:else}
					<DataTable
						columns={languageColumns}
						data={languages}
						pageSize={20}
						emptyMessage="No languages configured"
					/>
				{/if}
			</CardContent>
		</Card>
	{/if}

	{#if activeTab === "translations"}
		<FilterBar
			bind:value={filters}
			filters={[
				{
					key: "language",
					label: "Language",
					options: languages.map((l) => ({ value: l.code, label: l.name }))
				}
			]}
		/>

		<Card>
			<CardContent class="p-0">
				{#if loading}
					<p class="py-8 text-center text-sm text-gray-500">Loading...</p>
				{:else}
					<DataTable
						columns={translationColumns}
						data={translations}
						pageSize={50}
						emptyMessage="No translations found"
					/>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Add Language Modal -->
<Modal bind:open={createModalOpen} title="Add Language" description="Add a new language to the platform">
	<div class="space-y-4">
		<div>
			<label for="lang-code" class="mb-1 block text-sm font-medium text-gray-700">Language Code</label>
			<input
				id="lang-code"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., fr"
				bind:value={form.code}
			/>
		</div>
		<div>
			<label for="lang-name" class="mb-1 block text-sm font-medium text-gray-700">Language Name</label>
			<input
				id="lang-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., French"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="lang-flag" class="mb-1 block text-sm font-medium text-gray-700">Flag Emoji (optional)</label>
			<input
				id="lang-flag"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., 🇫🇷"
				bind:value={form.flag}
			/>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (createModalOpen = false)}>Cancel</Button>
			<Button onclick={submitLanguage} disabled={saving || !form.code.trim() || !form.name.trim()}>
				{saving ? "Adding..." : "Add Language"}
			</Button>
		</div>
	</div>
</Modal>

<!-- Edit Language Modal -->
<Modal bind:open={editModalOpen} title="Edit Language" description="Update language details">
	<div class="space-y-4">
		<div>
			<label for="edit-lang-code" class="mb-1 block text-sm font-medium text-gray-700">Language Code</label>
			<input
				id="edit-lang-code"
				type="text"
				class="w-full rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-500"
				value={editingLanguage?.code}
				disabled
			/>
		</div>
		<div>
			<label for="edit-lang-name" class="mb-1 block text-sm font-medium text-gray-700">Language Name</label>
			<input
				id="edit-lang-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., French"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="edit-lang-flag" class="mb-1 block text-sm font-medium text-gray-700">Flag Emoji (optional)</label>
			<input
				id="edit-lang-flag"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., 🇫🇷"
				bind:value={form.flag}
			/>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => (editModalOpen = false)}>Cancel</Button>
			<Button onclick={submitEditLanguage} disabled={saving || !form.name.trim()}>
				{saving ? "Saving..." : "Save Changes"}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title="Confirm Delete" description="This action cannot be undone">
	<div class="space-y-4">
		<p class="text-sm text-gray-600">
			Are you sure you want to delete this {deletingTarget?.type}? All associated data will be permanently removed.
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>Cancel</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? "Deleting..." : "Delete"}
			</Button>
		</div>
	</div>
</Modal>
