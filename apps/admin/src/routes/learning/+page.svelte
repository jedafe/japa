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
	import { BookOpen, Plus, GraduationCap, Layers, FileText, Trash2, Eye, Send } from "lucide-svelte";
	import { api } from "$lib/api";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	type Track = {
		id: string;
		title: string;
		slug: string;
		description: string;
		status: string;
		module_count: number;
		enrollment_count: number;
		created_at: string;
	};

	type Module = {
		id: string;
		track_id: string;
		track_title?: string;
		title: string;
		slug: string;
		lesson_count: number;
		sort_order: number;
	};

	type Lesson = {
		id: string;
		module_id: string;
		module_title?: string;
		title: string;
		slug: string;
		duration_minutes: number;
		sort_order: number;
	};

	let activeTab = $state("tracks");
	let tracks = $state<Track[]>([]);
	let modules = $state<Module[]>([]);
	let lessons = $state<Lesson[]>([]);
	let loading = $state(true);
	let error = $state("");
	let filters = $state<Record<string, string>>({});

	// Create modal
	let showCreateModal = $state(false);
	let creating = $state(false);
	let newTrack = $state({ title: "", description: "" });
	let newModule = $state({ track_id: "", title: "" });
	let newLesson = $state({ module_id: "", title: "", body: "" });

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleteTarget = $state<{ type: string; id: string; title: string } | null>(null);
	let deleting = $state(false);

	// Publish confirmation
	let showPublishModal = $state(false);
	let publishTarget = $state<{ id: string; title: string } | null>(null);
	let publishing = $state(false);

	const canManageLearning = $derived(hasPermission("learning.admin.manage"));

	const tabs = $derived([
		{ value: "tracks", label: "Tracks", icon: GraduationCap, count: tracks.length },
		{ value: "modules", label: "Modules", icon: Layers, count: modules.length },
		{ value: "lessons", label: "Lessons", icon: FileText, count: lessons.length }
	]);

	const trackColumns = [
		{
			key: "title",
			label: "Track",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					<p class="text-sm text-muted line-clamp-1">${row.description || "No description"}</p>
				</div>
			`
		},
		{
			key: "module_count",
			label: "Modules",
			sortable: true,
			render: (value: number) => `<span class="font-medium">${value ?? 0}</span>`
		},
		{
			key: "enrollment_count",
			label: "Enrollments",
			sortable: true,
			render: (value: number) => `<span class="font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: "status",
			label: "Status",
			sortable: true,
			render: (value: string) => {
				const variant = value === "published" ? "success" : value === "draft" ? "warning" : "secondary";
				const classes: Record<string, string> = {
					success: "bg-success/15 text-success border-success",
					warning: "bg-warning/15 text-warning border-warning",
					secondary: "bg-surface text-foreground border"
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[variant]}">${value}</span>`;
			}
		},
		{
			key: "actions",
			label: "",
			class: "w-32",
			component: TrackActionsCell
		}
	];

	const moduleColumns = [
		{
			key: "title",
			label: "Module",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					${row.track_title ? `<p class="text-sm text-muted">${row.track_title}</p>` : ""}
				</div>
			`
		},
		{
			key: "lesson_count",
			label: "Lessons",
			sortable: true,
			render: (value: number) => `<span class="font-medium">${value ?? 0}</span>`
		},
		{
			key: "sort_order",
			label: "Order",
			sortable: true,
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value}</span>`
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			component: ModuleActionsCell
		}
	];

	const lessonColumns = [
		{
			key: "title",
			label: "Lesson",
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					${row.module_title ? `<p class="text-sm text-muted">${row.module_title}</p>` : ""}
				</div>
			`
		},
		{
			key: "duration_minutes",
			label: "Duration",
			sortable: true,
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value ? `${value} min` : "—"}</span>`
		},
		{
			key: "sort_order",
			label: "Order",
			sortable: true,
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value}</span>`
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			component: LessonActionsCell
		}
	];

	let currentColumns = $derived(
		activeTab === "tracks" ? trackColumns : activeTab === "modules" ? moduleColumns : lessonColumns
	);

	let currentData = $derived(
		activeTab === "tracks" ? tracks : activeTab === "modules" ? modules : lessons
	);

	async function refresh() {
		loading = true;
		error = "";
		try {
			if (activeTab === "tracks") {
				const result = await api<Track[]>("/admin/learn/tracks");
				tracks = result.ok ? (result.data ?? []) : [];
			} else if (activeTab === "modules") {
				const result = await api<Module[]>("/admin/learn/modules");
				modules = result.ok ? (result.data ?? []) : [];
			} else {
				const result = await api<Lesson[]>("/admin/learn/lessons");
				lessons = result.ok ? (result.data ?? []) : [];
			}
		} catch {
			error = "Failed to load data";
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filters = values;
	}

	function openCreate() {
		showCreateModal = true;
	}

	async function createTrack() {
		if (!newTrack.title.trim() || creating) return;
		creating = true;
		try {
			const result = await api("/admin/learn/tracks", {
				method: "POST",
				body: newTrack
			});
			if (result.ok) {
				showCreateModal = false;
				newTrack = { title: "", description: "" };
				await refresh();
			}
		} catch {
			error = "Failed to create track";
		} finally {
			creating = false;
		}
	}

	async function createModule() {
		if (!newModule.title.trim() || !newModule.track_id || creating) return;
		creating = true;
		try {
			await api(`/admin/learn/tracks/${newModule.track_id}/modules`, {
				method: "POST",
				body: { title: newModule.title }
			});
			showCreateModal = false;
			newModule = { track_id: "", title: "" };
			await refresh();
		} catch {
			error = "Failed to create module";
		} finally {
			creating = false;
		}
	}

	async function createLesson() {
		if (!newLesson.title.trim() || !newLesson.module_id || creating) return;
		creating = true;
		try {
			await api(`/admin/learn/modules/${newLesson.module_id}/lessons`, {
				method: "POST",
				body: { title: newLesson.title, body: newLesson.body }
			});
			showCreateModal = false;
			newLesson = { module_id: "", title: "", body: "" };
			await refresh();
		} catch {
			error = "Failed to create lesson";
		} finally {
			creating = false;
		}
	}

	async function publishTrack(id: string) {
		publishing = true;
		try {
			await api(`/admin/learn/tracks/${id}/publish`, { method: "POST" });
			await refresh();
		} catch {
			error = "Failed to publish track";
		} finally {
			publishing = false;
		}
	}

	function openPublish(id: string, title: string) {
		publishTarget = { id, title };
		showPublishModal = true;
	}

	function confirmDelete(type: string, id: string, title: string) {
		deleteTarget = { type, id, title };
		showDeleteModal = true;
	}

	async function deleteItem() {
		if (!deleteTarget) return;
		deleting = true;
		try {
			const { type, id } = deleteTarget;
			if (type === "track") await api(`/admin/learn/tracks/${id}`, { method: "DELETE" });
			else if (type === "module") await api(`/admin/learn/modules/${id}`, { method: "DELETE" });
			else await api(`/admin/learn/lessons/${id}`, { method: "DELETE" });
			showDeleteModal = false;
			deleteTarget = null;
			await refresh();
		} catch {
			error = "Failed to delete item";
		} finally {
			deleting = false;
		}
	}

	$effect(() => {
		refresh();
	});
</script>

<svelte:head>
	<title>Learning — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Learning Management"
	description="Create and manage courses, modules, and lessons"
	actions={[
		...(canManageLearning ? [{ label: `New ${activeTab === "tracks" ? "Track" : activeTab === "modules" ? "Module" : "Lesson"}`, icon: Plus, onclick: openCreate }] : [])
	]}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg border border-danger bg-danger-50 p-4 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar
		bind:value={filters}
		filters={[]}
		searchPlaceholder="Search learning content..."
		onsearch={handleFilter}
		onclear={() => { filters = {}; }}
	/>

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<div class="space-y-3 p-4">
					{#each Array(5) as _}
						<div class="h-14 animate-pulse rounded-lg bg-surface"></div>
					{/each}
				</div>
			{:else}
				<DataTable
					columns={currentColumns}
					data={currentData}
					pageSize={15}
					emptyMessage={`No ${activeTab} found`}
				/>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- Create Modal -->
<Modal
	bind:open={showCreateModal}
	title={`Create ${activeTab === "tracks" ? "Track" : activeTab === "modules" ? "Module" : "Lesson"}`}
	description={`Add a new ${activeTab.slice(0, -1)} to the learning platform`}
	size="lg"
>
	{#if activeTab === "tracks"}
		<div class="space-y-4">
			<div>
				<label for="track-title" class="mb-1 block text-sm font-medium text-foreground">{t('learning.title')}</label>
				<input
					id="track-title"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder="e.g. JavaScript Fundamentals"
					bind:value={newTrack.title}
				/>
			</div>
			<div>
				<label for="track-description" class="mb-1 block text-sm font-medium text-foreground">{t('learning.description')}</label>
				<textarea
					id="track-description"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					rows="3"
					placeholder="What will students learn?"
					bind:value={newTrack.description}
				></textarea>
			</div>
		</div>
	{:else if activeTab === "modules"}
		<div class="space-y-4">
			<div>
				<label for="module-track-id" class="mb-1 block text-sm font-medium text-foreground">{t('learning.track_id')}</label>
				<input
					id="module-track-id"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder="Parent track ID"
					bind:value={newModule.track_id}
				/>
			</div>
			<div>
				<label for="module-title" class="mb-1 block text-sm font-medium text-foreground">{t('learning.title')}</label>
				<input
					id="module-title"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder="e.g. Variables and Types"
					bind:value={newModule.title}
				/>
			</div>
		</div>
	{:else}
		<div class="space-y-4">
			<div>
				<label for="lesson-module-id" class="mb-1 block text-sm font-medium text-foreground">{t('learning.module_id')}</label>
				<input
					id="lesson-module-id"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder="Parent module ID"
					bind:value={newLesson.module_id}
				/>
			</div>
			<div>
				<label for="lesson-title" class="mb-1 block text-sm font-medium text-foreground">{t('learning.title')}</label>
				<input
					id="lesson-title"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder="e.g. Introduction to Closures"
					bind:value={newLesson.title}
				/>
			</div>
			<div>
				<label for="lesson-body" class="mb-1 block text-sm font-medium text-foreground">{t('learning.content_markdown')}</label>
				<textarea
					id="lesson-body"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					rows="8"
					placeholder="Lesson content in Markdown..."
					bind:value={newLesson.body}
				></textarea>
			</div>
		</div>
	{/if}

	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showCreateModal = false)}>{t('learning.cancel')}</Button>
		<Button
			onclick={activeTab === "tracks" ? createTrack : activeTab === "modules" ? createModule : createLesson}
			disabled={creating}
		>
			{#if creating}
				Creating...
			{:else}
				Create
			{/if}
		</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Delete Item" description="This action cannot be undone." size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-danger-50 p-4">
			<p class="text-sm text-danger-700">
				Are you sure you want to delete <strong>{deleteTarget.title}</strong>?
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('learning.cancel')}</Button>
		<Button variant="destructive" onclick={deleteItem} disabled={deleting}>
			{#if deleting}
				Deleting...
			{:else}
				Delete
			{/if}
		</Button>
	</div>
</Modal>

<!-- Publish Confirm Modal -->
<Modal bind:open={showPublishModal} title="Publish Track" description="This will make the track visible to learners." size="sm">
	{#if publishTarget}
		<div class="rounded-lg bg-success-50 p-4">
			<p class="text-sm text-success-700">
				Publish <strong>{publishTarget.title}</strong>?
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showPublishModal = false)}>{t('learning.cancel')}</Button>
		<Button onclick={async () => { if (publishTarget) { await publishTrack(publishTarget.id); showPublishModal = false; publishTarget = null; }}} disabled={publishing}>
			{#if publishing}
				Publishing...
			{:else}
				Publish
			{/if}
		</Button>
	</div>
</Modal>

<!-- Action Cells -->
{#snippet TrackActionsCell(row: any)}
	<div class="flex items-center gap-1">
		{#if canManageLearning && row.status === "draft"}
			<Button variant="ghost" size="sm" onclick={() => openPublish(row.id, row.title)} style="color: var(--color-success)">
				<Send class="h-3.5 w-3.5" /> Publish
			</Button>
		{/if}
		{#if canManageLearning}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("track", row.id, row.title)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

{#snippet ModuleActionsCell(row: any)}
	<div class="flex items-center gap-1">
		{#if canManageLearning}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("module", row.id, row.title)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

{#snippet LessonActionsCell(row: any)}
	<div class="flex items-center gap-1">
		{#if canManageLearning}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("lesson", row.id, row.title)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
