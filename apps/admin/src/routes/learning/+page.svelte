<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { Plus, GraduationCap, Layers, FileText, Trash2, Send, Pencil, Users } from "lucide-svelte";
	import { api } from "$lib/api";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	// ── Types ──────────────────────────────────────────────────────────────

	type Track = {
		id: string;
		title: string;
		slug: string;
		description: string;
		difficulty: string;
		is_published: boolean;
		sort_order: number;
		created_at: string;
		updated_at: string;
	};

	type Module = {
		id: string;
		track_id: string;
		track_title?: string;
		title: string;
		slug: string;
		description?: string;
		lesson_count: number;
		sort_order: number;
		created_at: string;
	};

	type Lesson = {
		id: string;
		module_id: string;
		module_title?: string;
		title: string;
		slug: string;
		lesson_type: string;
		content?: string;
		duration_minutes?: number;
		sort_order: number;
		is_published: boolean;
		created_at: string;
	};

	type Enrollment = {
		id: string;
		user_id: string;
		access_type: string;
		progress_pct: number;
		enrolled_at: string;
		completed_at?: string;
	};

	// ── State ──────────────────────────────────────────────────────────────

	let activeTab = $state("tracks");
	let tracks = $state<Track[]>([]);
	let modules = $state<Module[]>([]);
	let lessons = $state<Lesson[]>([]);
	let loading = $state(true);
	let error = $state("");
	let filters = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Create modal
	let showCreateModal = $state(false);
	let creating = $state(false);
	let newTrack = $state({ title: "", slug: "", description: "", difficulty: "" });
	let newModule = $state({ track_id: "", title: "", slug: "", description: "" });
	let newLesson = $state({
		selectedTrackId: "",
		module_id: "",
		title: "",
		slug: "",
		lesson_type: "article",
		content: "",
		duration_minutes: 0,
		sort_order: 0
	});

	// Edit modal
	let showEditModal = $state(false);
	let editingItem = $state<any>(null);
	let editForm = $state<Record<string, any>>({});
	let savingEdit = $state(false);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleteTarget = $state<{ type: string; id: string; title: string } | null>(null);
	let deleting = $state(false);

	// Publish confirmation
	let showPublishModal = $state(false);
	let publishTarget = $state<{ id: string; title: string } | null>(null);
	let publishing = $state(false);

	// Enrollments modal
	let showEnrollmentsModal = $state(false);
	let enrollmentsTrack = $state<{ id: string; title: string } | null>(null);
	let enrollments = $state<Enrollment[]>([]);
	let loadingEnrollments = $state(false);

	// Modules for lesson create dropdown
	let modulesForLesson = $state<Module[]>([]);

	const canManageLearning = $derived(hasPermission("learning.admin.manage"));

	const tabs = $derived([
		{ value: "tracks", label: t('app.learning.tab_tracks', 'Tracks'), icon: GraduationCap, count: tracks.length },
		{ value: "modules", label: t('app.learning.tab_modules', 'Modules'), icon: Layers, count: modules.length },
		{ value: "lessons", label: t('app.learning.tab_lessons', 'Lessons'), icon: FileText, count: lessons.length }
	]);

	// ── Helpers ────────────────────────────────────────────────────────────

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	const difficultyOptions = [
		{ value: "", label: "—" },
		{ value: "beginner", label: "Beginner" },
		{ value: "intermediate", label: "Intermediate" },
		{ value: "advanced", label: "Advanced" }
	];

	const lessonTypeOptions = [
		{ value: "article", label: "Article" },
		{ value: "video", label: "Video" },
		{ value: "quiz", label: "Quiz" }
	];

	// ── Columns ────────────────────────────────────────────────────────────

	const trackColumns = [
		{
			key: "title",
			label: t('app.learning.col_track', 'Track'),
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					<p class="text-sm text-muted line-clamp-1">${row.description || t('app.learning.no_description', 'No description')}</p>
				</div>
			`
		},
		{
			key: "difficulty",
			label: t('app.learning.col_difficulty', 'Difficulty'),
			sortable: true,
			render: (value: string) => `<span class="text-sm text-foreground-alt">${value || "—"}</span>`
		},
		{
			key: "is_published",
			label: t('app.learning.col_status', 'Status'),
			sortable: true,
			render: (value: boolean) => {
				const label = value ? "published" : "draft";
				const variant = value ? "success" : "warning";
				const classes: Record<string, string> = {
					success: "bg-success/15 text-success border-success",
					warning: "bg-warning/15 text-warning border-warning",
					secondary: "bg-surface text-foreground border"
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[variant]}">${label}</span>`;
			}
		},
		{
			key: "actions",
			label: "",
			class: "w-48",
			componentSnippet: TrackActionsCell
		}
	];

	const moduleColumns = [
		{
			key: "title",
			label: t('app.learning.col_module', 'Module'),
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
			label: t('app.learning.col_lessons', 'Lessons'),
			sortable: true,
			render: (value: number) => `<span class="font-medium">${value ?? 0}</span>`
		},
		{
			key: "sort_order",
			label: t('app.learning.col_order', 'Order'),
			sortable: true,
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value}</span>`
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			componentSnippet: ModuleActionsCell
		}
	];

	const lessonColumns = [
		{
			key: "title",
			label: t('app.learning.col_lesson', 'Lesson'),
			sortable: true,
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					${row.module_title ? `<p class="text-sm text-muted">${row.module_title}</p>` : ""}
				</div>
			`
		},
		{
			key: "lesson_type",
			label: t('app.learning.col_type', 'Type'),
			sortable: true,
			render: (value: string) => `<span class="text-sm text-foreground-alt">${value || "—"}</span>`
		},
		{
			key: "duration_minutes",
			label: t('app.learning.col_duration', 'Duration'),
			sortable: true,
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value ? `${value} ${t('app.learning.min', 'min')}` : "—"}</span>`
		},
		{
			key: "sort_order",
			label: t('app.learning.col_order', 'Order'),
			sortable: true,
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value}</span>`
		},
		{
			key: "actions",
			label: "",
			class: "w-24",
			componentSnippet: LessonActionsCell
		}
	];

	let currentColumns = $derived(
		activeTab === "tracks" ? trackColumns : activeTab === "modules" ? moduleColumns : lessonColumns
	);

	let currentData = $derived.by(() => {
		const source = activeTab === "tracks" ? tracks : activeTab === "modules" ? modules : lessons;
		const q = (filters.search || "").toLowerCase().trim();
		if (!q) return source;
		return source.filter((item: any) => {
			const searchable = [item.title, item.description, item.slug, item.track_title, item.module_title]
				.filter(Boolean)
				.join(" ")
				.toLowerCase();
			return searchable.includes(q);
		});
	});

	// ── Data Loading ───────────────────────────────────────────────────────

	async function refresh() {
		loading = true;
		error = "";
		try {
			if (activeTab === "tracks") {
				const result = await api<Track[]>("/admin/learn/tracks");
				if (result.ok) {
					tracks = result.data ?? [];
				} else {
					error = result.error || t('app.learning.error_load', 'Failed to load data');
				}
			} else if (activeTab === "modules") {
				const result = await api<Module[]>("/admin/learn/modules");
				if (result.ok) {
					modules = result.data ?? [];
				} else {
					error = result.error || t('app.learning.error_load', 'Failed to load data');
				}
			} else {
				const result = await api<Lesson[]>("/admin/learn/lessons");
				if (result.ok) {
					lessons = result.data ?? [];
				} else {
					error = result.error || t('app.learning.error_load', 'Failed to load data');
				}
			}
		} catch {
			error = t('app.learning.error_load', 'Failed to load data');
		} finally {
			loading = false;
		}
	}

	async function fetchTracks() {
		const result = await api<Track[]>("/admin/learn/tracks");
		if (result.ok) {
			tracks = result.data ?? [];
		}
	}

	async function fetchModulesForTrack(trackId: string) {
		const result = await api<Module[]>(`/admin/learn/tracks/${trackId}/modules`);
		if (result.ok) {
			modulesForLesson = result.data ?? [];
		} else {
			modulesForLesson = [];
		}
	}

	function handleFilter(values: Record<string, string>) {
		filters = values;
	}

	// ── Create ─────────────────────────────────────────────────────────────

	function openCreate() {
		showCreateModal = true;
	}

	async function createTrack() {
		if (!newTrack.title.trim() || creating) return;
		creating = true;
		error = "";
		try {
			const slug = newTrack.slug.trim() || slugify(newTrack.title);
			const result = await api("/admin/learn/tracks", {
				method: "POST",
				body: {
					title: newTrack.title.trim(),
					slug,
					description: newTrack.description.trim(),
					difficulty: newTrack.difficulty
				}
			});
			if (result.ok) {
				showCreateModal = false;
				newTrack = { title: "", slug: "", description: "", difficulty: "" };
				await refresh();
			} else {
				error = result.error || t('app.learning.error_create_track', 'Failed to create track');
			}
		} catch {
			error = t('app.learning.error_create_track', 'Failed to create track');
		} finally {
			creating = false;
		}
	}

	async function createModule() {
		if (!newModule.title.trim() || !newModule.track_id || creating) return;
		creating = true;
		error = "";
		try {
			const slug = newModule.slug.trim() || slugify(newModule.title);
			const result = await api(`/admin/learn/tracks/${newModule.track_id}/modules`, {
				method: "POST",
				body: {
					title: newModule.title.trim(),
					slug,
					description: newModule.description.trim()
				}
			});
			if (result.ok) {
				showCreateModal = false;
				newModule = { track_id: "", title: "", slug: "", description: "" };
				await refresh();
			} else {
				error = result.error || t('app.learning.error_create_module', 'Failed to create module');
			}
		} catch {
			error = t('app.learning.error_create_module', 'Failed to create module');
		} finally {
			creating = false;
		}
	}

	async function createLesson() {
		if (!newLesson.title.trim() || !newLesson.module_id || creating) return;
		creating = true;
		error = "";
		try {
			const slug = newLesson.slug.trim() || slugify(newLesson.title);
			const result = await api(`/admin/learn/modules/${newLesson.module_id}/lessons`, {
				method: "POST",
				body: {
					title: newLesson.title.trim(),
					slug,
					lesson_type: newLesson.lesson_type,
					content: newLesson.content.trim(),
					duration_minutes: newLesson.duration_minutes || undefined,
					sort_order: newLesson.sort_order
				}
			});
			if (result.ok) {
				showCreateModal = false;
				newLesson = {
					selectedTrackId: "",
					module_id: "",
					title: "",
					slug: "",
					lesson_type: "article",
					content: "",
					duration_minutes: 0,
					sort_order: 0
				};
				modulesForLesson = [];
				await refresh();
			} else {
				error = result.error || t('app.learning.error_create_lesson', 'Failed to create lesson');
			}
		} catch {
			error = t('app.learning.error_create_lesson', 'Failed to create lesson');
		} finally {
			creating = false;
		}
	}

	// ── Publish ────────────────────────────────────────────────────────────

	function openPublish(id: string, title: string) {
		publishTarget = { id, title };
		showPublishModal = true;
	}

	async function publishTrack(id: string) {
		publishing = true;
		error = "";
		try {
			const result = await api(`/admin/learn/tracks/${id}/publish`, {
				method: "POST",
				body: { is_published: true }
			});
			if (result.ok) {
				showPublishModal = false;
				publishTarget = null;
				await refresh();
			} else {
				error = result.error || t('app.learning.error_publish_track', 'Failed to publish track');
			}
		} catch {
			error = t('app.learning.error_publish_track', 'Failed to publish track');
		} finally {
			publishing = false;
		}
	}

	// ── Edit ───────────────────────────────────────────────────────────────

	function openEdit(type: string, item: any) {
		editingItem = { ...item, _type: type };
		if (type === "track") {
			editForm = {
				title: item.title,
				description: item.description || "",
				difficulty: item.difficulty || "",
				is_published: item.is_published,
				sort_order: item.sort_order ?? 0
			};
		} else if (type === "module") {
			editForm = {
				title: item.title,
				description: item.description || "",
				sort_order: item.sort_order ?? 0
			};
		} else {
			editForm = {
				title: item.title,
				lesson_type: item.lesson_type || "article",
				content: item.content || "",
				duration_minutes: item.duration_minutes ?? 0,
				is_published: item.is_published,
				sort_order: item.sort_order ?? 0
			};
		}
		showEditModal = true;
	}

	async function saveEdit() {
		if (!editingItem || savingEdit) return;
		savingEdit = true;
		error = "";
		try {
			const { _type, id } = editingItem;
			let result;
			if (_type === "track") {
				result = await api(`/admin/learn/tracks/${id}`, {
					method: "PATCH",
					body: editForm
				});
			} else if (_type === "module") {
				result = await api(`/admin/learn/modules/${id}`, {
					method: "PATCH",
					body: editForm
				});
			} else {
				result = await api(`/admin/learn/lessons/${id}`, {
					method: "PATCH",
					body: editForm
				});
			}
			if (result.ok) {
				showEditModal = false;
				editingItem = null;
				await refresh();
			} else {
				error = result.error || t('app.learning.error_save', 'Failed to save changes');
			}
		} catch {
			error = t('app.learning.error_save', 'Failed to save changes');
		} finally {
			savingEdit = false;
		}
	}

	// ── Delete ─────────────────────────────────────────────────────────────

	function confirmDelete(type: string, id: string, title: string) {
		deleteTarget = { type, id, title };
		showDeleteModal = true;
	}

	async function deleteItem() {
		if (!deleteTarget) return;
		deleting = true;
		error = "";
		try {
			const { type, id } = deleteTarget;
			let result;
			if (type === "track") {
				result = await api(`/admin/learn/tracks/${id}`, { method: "DELETE" });
			} else if (type === "module") {
				result = await api(`/admin/learn/modules/${id}`, { method: "DELETE" });
			} else {
				result = await api(`/admin/learn/lessons/${id}`, { method: "DELETE" });
			}
			if (result.ok) {
				showDeleteModal = false;
				deleteTarget = null;
				await refresh();
			} else {
				error = result.error || t('app.learning.error_delete_item', 'Failed to delete item');
			}
		} catch {
			error = t('app.learning.error_delete_item', 'Failed to delete item');
		} finally {
			deleting = false;
		}
	}

	// ── Enrollments ────────────────────────────────────────────────────────

	async function openEnrollments(id: string, title: string) {
		enrollmentsTrack = { id, title };
		showEnrollmentsModal = true;
		loadingEnrollments = true;
		enrollments = [];
		try {
			const result = await api<Enrollment[]>(`/admin/learn/tracks/${id}/enrollments`);
			if (result.ok) {
				enrollments = result.data ?? [];
			} else {
				error = result.error || t('app.learning.error_enrollments', 'Failed to load enrollments');
			}
		} catch {
			error = t('app.learning.error_enrollments', 'Failed to load enrollments');
		} finally {
			loadingEnrollments = false;
		}
	}

	// ── Bulk Delete ────────────────────────────────────────────────────────

	async function bulkDelete() {
		if (selectedIds.size === 0 || bulkDeleting) return;
		bulkDeleting = true;
		error = "";
		try {
			const ids = Array.from(selectedIds);
			const endpoint = activeTab === "tracks"
				? (id: string) => `/admin/learn/tracks/${id}`
				: activeTab === "modules"
					? (id: string) => `/admin/learn/modules/${id}`
					: (id: string) => `/admin/learn/lessons/${id}`;
			await Promise.all(ids.map((id) => api(endpoint(id), { method: "DELETE" })));
			selectedIds = new Set();
			await refresh();
		} catch {
			error = t('app.learning.error_delete_bulk', 'Failed to delete selected items');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Effects ────────────────────────────────────────────────────────────

	$effect(() => {
		refresh();
	});

	$effect(() => {
		void activeTab;
		selectedIds = new Set();
	});

	// When lesson create modal opens and we're on the lessons tab, pre-fetch tracks
	$effect(() => {
		if (showCreateModal && activeTab === "lessons") {
			fetchTracks();
		}
	});

	// When module create modal opens, pre-fetch tracks
	$effect(() => {
		if (showCreateModal && activeTab === "modules") {
			fetchTracks();
		}
	});

	// When the selected track changes in the lesson create form, fetch its modules
	$effect(() => {
		if (newLesson.selectedTrackId) {
			fetchModulesForTrack(newLesson.selectedTrackId);
		} else {
			modulesForLesson = [];
		}
	});
</script>

<svelte:head>
	<title>{t('app.learning.page_title', 'Learning — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.learning.title', 'Learning Management')}
	description={t('app.learning.description', 'Create and manage courses, modules, and lessons')}
	actions={[
		...(canManageLearning ? [{ label: `${t('app.learning.new_prefix', 'New')} ${activeTab === "tracks" ? t('app.learning.track', 'Track') : activeTab === "modules" ? t('app.learning.module', 'Module') : t('app.learning.lesson', 'Lesson')}`, icon: Plus, onclick: openCreate }] : [])
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
		searchPlaceholder={t('app.learning.search_placeholder', 'Search learning content...')}
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
					emptyMessage={t(`app.learning.empty_${activeTab}`, `No ${activeTab} found`)}
					selectable={canManageLearning}
					bind:selectedIds
				>
					{#snippet bulkActions()}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
							<Trash2 class="h-3.5 w-3.5" />
							{bulkDeleting ? t('app.learning.deleting', 'Deleting...') : t('app.learning.delete_selected', 'Delete Selected')}
						</Button>
					{/snippet}
				</DataTable>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- ══════════════════════════════════════════════════════════════════════════
     CREATE MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<Modal
	bind:open={showCreateModal}
	title={`${t('app.learning.create_prefix', 'Create')} ${activeTab === "tracks" ? t('app.learning.track', 'Track') : activeTab === "modules" ? t('app.learning.module', 'Module') : t('app.learning.lesson', 'Lesson')}`}
	description={`${t('app.learning.add_new_prefix', 'Add a new')} ${activeTab.slice(0, -1)} ${t('app.learning.add_new_suffix', 'to the learning platform')}`}
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
					placeholder={t('app.learning.placeholder_track_title', 'e.g. JavaScript Fundamentals')}
					bind:value={newTrack.title}
					onblur={() => { if (!newTrack.slug) newTrack.slug = slugify(newTrack.title); }}
				/>
			</div>
			<div>
				<label for="track-slug" class="mb-1 block text-sm font-medium text-foreground">{t('learning.slug')}</label>
				<input
					id="track-slug"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.learning.placeholder_track_slug', 'e.g. javascript-fundamentals')}
					bind:value={newTrack.slug}
				/>
			</div>
			<div>
				<label for="track-description" class="mb-1 block text-sm font-medium text-foreground">{t('learning.description')}</label>
				<textarea
					id="track-description"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					rows="3"
					placeholder={t('app.learning.placeholder_track_description', 'What will students learn?')}
					bind:value={newTrack.description}
				></textarea>
			</div>
			<div>
				<label for="track-difficulty" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.difficulty', 'Difficulty')}</label>
				<select
					id="track-difficulty"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={newTrack.difficulty}
				>
					{#each difficultyOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
		</div>
	{:else if activeTab === "modules"}
		<div class="space-y-4">
			<div>
				<label for="module-track-id" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.track', 'Track')}</label>
				<select
					id="module-track-id"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={newModule.track_id}
				>
					<option value="">{t('app.learning.select_track', 'Select a track...')}</option>
					{#each tracks as track}
						<option value={track.id}>{track.title}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="module-title" class="mb-1 block text-sm font-medium text-foreground">{t('learning.title')}</label>
				<input
					id="module-title"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.learning.placeholder_module_title', 'e.g. Variables and Types')}
					bind:value={newModule.title}
					onblur={() => { if (!newModule.slug) newModule.slug = slugify(newModule.title); }}
				/>
			</div>
			<div>
				<label for="module-slug" class="mb-1 block text-sm font-medium text-foreground">{t('learning.slug')}</label>
				<input
					id="module-slug"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.learning.placeholder_module_slug', 'e.g. variables-and-types')}
					bind:value={newModule.slug}
				/>
			</div>
			<div>
				<label for="module-description" class="mb-1 block text-sm font-medium text-foreground">{t('learning.description')}</label>
				<textarea
					id="module-description"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					rows="3"
					placeholder={t('app.learning.placeholder_module_description', 'What will students learn in this module?')}
					bind:value={newModule.description}
				></textarea>
			</div>
		</div>
	{:else}
		<div class="space-y-4">
			<div>
				<label for="lesson-track-select" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.track', 'Track')}</label>
				<select
					id="lesson-track-select"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={newLesson.selectedTrackId}
				>
					<option value="">{t('app.learning.select_track', 'Select a track...')}</option>
					{#each tracks as track}
						<option value={track.id}>{track.title}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="lesson-module-select" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.module', 'Module')}</label>
				<select
					id="lesson-module-select"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={newLesson.module_id}
					disabled={!newLesson.selectedTrackId}
				>
					<option value="">{t('app.learning.select_module', 'Select a module...')}</option>
					{#each modulesForLesson as mod}
						<option value={mod.id}>{mod.title}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="lesson-title" class="mb-1 block text-sm font-medium text-foreground">{t('learning.title')}</label>
				<input
					id="lesson-title"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.learning.placeholder_lesson_title', 'e.g. Introduction to Closures')}
					bind:value={newLesson.title}
					onblur={() => { if (!newLesson.slug) newLesson.slug = slugify(newLesson.title); }}
				/>
			</div>
			<div>
				<label for="lesson-slug" class="mb-1 block text-sm font-medium text-foreground">{t('learning.slug')}</label>
				<input
					id="lesson-slug"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.learning.placeholder_lesson_slug', 'e.g. introduction-to-closures')}
					bind:value={newLesson.slug}
				/>
			</div>
			<div>
				<label for="lesson-type" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.lesson_type', 'Lesson Type')}</label>
				<select
					id="lesson-type"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={newLesson.lesson_type}
				>
					{#each lessonTypeOptions as opt}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="lesson-content" class="mb-1 block text-sm font-medium text-foreground">{t('learning.content_markdown')}</label>
				<textarea
					id="lesson-content"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					rows="6"
					placeholder={t('app.learning.placeholder_lesson_body', 'Lesson content in Markdown...')}
					bind:value={newLesson.content}
				></textarea>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="lesson-duration" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.duration_minutes', 'Duration (min)')}</label>
					<input
						id="lesson-duration"
						type="number"
						min="0"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						placeholder="0"
						bind:value={newLesson.duration_minutes}
					/>
				</div>
				<div>
					<label for="lesson-sort-order" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.sort_order', 'Sort Order')}</label>
					<input
						id="lesson-sort-order"
						type="number"
						min="0"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						placeholder="0"
						bind:value={newLesson.sort_order}
					/>
				</div>
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
				{t('app.learning.creating', 'Creating...')}
			{:else}
				{t('app.learning.create', 'Create')}
			{/if}
		</Button>
	</div>
</Modal>

<!-- ══════════════════════════════════════════════════════════════════════════
     EDIT MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<Modal
	bind:open={showEditModal}
	title={editingItem?._type === "track"
		? t('app.learning.edit_track', 'Edit Track')
		: editingItem?._type === "module"
			? t('app.learning.edit_module', 'Edit Module')
			: t('app.learning.edit_lesson', 'Edit Lesson')}
	size="lg"
>
	{#if editingItem}
		{#if editingItem._type === "track"}
			<div class="space-y-4">
				<div>
					<label for="edit-track-title" class="mb-1 block text-sm font-medium text-foreground">{t('learning.title')}</label>
					<input
						id="edit-track-title"
						type="text"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.title}
					/>
				</div>
				<div>
					<label for="edit-track-description" class="mb-1 block text-sm font-medium text-foreground">{t('learning.description')}</label>
					<textarea
						id="edit-track-description"
						class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						rows="3"
						bind:value={editForm.description}
					></textarea>
				</div>
				<div>
					<label for="edit-track-difficulty" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.difficulty', 'Difficulty')}</label>
					<select
						id="edit-track-difficulty"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.difficulty}
					>
						{#each difficultyOptions as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="edit-track-sort-order" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.sort_order', 'Sort Order')}</label>
						<input
							id="edit-track-sort-order"
							type="number"
							min="0"
							class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
							bind:value={editForm.sort_order}
						/>
					</div>
					<div class="flex items-end pb-1">
						<div class="flex items-center gap-2">
							<input id="edit-track-published" type="checkbox" bind:checked={editForm.is_published} class="h-4 w-4 rounded" />
							<label for="edit-track-published" class="text-sm text-foreground">{t('app.learning.is_published', 'Published')}</label>
						</div>
					</div>
				</div>
			</div>
		{:else if editingItem._type === "module"}
			<div class="space-y-4">
				<div>
					<label for="edit-module-title" class="mb-1 block text-sm font-medium text-foreground">{t('learning.title')}</label>
					<input
						id="edit-module-title"
						type="text"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.title}
					/>
				</div>
				<div>
					<label for="edit-module-description" class="mb-1 block text-sm font-medium text-foreground">{t('learning.description')}</label>
					<textarea
						id="edit-module-description"
						class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						rows="3"
						bind:value={editForm.description}
					></textarea>
				</div>
				<div>
					<label for="edit-module-sort-order" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.sort_order', 'Sort Order')}</label>
					<input
						id="edit-module-sort-order"
						type="number"
						min="0"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.sort_order}
					/>
				</div>
			</div>
		{:else}
			<div class="space-y-4">
				<div>
					<label for="edit-lesson-title" class="mb-1 block text-sm font-medium text-foreground">{t('learning.title')}</label>
					<input
						id="edit-lesson-title"
						type="text"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.title}
					/>
				</div>
				<div>
					<label for="edit-lesson-type" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.lesson_type', 'Lesson Type')}</label>
					<select
						id="edit-lesson-type"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.lesson_type}
					>
						{#each lessonTypeOptions as opt}
							<option value={opt.value}>{opt.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="edit-lesson-content" class="mb-1 block text-sm font-medium text-foreground">{t('learning.content_markdown')}</label>
					<textarea
						id="edit-lesson-content"
						class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						rows="6"
						bind:value={editForm.content}
					></textarea>
				</div>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="edit-lesson-duration" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.duration_minutes', 'Duration (min)')}</label>
						<input
							id="edit-lesson-duration"
							type="number"
							min="0"
							class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
							bind:value={editForm.duration_minutes}
						/>
					</div>
					<div>
						<label for="edit-lesson-sort-order" class="mb-1 block text-sm font-medium text-foreground">{t('app.learning.sort_order', 'Sort Order')}</label>
						<input
							id="edit-lesson-sort-order"
							type="number"
							min="0"
							class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
							bind:value={editForm.sort_order}
						/>
					</div>
				</div>
				<div class="flex items-center gap-6">
					<div class="flex items-center gap-2">
						<input id="edit-lesson-published" type="checkbox" bind:checked={editForm.is_published} class="h-4 w-4 rounded" />
						<label for="edit-lesson-published" class="text-sm text-foreground">{t('app.learning.is_published', 'Published')}</label>
					</div>
				</div>
			</div>
		{/if}
	{/if}

	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showEditModal = false)}>{t('learning.cancel')}</Button>
		<Button onclick={saveEdit} disabled={savingEdit}>
			{#if savingEdit}
				{t('app.learning.saving', 'Saving...')}
			{:else}
				{t('app.learning.save', 'Save')}
			{/if}
		</Button>
	</div>
</Modal>

<!-- ══════════════════════════════════════════════════════════════════════════
     DELETE CONFIRMATION MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<Modal bind:open={showDeleteModal} title={t('app.learning.modal_delete_title', 'Delete Item')} description={t('app.learning.modal_delete_description', 'This action cannot be undone.')} size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-danger-50 p-4">
			<p class="text-sm text-danger-700">
				{t('app.learning.delete_confirm', 'Are you sure you want to delete')} <strong>{deleteTarget.title}</strong>?
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('learning.cancel')}</Button>
		<Button variant="destructive" onclick={deleteItem} disabled={deleting}>
			{#if deleting}
				{t('app.learning.deleting', 'Deleting...')}
			{:else}
				{t('app.learning.delete', 'Delete')}
			{/if}
		</Button>
	</div>
</Modal>

<!-- ══════════════════════════════════════════════════════════════════════════
     PUBLISH CONFIRM MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<Modal bind:open={showPublishModal} title={t('app.learning.modal_publish_title', 'Publish Track')} description={t('app.learning.modal_publish_description', 'This will make the track visible to learners.')} size="sm">
	{#if publishTarget}
		<div class="rounded-lg bg-success-50 p-4">
			<p class="text-sm text-success-700">
				{t('app.learning.publish_confirm', 'Publish')} <strong>{publishTarget.title}</strong>?
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showPublishModal = false)}>{t('learning.cancel')}</Button>
		<Button onclick={async () => { if (publishTarget) { await publishTrack(publishTarget.id); }}} disabled={publishing}>
			{#if publishing}
				{t('app.learning.publishing', 'Publishing...')}
			{:else}
				{t('app.learning.publish', 'Publish')}
			{/if}
		</Button>
	</div>
</Modal>

<!-- ══════════════════════════════════════════════════════════════════════════
     ENROLLMENTS MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<Modal
	bind:open={showEnrollmentsModal}
	title={enrollmentsTrack ? `${t('app.learning.enrollments_for', 'Enrollments for')} ${enrollmentsTrack.title}` : t('app.learning.enrollments', 'Enrollments')}
	size="xl"
>
	{#if loadingEnrollments}
		<div class="space-y-3">
			{#each Array(3) as _}
				<div class="h-10 animate-pulse rounded-lg bg-surface"></div>
			{/each}
		</div>
	{:else if enrollments.length === 0}
		<div class="rounded-lg bg-surface p-8 text-center">
			<p class="text-sm text-muted">{t('app.learning.no_enrollments', 'No enrollments found for this track.')}</p>
		</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b">
						<th class="px-3 py-2 text-left font-medium text-muted">{t('app.learning.col_user_id', 'User ID')}</th>
						<th class="px-3 py-2 text-left font-medium text-muted">{t('app.learning.col_access_type', 'Access Type')}</th>
						<th class="px-3 py-2 text-left font-medium text-muted">{t('app.learning.col_progress', 'Progress')}</th>
						<th class="px-3 py-2 text-left font-medium text-muted">{t('app.learning.col_enrolled_at', 'Enrolled')}</th>
					</tr>
				</thead>
				<tbody>
					{#each enrollments as enrollment}
						<tr class="border-b last:border-0">
							<td class="px-3 py-2 font-mono text-xs">{enrollment.user_id}</td>
							<td class="px-3 py-2">{enrollment.access_type}</td>
							<td class="px-3 py-2">
								<div class="flex items-center gap-2">
									<div class="h-2 w-24 overflow-hidden rounded-full bg-surface">
										<div class="h-full rounded-full bg-primary" style="width: {enrollment.progress_pct}%"></div>
									</div>
									<span class="text-xs text-muted">{enrollment.progress_pct}%</span>
								</div>
							</td>
							<td class="px-3 py-2 text-xs text-muted">{new Date(enrollment.enrolled_at).toLocaleDateString()}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	<div class="flex justify-end pt-4">
		<Button variant="outline" onclick={() => (showEnrollmentsModal = false)}>{t('learning.close', 'Close')}</Button>
	</div>
</Modal>

<!-- ══════════════════════════════════════════════════════════════════════════
     ACTION CELLS
     ══════════════════════════════════════════════════════════════════════════ -->
{#snippet TrackActionsCell({ value, row }: { value: any; row: any })}
	<div class="flex items-center gap-1">
		{#if canManageLearning && row.is_published === false}
			<Button variant="ghost" size="sm" onclick={() => openPublish(row.id, row.title)} style="color: var(--color-success)">
				<Send class="h-3.5 w-3.5" />
			</Button>
		{/if}
		{#if canManageLearning}
			<Button variant="ghost" size="sm" onclick={() => openEdit("track", row)} style="color: var(--color-info)">
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openEnrollments(row.id, row.title)} style="color: var(--color-primary)">
				<Users class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("track", row.id, row.title)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

{#snippet ModuleActionsCell({ value, row }: { value: any; row: any })}
	<div class="flex items-center gap-1">
		{#if canManageLearning}
			<Button variant="ghost" size="sm" onclick={() => openEdit("module", row)} style="color: var(--color-info)">
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("module", row.id, row.title)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

{#snippet LessonActionsCell({ value, row }: { value: any; row: any })}
	<div class="flex items-center gap-1">
		{#if canManageLearning}
			<Button variant="ghost" size="sm" onclick={() => openEdit("lesson", row)} style="color: var(--color-info)">
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("lesson", row.id, row.title)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
