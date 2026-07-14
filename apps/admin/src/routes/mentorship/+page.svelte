<script lang="ts">
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
	import { Users, Plus, Pencil, Trash2, UserCheck } from "lucide-svelte";
	import { api } from "$lib/api";

	type Program = {
		id: string;
		name: string;
		description?: string;
		is_active: boolean;
		mentor_count: number;
		mentee_count: number;
		match_count?: number;
		created_at: string;
	};

	let { data } = $props();

	let activeTab = $state("all");
	let programs = $state<Program[]>([]);
	let loading = $state(true);
	let filters = $state<Record<string, string>>({});

	// Create/Edit modal
	let showModal = $state(false);
	let editingProgram = $state<Program | null>(null);
	let saving = $state(false);
	let name = $state("");
	let description = $state("");
	let isActive = $state(true);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Program | null>(null);
	let deleting = $state(false);

	const tabs = [
		{ value: "all", label: "All Programs", icon: Users },
		{ value: "active", label: "Active", icon: UserCheck },
		{ value: "inactive", label: "Inactive", icon: Users }
	];

	const columns = [
		{
			key: "name",
			label: "Program",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-gray-900">${row.name}</p>
					${row.description ? `<p class="text-sm text-gray-500 line-clamp-1">${row.description}</p>` : ""}
				</div>
			`
		},
		{
			key: "mentor_count",
			label: "Mentors",
			sortable: true,
			render: (value: number) => `<span class="font-medium">${value ?? 0}</span>`
		},
		{
			key: "mentee_count",
			label: "Mentees",
			sortable: true,
			render: (value: number) => `<span class="font-medium">${value ?? 0}</span>`
		},
		{
			key: "match_count",
			label: "Matches",
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
			class: "w-24",
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__mentorshipEdit('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100">Edit</button>
					<button onclick="window.__mentorshipDelete('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
				</div>
			`
		}
	];

	let filteredPrograms = $derived.by(() => {
		let result = [...programs];
		if (activeTab === "active") result = result.filter((p) => p.is_active);
		if (activeTab === "inactive") result = result.filter((p) => !p.is_active);
		return result;
	});

	async function loadPrograms() {
		loading = true;
		try {
			const result = await api<Program[]>("/admin/mentorship/programs");
			if (result.ok) {
				programs = result.data ?? [];
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

	function openCreate() {
		editingProgram = null;
		name = "";
		description = "";
		isActive = true;
		showModal = true;
	}

	function openEdit(program: Program) {
		editingProgram = program;
		name = program.name;
		description = program.description || "";
		isActive = program.is_active;
		showModal = true;
	}

	async function saveProgram() {
		if (!name.trim()) return;
		saving = true;
		try {
			if (editingProgram) {
				const result = await api(`/admin/mentorship/programs/${editingProgram.id}`, {
					method: "PATCH",
					body: { name: name.trim(), description: description.trim() || undefined, is_active: isActive }
				});
				if (result.ok) {
					showModal = false;
					editingProgram = null;
					loadPrograms();
				}
			} else {
				const result = await api("/admin/mentorship/programs", {
					method: "POST",
					body: { name: name.trim(), description: description.trim() || undefined, is_active: isActive }
				});
				if (result.ok) {
					showModal = false;
					loadPrograms();
				}
			}
		} catch {
			// API not available
		} finally {
			saving = false;
		}
	}

	function confirmDelete(program: Program) {
		deleteTarget = program;
		showDeleteModal = true;
	}

	async function deleteProgram() {
		if (!deleteTarget) return;
		deleting = true;
		try {
			await api(`/admin/mentorship/programs/${deleteTarget.id}`, { method: "DELETE" });
			programs = programs.filter((p) => p.id !== deleteTarget!.id);
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
		(window as any).__mentorshipEdit = (id: string) => {
			const program = programs.find((p) => p.id === id);
			if (program) openEdit(program);
		};
		(window as any).__mentorshipDelete = (id: string) => {
			const program = programs.find((p) => p.id === id);
			if (program) confirmDelete(program);
		};
	});

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadPrograms();
		}
	});
</script>

<svelte:head>
	<title>Mentorship — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Mentorship"
	description="Manage mentorship programs and matching"
	actions={[
		{ label: "New Program", icon: Plus, onclick: openCreate }
	]}
/>

<div class="space-y-6">
	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[]}
		searchPlaceholder="Search programs..."
		onsearch={handleFilter}
		onclear={() => { filters = {}; }}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-gray-500">Loading programs...</p>
			{:else}
				<DataTable
					columns={columns}
					data={filteredPrograms}
					pageSize={15}
					emptyMessage="No mentorship programs found"
				/>
			{/if}
		</CardContent>
	</Card>

	<!-- Summary Stats -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-4">
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-gray-500">Total Programs</p>
				<p class="text-2xl font-bold text-gray-900">{programs.length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-gray-500">Active Programs</p>
				<p class="text-2xl font-bold text-gray-900">{programs.filter((p) => p.is_active).length}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-gray-500">Total Mentors</p>
				<p class="text-2xl font-bold text-gray-900">{programs.reduce((sum, p) => sum + (p.mentor_count ?? 0), 0)}</p>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-4">
				<p class="text-sm text-gray-500">Total Mentees</p>
				<p class="text-2xl font-bold text-gray-900">{programs.reduce((sum, p) => sum + (p.mentee_count ?? 0), 0)}</p>
			</CardContent>
		</Card>
	</div>
</div>

<!-- Create/Edit Modal -->
<Modal
	bind:open={showModal}
	title={editingProgram ? "Edit Program" : "New Program"}
	description={editingProgram ? "Update the mentorship program details" : "Create a new mentorship program"}
	size="lg"
>
	<div class="space-y-4">
		<div>
			<label for="mp-name" class="mb-1 block text-sm font-medium text-gray-900">Program Name</label>
			<input
				id="mp-name"
				type="text"
				class="h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="e.g. Career Growth Mentorship"
				bind:value={name}
			/>
		</div>
		<div>
			<label for="mp-desc" class="mb-1 block text-sm font-medium text-gray-900">Description</label>
			<textarea
				id="mp-desc"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				rows="3"
				placeholder="What is this program about?"
				bind:value={description}
			></textarea>
		</div>
		<div class="flex items-center gap-3">
			<button
				type="button"
				role="switch"
				aria-checked={isActive}
				aria-label="Active status"
				onclick={() => (isActive = !isActive)}
				class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 {isActive ? "bg-primary-600" : "bg-gray-200"}"
			>
				<span class="inline-block h-4 w-4 transform rounded-full bg-white shadow-sm transition-transform {isActive ? "translate-x-6" : "translate-x-1"}"></span>
			</button>
			<span class="text-sm text-gray-700">{isActive ? "Active" : "Inactive"}</span>
		</div>
	</div>

	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showModal = false)}>Cancel</Button>
		<Button onclick={saveProgram} disabled={saving || !name.trim()}>
			{#if saving}
				Saving...
			{:else}
				{editingProgram ? "Save Changes" : "Create Program"}
			{/if}
		</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Delete Program" description="This action cannot be undone." size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-danger-50 p-4">
			<p class="text-sm text-danger-700">
				Are you sure you want to delete <strong>{deleteTarget.name}</strong>?
				{#if deleteTarget.mentor_count > 0 || deleteTarget.mentee_count > 0}
					This program has {deleteTarget.mentor_count} mentors and {deleteTarget.mentee_count} mentees.
				{/if}
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>Cancel</Button>
		<Button variant="destructive" onclick={deleteProgram} disabled={deleting}>
			{#if deleting}
				Deleting...
			{:else}
				Delete Program
			{/if}
		</Button>
	</div>
</Modal>
