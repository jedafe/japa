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
	import { Download, Plus, RefreshCw, Clock, CheckCircle, XCircle, FileDown } from "lucide-svelte";

	type DataExport = {
		id: string;
		name: string;
		type: string;
		format: string;
		status: "pending" | "processing" | "completed" | "failed";
		fileSize: number | null;
		fileUrl: string | null;
		createdAt: string;
		completedAt: string | null;
		errorMessage: string | null;
	};

	let exports = $state<DataExport[]>([]);
	let loading = $state(true);
	let activeTab = $state("all");
	let filters = $state<Record<string, string>>({});
	let createModalOpen = $state(false);
	let deleteConfirmOpen = $state(false);
	let deletingId = $state<string | null>(null);
	let saving = $state(false);
	let errorMessage = $state("");
	let successMessage = $state("");

	let form = $state({
		name: "",
		type: "users",
		format: "csv",
		includeMetadata: true
	});

	const exportTypes = [
		{ value: "users", label: "Users" },
		{ value: "posts", label: "Posts" },
		{ value: "comments", label: "Comments" },
		{ value: "orders", label: "Orders" },
		{ value: "transactions", label: "Transactions" },
		{ value: "analytics", label: "Analytics" },
		{ value: "moderation", label: "Moderation Logs" }
	];

	const formatOptions = [
		{ value: "csv", label: "CSV" },
		{ value: "json", label: "JSON" },
		{ value: "xlsx", label: "Excel (XLSX)" }
	];

	let tabs = $derived([
		{ value: "all", label: "All Exports", icon: Download, count: exports.length },
		{ value: "active", label: "In Progress", icon: Clock, count: exports.filter((e) => e.status === "pending" || e.status === "processing").length },
		{ value: "completed", label: "Completed", icon: CheckCircle, count: exports.filter((e) => e.status === "completed").length },
		{ value: "failed", label: "Failed", icon: XCircle, count: exports.filter((e) => e.status === "failed").length }
	]);

	let filteredExports = $derived.by(() => {
		let result = [...exports];
		if (activeTab === "active") result = result.filter((e) => e.status === "pending" || e.status === "processing");
		if (activeTab === "completed") result = result.filter((e) => e.status === "completed");
		if (activeTab === "failed") result = result.filter((e) => e.status === "failed");
		if (filters.type) result = result.filter((e) => e.type === filters.type);
		return result;
	});

	function formatFileSize(bytes: number | null): string {
		if (!bytes) return "—";
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function statusVariant(status: string) {
		switch (status) {
			case "completed": return "success";
			case "failed": return "destructive";
			case "processing": return "warning";
			default: return "secondary";
		}
	}

	const exportColumns = [
		{
			key: "name",
			label: "Name",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-gray-900">${row.name}</p>
					<p class="text-xs text-gray-500">${row.type} · ${row.format.toUpperCase()}</p>
				</div>
			`
		},
		{
			key: "status",
			label: "Status",
			sortable: true,
			render: (value: string) => {
				const variant = statusVariant(value);
				const cls =
					variant === "success"
						? "bg-success-100 text-success-800 border-success-200"
						: variant === "destructive"
							? "bg-danger-100 text-danger-800 border-danger-200"
							: variant === "warning"
								? "bg-warning-100 text-warning-800 border-warning-200"
								: "bg-gray-100 text-gray-800 border-gray-200";
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${value.charAt(0).toUpperCase() + value.slice(1)}</span>`;
			}
		},
		{
			key: "fileSize",
			label: "Size",
			sortable: true,
			render: (value: number | null) => `<span class="text-sm text-gray-600">${formatFileSize(value)}</span>`
		},
		{
			key: "createdAt",
			label: "Created",
			sortable: true,
			render: (value: string) => `<span class="text-sm text-gray-600">${new Date(value).toLocaleString()}</span>`
		},
		{
			key: "completedAt",
			label: "Completed",
			sortable: true,
			render: (value: string | null) => value ? `<span class="text-sm text-gray-600">${new Date(value).toLocaleString()}</span>` : `<span class="text-sm text-gray-400">—</span>`
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			render: (_: any, row: any) => {
				if (row.status === "completed" && row.fileUrl) {
					return `
						<div class="flex items-center gap-1">
							<a href="${row.fileUrl}" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-primary-700 hover:bg-primary-50">Download</a>
							<button onclick="window.__deleteExport('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
						</div>
					`;
				}
				return `
					<div class="flex items-center gap-1">
						<button onclick="window.__deleteExport('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
					</div>
				`;
			}
		}
	];

	async function loadExports() {
		loading = true;
		const result = await api<DataExport[]>("/data-exports");
		if (result.ok) {
			exports = Array.isArray(result.data) ? result.data : [];
		}
		loading = false;
	}

	async function submitExport() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = "";
		const result = await api("/data-exports", {
			method: "POST",
			body: {
				name: form.name.trim(),
				type: form.type,
				format: form.format,
				includeMetadata: form.includeMetadata
			}
		});
		if (result.ok) {
			createModalOpen = false;
			resetForm();
			successMessage = "Export job queued successfully";
			loadExports();
		} else {
			errorMessage = result.error ?? "Failed to create export";
		}
		saving = false;
	}

	function openDeleteConfirm(id: string) {
		deletingId = id;
		deleteConfirmOpen = true;
	}

	async function confirmDelete() {
		if (!deletingId) return;
		saving = true;
		const result = await api(`/data-exports/${deletingId}`, { method: "DELETE" });
		if (result.ok) {
			successMessage = "Export deleted";
			loadExports();
		} else {
			errorMessage = result.error ?? "Failed to delete export";
		}
		deleteConfirmOpen = false;
		deletingId = null;
		saving = false;
	}

	function resetForm() {
		form = { name: "", type: "users", format: "csv", includeMetadata: true };
	}

	$effect(() => {
		loadExports();
	});

	$effect(() => {
		(window as any).__deleteExport = openDeleteConfirm;
		return () => {
			delete (window as any).__deleteExport;
		};
	});
</script>

<svelte:head>
	<title>Data Exports — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Data Exports"
	description="Export platform data in various formats"
	actions={[
		{ label: "New Export", icon: Plus, onclick: () => (createModalOpen = true) },
		{ label: "Refresh", icon: RefreshCw, variant: "ghost", onclick: loadExports }
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

	<!-- Summary Cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-4">
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">Total Exports</p>
						<p class="text-2xl font-bold text-gray-900">{exports.length}</p>
					</div>
					<div class="rounded-lg bg-primary-100 p-3">
						<Download class="h-6 w-6 text-primary-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">In Progress</p>
						<p class="text-2xl font-bold text-warning-600">
							{exports.filter((e) => e.status === "pending" || e.status === "processing").length}
						</p>
					</div>
					<div class="rounded-lg bg-warning-100 p-3">
						<Clock class="h-6 w-6 text-warning-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">Completed</p>
						<p class="text-2xl font-bold text-success-600">{exports.filter((e) => e.status === "completed").length}</p>
					</div>
					<div class="rounded-lg bg-success-100 p-3">
						<CheckCircle class="h-6 w-6 text-success-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-gray-500">Failed</p>
						<p class="text-2xl font-bold text-danger-600">{exports.filter((e) => e.status === "failed").length}</p>
					</div>
					<div class="rounded-lg bg-danger-100 p-3">
						<XCircle class="h-6 w-6 text-danger-600" />
					</div>
				</div>
			</CardContent>
		</Card>
	</div>

	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[
			{
				key: "type",
				label: "Export Type",
				options: exportTypes
			}
		]}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-gray-500">Loading...</p>
			{:else}
				<DataTable
					columns={exportColumns}
					data={filteredExports}
					pageSize={20}
					emptyMessage="No data exports found"
				/>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create Export Modal -->
<Modal bind:open={createModalOpen} title="New Data Export" description="Queue a new data export job">
	<div class="space-y-4">
		<div>
			<label for="export-name" class="mb-1 block text-sm font-medium text-gray-700">Export Name</label>
			<input
				id="export-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g., Users Q1 2024"
				bind:value={form.name}
			/>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="export-type" class="mb-1 block text-sm font-medium text-gray-700">Data Type</label>
				<Select bind:value={form.type} options={exportTypes} />
			</div>
			<div>
				<label for="export-format" class="mb-1 block text-sm font-medium text-gray-700">Format</label>
				<Select bind:value={form.format} options={formatOptions} />
			</div>
		</div>
		<div class="flex items-center gap-2">
			<input
				id="export-metadata"
				type="checkbox"
				bind:checked={form.includeMetadata}
				class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
			/>
			<label for="export-metadata" class="text-sm font-medium text-gray-700">Include metadata (timestamps, IDs)</label>
		</div>
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { createModalOpen = false; resetForm(); }}>Cancel</Button>
			<Button onclick={submitExport} disabled={saving || !form.name.trim()}>
				{saving ? "Queuing..." : "Start Export"}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={deleteConfirmOpen} title="Confirm Delete" description="This action cannot be undone">
	<div class="space-y-4">
		<p class="text-sm text-gray-600">
			Are you sure you want to delete this export? The downloaded file will no longer be available.
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (deleteConfirmOpen = false)}>Cancel</Button>
			<Button variant="destructive" onclick={confirmDelete} disabled={saving}>
				{saving ? "Deleting..." : "Delete Export"}
			</Button>
		</div>
	</div>
</Modal>
