<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { page } from "$app/state";
	import { goto } from "$app/navigation";
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { FileText, Plus, Eye, BarChart3, RefreshCw } from "lucide-svelte";
	import { api } from "$lib/api";

	type Form = {
		id: string;
		title: string;
		slug: string;
		space_id?: string;
		space_name?: string;
		submission_count: number;
		is_active: boolean;
		fields_count?: number;
		created_at: string;
		updated_at?: string;
	};

	let { data } = $props();

	let activeTab = $state("all");
	let forms = $state<Form[]>([]);
	let loading = $state(true);
	let filters = $state<Record<string, string>>({});

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Form | null>(null);
	let deleting = $state(false);

	const tabs = [
		{ value: "all", label: "All Forms", icon: FileText },
		{ value: "active", label: "Active", icon: Eye },
		{ value: "inactive", label: "Inactive", icon: FileText }
	];

	const columns = [
		{
			key: "title",
			label: "Form",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-gray-900">${row.title}</p>
					<p class="text-xs font-mono text-gray-500">${row.slug}</p>
				</div>
			`
		},
		{
			key: "space_name",
			label: "Space",
			sortable: true,
			render: (value: string) => `<span class="text-sm text-gray-700">${value || "Global"}</span>`
		},
		{
			key: "submission_count",
			label: "Submissions",
			sortable: true,
			render: (value: number) => `<span class="font-medium text-gray-900">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: "fields_count",
			label: "Fields",
			sortable: true,
			render: (value: number) => `<span class="text-sm text-gray-700">${value ?? "—"}</span>`
		},
		{
			key: "is_active",
			label: "Status",
			sortable: true,
			render: (value: boolean) => {
				const variant = value ? "success" : "secondary";
				const classes = value ? "bg-success-100 text-success-800 border-success-200" : "bg-gray-100 text-gray-800 border-gray-200";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes}">${value ? "Active" : "Inactive"}</span>`;
			}
		},
		{
			key: "created_at",
			label: "Created",
			sortable: true,
			render: (value: string) => new Date(value).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" })
		},
		{
			key: "actions",
			label: "",
			class: "w-32",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__formsView('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">${t("common.view")}</button>
					<button onclick="window.__formsSubmissions('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-primary-700 hover:bg-primary-50">Data</button>
					<button onclick="window.__formsDelete('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">${t("common.delete")}</button>
				</div>
			`
		}
	];

	let filteredForms = $derived.by(() => {
		let result = [...forms];
		if (activeTab === "active") result = result.filter((f) => f.is_active);
		if (activeTab === "inactive") result = result.filter((f) => !f.is_active);
		return result;
	});

	async function loadForms() {
		loading = true;
		try {
			const result = await api<Form[]>("/forms");
			if (result.ok) {
				forms = result.data ?? [];
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filters = values;
	}

	function confirmDelete(form: Form) {
		deleteTarget = form;
		showDeleteModal = true;
	}

	async function deleteForm() {
		if (!deleteTarget) return;
		deleting = true;
		try {
			await api(`/forms/${deleteTarget.id}`, { method: "DELETE" });
			forms = forms.filter((f) => f.id !== deleteTarget!.id);
			showDeleteModal = false;
			deleteTarget = null;
		} catch {
			// API not available
		} finally {
			deleting = false;
		}
	}

	// Expose window functions for DataTable inline actions
	$effect(() => {
		(window as any).__formsView = (id: string) => {
			goto(`/forms/${id}`);
		};
		(window as any).__formsSubmissions = (id: string) => {
			goto(`/forms/${id}/submissions`);
		};
		(window as any).__formsDelete = (id: string) => {
			const form = forms.find((f) => f.id === id);
			if (form) confirmDelete(form);
		};
	});

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadForms();
		}
	});
</script>

<svelte:head>
	<title>Forms — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Forms"
	description="Manage platform forms and collect user submissions"
	actions={[
		{ label: "Refresh", icon: RefreshCw, onclick: loadForms, variant: "outline" }
	]}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[
			{
				key: "space",
				label: "Space",
				options: [
					{ value: "global", label: "Global" },
					{ value: "space", label: "Space-specific" }
				]
			}
		]}
		onsearch={handleFilter}
		onclear={() => { filters = {}; }}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-gray-500">{t('forms.loading_forms')}</p>
			{:else}
				<DataTable
					columns={columns}
					data={filteredForms}
					pageSize={20}
					emptyMessage="No forms found"
				/>
			{/if}
		</CardContent>
	</Card>

	<!-- Summary Stats -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-gray-500">{t('forms.total_forms')}</p>
				<p class="text-2xl font-bold text-gray-900">{forms.length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-gray-500">{t('forms.active_forms')}</p>
				<p class="text-2xl font-bold text-gray-900">{forms.filter((f) => f.is_active).length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-gray-500">{t('forms.total_submissions')}</p>
				<p class="text-2xl font-bold text-gray-900">{forms.reduce((sum, f) => sum + (f.submission_count ?? 0), 0).toLocaleString()}</p>
			</CardContent>
		</Card>
	</div>
</div>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Delete Form" description="This action cannot be undone. All submissions will be lost." size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-danger-50 p-4">
			<p class="text-sm text-danger-700">
				Are you sure you want to delete <strong>{deleteTarget.title}</strong>?
				This will permanently remove the form and all its submissions.
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('forms.cancel')}</Button>
		<Button variant="destructive" onclick={deleteForm} disabled={deleting}>
			{#if deleting}
				Deleting...
			{:else}
				Delete Form
			{/if}
		</Button>
	</div>
</Modal>
