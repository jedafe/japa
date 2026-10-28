<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { Tabs } from "$lib/components/ui/tabs";
	import { DataTable } from "$lib/components/ui/data-table";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import type { ActionItem, BulkAction } from "$lib/components/ui/data-table";
	import { Plus, GraduationCap, Layers, FileText, Trash2, Send, Pencil, Users } from "lucide-svelte";
	import { onMount } from "svelte";
	import { api } from "$lib/api";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	// ── Types ──────────────────────────────────────────────────────────────

	type Track = {
		id: string;
		title: string;
		slug: string;
		description: string;
		difficulty: string;
		is_published: boolean;
		sort_order: number;
		category?: string;
		created_at: string;
		updated_at: string;
	};

	type Category = { id: string; slug: string; name: string; sort_order: number };

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
		username?: string;
		display_name?: string;
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
	let searchQuery = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Create SlideOver
	let showCreateSlideOver = $state(false);
	let creating = $state(false);
	let newTrack = $state({ title: "", slug: "", description: "", difficulty: "", category: "", subscription_eligible: false });
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

	// Edit SlideOver
	let showEditSlideOver = $state(false);
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
	let academy_enrollments = $state<Enrollment[]>([]);
	let loadingEnrollments = $state(false);

	// Modules for lesson create dropdown
	let modulesForLesson = $state<Module[]>([]);

	// Categories from the public API (Rule 16 — no hardcoded option lists)
	let categories = $state<Category[]>([]);

	const canManageAcademy = $derived(hasPermission("academy.admin.manage"));

	const tabs = $derived([
		{ value: "tracks", label: t('app.academy.tab_tracks', 'Tracks'), icon: GraduationCap, count: tracks.length },
		{ value: "modules", label: t('app.academy.tab_modules', 'Modules'), icon: Layers, count: modules.length },
		{ value: "lessons", label: t('app.academy.tab_lessons', 'Lessons'), icon: FileText, count: lessons.length }
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
			label: t('app.academy.col_track', 'Track'),
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					<p class="text-sm text-muted line-clamp-1">${row.description || t('app.academy.no_description', 'No description')}</p>
				</div>
			`
		},
		{
			key: "difficulty",
			label: t('app.academy.col_difficulty', 'Difficulty'),
			render: (value: string) => `<span class="text-sm text-foreground-alt">${value || "—"}</span>`
		},
		{
			key: "category",
			label: t('app.academy.col_category', 'Category'),
			render: (value: string) => `<span class="text-sm text-foreground-alt">${value || "—"}</span>`
		},
		{
			key: "subscription_eligible",
			label: t('app.academy.col_sub_eligible', 'Sub.'),
			render: (value: boolean) => {
				if (value) {
					return `<span class="inline-flex items-center rounded-full border bg-primary/15 text-primary border-primary px-2 py-0.5 text-xs font-semibold">${t('app.academy.sub_badge', 'Sub')}</span>`;
				}
				return `<span class="text-sm text-muted">—</span>`;
			}
		},
		{
			key: "is_published",
			label: t('app.academy.col_status', 'Status'),
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
		}
	];

	const moduleColumns = [
		{
			key: "title",
			label: t('app.academy.col_module', 'Module'),
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					${row.track_title ? `<p class="text-sm text-muted">${row.track_title}</p>` : ""}
				</div>
			`
		},
		{
			key: "lesson_count",
			label: t('app.academy.col_lessons', 'Lessons'),
			render: (value: number) => `<span class="font-medium">${value ?? 0}</span>`
		},
		{
			key: "sort_order",
			label: t('app.academy.col_order', 'Order'),
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value}</span>`
		}
	];

	const lessonColumns = [
		{
			key: "title",
			label: t('app.academy.col_lesson', 'Lesson'),
			render: (_: any, row: any) => `
				<div>
					<p class="font-medium text-foreground">${row.title}</p>
					${row.module_title ? `<p class="text-sm text-muted">${row.module_title}</p>` : ""}
				</div>
			`
		},
		{
			key: "lesson_type",
			label: t('app.academy.col_type', 'Type'),
			render: (value: string) => `<span class="text-sm text-foreground-alt">${value || "—"}</span>`
		},
		{
			key: "duration_minutes",
			label: t('app.academy.col_duration', 'Duration'),
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value ? `${value} ${t('app.academy.min', 'min')}` : "—"}</span>`
		},
		{
			key: "sort_order",
			label: t('app.academy.col_order', 'Order'),
			render: (value: number) => `<span class="text-sm text-foreground-alt">${value}</span>`
		}
	];

	const trackActions: ActionItem[] = [
		{ label: t('app.academy.publish', 'Publish'), icon: Send, onclick: (row) => openPublish(row.id, row.title), show: (row) => canManageAcademy && row.is_published === false },
		{ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (row) => openEdit("track", row), show: () => canManageAcademy },
		{ label: t('app.academy.enrollments', 'Enrollments'), icon: Users, onclick: (row) => openEnrollments(row.id, row.title), show: () => canManageAcademy },
		{ label: t('common.delete', 'Delete'), icon: Trash2, variant: 'destructive', onclick: (row) => confirmDelete("track", row.id, row.title), show: () => canManageAcademy }
	];

	const moduleActions: ActionItem[] = [
		{ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (row) => openEdit("module", row), show: () => canManageAcademy },
		{ label: t('common.delete', 'Delete'), icon: Trash2, variant: 'destructive', onclick: (row) => confirmDelete("module", row.id, row.title), show: () => canManageAcademy }
	];

	const lessonActions: ActionItem[] = [
		{ label: t('common.edit', 'Edit'), icon: Pencil, onclick: (row) => openEdit("lesson", row), show: () => canManageAcademy },
		{ label: t('common.delete', 'Delete'), icon: Trash2, variant: 'destructive', onclick: (row) => confirmDelete("lesson", row.id, row.title), show: () => canManageAcademy }
	];

	let currentColumns = $derived(
		activeTab === "tracks" ? trackColumns : activeTab === "modules" ? moduleColumns : lessonColumns
	);

	let currentActions = $derived(
		activeTab === "tracks" ? trackActions : activeTab === "modules" ? moduleActions : lessonActions
	);

	const bulkActions: BulkAction[] = $derived(canManageAcademy
		? [{ label: t('app.academy.delete_selected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }]
		: []);

	let currentData = $derived.by(() => {
		const source = activeTab === "tracks" ? tracks : activeTab === "modules" ? modules : lessons;
		const q = searchQuery.toLowerCase().trim();
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
				const result = await api<Track[]>("/admin/academy/curriculum/tracks");
				if (result.ok) {
					tracks = result.data ?? [];
				} else {
					error = result.error || t('app.academy.error_load', 'Failed to load data');
				}
			} else if (activeTab === "modules") {
				const result = await api<Module[]>("/admin/academy/curriculum/modules");
				if (result.ok) {
					modules = result.data ?? [];
				} else {
					error = result.error || t('app.academy.error_load', 'Failed to load data');
				}
			} else {
				const result = await api<Lesson[]>("/admin/academy/curriculum/lessons");
				if (result.ok) {
					lessons = result.data ?? [];
				} else {
					error = result.error || t('app.academy.error_load', 'Failed to load data');
				}
			}
		} catch {
			error = t('app.academy.error_load', 'Failed to load data');
		} finally {
			loading = false;
		}
	}

	async function fetchTracks() {
		const result = await api<Track[]>("/admin/academy/curriculum/tracks");
		if (result.ok) {
			tracks = result.data ?? [];
		}
	}

	async function fetchModulesForTrack(trackId: string) {
		const result = await api<Module[]>(`/admin/academy/curriculum/tracks/${trackId}/modules`);
		if (result.ok) {
			modulesForLesson = result.data ?? [];
		} else {
			modulesForLesson = [];
		}
	}

	async function fetchCategories() {
		const result = await api<Category[]>("/academy/categories");
		if (result.ok) {
			categories = result.data ?? [];
		}
	}

	function handleFilter(_values: Record<string, string>) {
		// search handled via DataTable onsearch; kept for compatibility
	}

	// ── Create ─────────────────────────────────────────────────────────────

	function openCreate() {
		showCreateSlideOver = true;
	}

	async function createTrack() {
		if (!newTrack.title.trim() || creating) return;
		creating = true;
		error = "";
		try {
			const slug = newTrack.slug.trim() || slugify(newTrack.title);
			const result = await api("/admin/academy/curriculum/tracks", {
				method: "POST",
				body: {
					title: newTrack.title.trim(),
					slug,
					description: newTrack.description.trim(),
					difficulty: newTrack.difficulty,
					category: newTrack.category || undefined,
					subscription_eligible: newTrack.subscription_eligible
				}
			});
			if (result.ok) {
				showCreateSlideOver = false;
				newTrack = { title: "", slug: "", description: "", difficulty: "", category: "", subscription_eligible: false };
				await refresh();
			} else {
				error = result.error || t('app.academy.error_create_track', 'Failed to create track');
			}
		} catch {
			error = t('app.academy.error_create_track', 'Failed to create track');
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
			const result = await api(`/admin/academy/curriculum/tracks/${newModule.track_id}/modules`, {
				method: "POST",
				body: {
					title: newModule.title.trim(),
					slug,
					description: newModule.description.trim()
				}
			});
			if (result.ok) {
				showCreateSlideOver = false;
				newModule = { track_id: "", title: "", slug: "", description: "" };
				await refresh();
			} else {
				error = result.error || t('app.academy.error_create_module', 'Failed to create module');
			}
		} catch {
			error = t('app.academy.error_create_module', 'Failed to create module');
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
			const result = await api(`/admin/academy/curriculum/modules/${newLesson.module_id}/lessons`, {
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
				showCreateSlideOver = false;
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
				error = result.error || t('app.academy.error_create_lesson', 'Failed to create lesson');
			}
		} catch {
			error = t('app.academy.error_create_lesson', 'Failed to create lesson');
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
			const result = await api(`/admin/academy/curriculum/tracks/${id}/publish`, {
				method: "POST",
				body: { is_published: true }
			});
			if (result.ok) {
				showPublishModal = false;
				publishTarget = null;
				await refresh();
			} else {
				error = result.error || t('app.academy.error_publish_track', 'Failed to publish track');
			}
		} catch {
			error = t('app.academy.error_publish_track', 'Failed to publish track');
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
				sort_order: item.sort_order ?? 0,
				category: item.category || "",
				subscription_eligible: item.subscription_eligible ?? false
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
		showEditSlideOver = true;
	}

	async function saveEdit() {
		if (!editingItem || savingEdit) return;
		savingEdit = true;
		error = "";
		try {
			const { _type, id } = editingItem;
			let result;
			if (_type === "track") {
				const { category, ...rest } = editForm;
				result = await api(`/admin/academy/curriculum/tracks/${id}`, {
					method: "PATCH",
					body: { ...rest, category: category || undefined }
				});
			} else if (_type === "module") {
				result = await api(`/admin/academy/curriculum/modules/${id}`, {
					method: "PATCH",
					body: editForm
				});
			} else {
				result = await api(`/admin/academy/curriculum/lessons/${id}`, {
					method: "PATCH",
					body: editForm
				});
			}
			if (result.ok) {
				showEditSlideOver = false;
				editingItem = null;
				await refresh();
			} else {
				error = result.error || t('app.academy.error_save', 'Failed to save changes');
			}
		} catch {
			error = t('app.academy.error_save', 'Failed to save changes');
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
				result = await api(`/admin/academy/curriculum/tracks/${id}`, { method: "DELETE" });
			} else if (type === "module") {
				result = await api(`/admin/academy/curriculum/modules/${id}`, { method: "DELETE" });
			} else {
				result = await api(`/admin/academy/curriculum/lessons/${id}`, { method: "DELETE" });
			}
			if (result.ok) {
				showDeleteModal = false;
				deleteTarget = null;
				await refresh();
			} else {
				error = result.error || t('app.academy.error_delete_item', 'Failed to delete item');
			}
		} catch {
			error = t('app.academy.error_delete_item', 'Failed to delete item');
		} finally {
			deleting = false;
		}
	}

	// ── Enrollments ────────────────────────────────────────────────────────

	async function openEnrollments(id: string, title: string) {
		enrollmentsTrack = { id, title };
		showEnrollmentsModal = true;
		loadingEnrollments = true;
		academy_enrollments = [];
		try {
			const result = await api<Enrollment[]>(`/admin/academy/curriculum/tracks/${id}/enrollments`);
			if (result.ok) {
				academy_enrollments = result.data ?? [];
			} else {
				error = result.error || t('app.academy.error_enrollments', 'Failed to load enrollments');
			}
		} catch {
			error = t('app.academy.error_enrollments', 'Failed to load enrollments');
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
				? (id: string) => `/admin/academy/curriculum/tracks/${id}`
				: activeTab === "modules"
					? (id: string) => `/admin/academy/curriculum/modules/${id}`
					: (id: string) => `/admin/academy/curriculum/lessons/${id}`;
			await Promise.all(ids.map((id) => api(endpoint(id), { method: "DELETE" })));
			selectedIds = new Set();
			await refresh();
		} catch {
			error = t('app.academy.error_delete_bulk', 'Failed to delete selected items');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Effects ────────────────────────────────────────────────────────────

	onMount(() => {
		fetchCategories();
	});

	$effect(() => {
		refresh();
	});

	$effect(() => {
		void activeTab;
		selectedIds = new Set();
	});

	// When lesson create modal opens and we're on the lessons tab, pre-fetch tracks
	$effect(() => {
		if (showCreateSlideOver && activeTab === "lessons") {
			fetchTracks();
		}
	});

	// When module create modal opens, pre-fetch tracks
	$effect(() => {
		if (showCreateSlideOver && activeTab === "modules") {
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
	<title>{t('app.academy.page_title', 'Academy Curriculum — {site} Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.academy.title', 'Academy Curriculum')}
	description={t('app.academy.description', 'Create and manage courses, modules, and lessons')}
	actions={[
		...(canManageAcademy ? [{ label: `${t('app.academy.new_prefix', 'New')} ${activeTab === "tracks" ? t('app.academy.track', 'Track') : activeTab === "modules" ? t('app.academy.module', 'Module') : t('app.academy.lesson', 'Lesson')}`, icon: Plus, onclick: openCreate }] : [])
	]}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg border border-danger bg-danger-50 p-4 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

	<Card>
		<CardContent class="p-0">
			<DataTable
				columns={currentColumns}
				data={currentData}
				{loading}
				pageSize={15}
				emptyMessage={t(`app.academy.empty_${activeTab}`, `No ${activeTab} found`)}
				searchable
				searchPlaceholder={t('app.academy.search_placeholder', 'Search academy content...')}
				onsearch={(q) => { searchQuery = q; }}
				selectable={canManageAcademy}
				bind:selectedIds
				actions={currentActions}
				bulkActionsList={bulkActions}
			/>
		</CardContent>
	</Card>
</div>

<!-- ══════════════════════════════════════════════════════════════════════════
     CREATE MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<SlideOver
	bind:open={showCreateSlideOver}
	title={`${t('app.academy.create_prefix', 'Create')} ${activeTab === "tracks" ? t('app.academy.track', 'Track') : activeTab === "modules" ? t('app.academy.module', 'Module') : t('app.academy.lesson', 'Lesson')}`}
	size="lg"
>
	{#if activeTab === "tracks"}
		<div class="space-y-4">
			<div>
				<label for="track-title" class="mb-1 block text-sm font-medium text-foreground">{t('academy.title')}</label>
				<input
					id="track-title"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.academy.placeholder_track_title', 'e.g. JavaScript Fundamentals')}
					bind:value={newTrack.title}
					onblur={() => { if (!newTrack.slug) newTrack.slug = slugify(newTrack.title); }}
				/>
			</div>
			<div>
				<label for="track-slug" class="mb-1 block text-sm font-medium text-foreground">{t('academy.slug')}</label>
				<input
					id="track-slug"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.academy.placeholder_track_slug', 'e.g. javascript-fundamentals')}
					bind:value={newTrack.slug}
				/>
			</div>
			<div>
				<label for="track-description" class="mb-1 block text-sm font-medium text-foreground">{t('academy.description')}</label>
				<textarea
					id="track-description"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					rows="3"
					placeholder={t('app.academy.placeholder_track_description', 'What will students learn?')}
					bind:value={newTrack.description}
				></textarea>
			</div>
			<div>
				<label for="track-difficulty" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.difficulty', 'Difficulty')}</label>
				<AutocompleteSelect
					id="track-difficulty"
					bind:value={newTrack.difficulty}
					options={difficultyOptions}
					class="w-full"
				/>
			</div>
			<div>
				<label for="track-category" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.col_category', 'Category')}</label>
				<AutocompleteSelect
					id="track-category"
					bind:value={newTrack.category}
					options={[{ value: '', label: '—' }, ...categories.map((cat) => ({ value: cat.slug, label: cat.name }))]}
					class="w-full"
				/>
			</div>
			<div class="flex items-center gap-2 pt-2">
				<input id="track-subscription-eligible" type="checkbox" bind:checked={newTrack.subscription_eligible} class="h-4 w-4 rounded" />
				<label for="track-subscription-eligible" class="text-sm text-foreground">{t('app.academy.subscription_eligible', 'Subscription eligible')}</label>
				<span class="text-xs text-muted">{t('app.academy.subscription_eligible_hint', '(included in subscription packages)')}</span>
			</div>
		</div>
	{:else if activeTab === "modules"}
		<div class="space-y-4">
			<div>
				<label for="module-track-id" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.track', 'Track')}</label>
				<AutocompleteSelect
					id="module-track-id"
					bind:value={newModule.track_id}
					options={[{ value: '', label: t('app.academy.select_track', 'Select a track...') }, ...tracks.map((track) => ({ value: track.id, label: track.title }))]}
					class="w-full"
				/>
			</div>
			<div>
				<label for="module-title" class="mb-1 block text-sm font-medium text-foreground">{t('academy.title')}</label>
				<input
					id="module-title"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.academy.placeholder_module_title', 'e.g. Variables and Types')}
					bind:value={newModule.title}
					onblur={() => { if (!newModule.slug) newModule.slug = slugify(newModule.title); }}
				/>
			</div>
			<div>
				<label for="module-slug" class="mb-1 block text-sm font-medium text-foreground">{t('academy.slug')}</label>
				<input
					id="module-slug"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.academy.placeholder_module_slug', 'e.g. variables-and-types')}
					bind:value={newModule.slug}
				/>
			</div>
			<div>
				<label for="module-description" class="mb-1 block text-sm font-medium text-foreground">{t('academy.description')}</label>
				<textarea
					id="module-description"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					rows="3"
					placeholder={t('app.academy.placeholder_module_description', 'What will students learn in this module?')}
					bind:value={newModule.description}
				></textarea>
			</div>
		</div>
	{:else}
		<div class="space-y-4">
			<div>
				<label for="lesson-track-select" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.track', 'Track')}</label>
				<AutocompleteSelect
					id="lesson-track-select"
					bind:value={newLesson.selectedTrackId}
					options={[{ value: '', label: t('app.academy.select_track', 'Select a track...') }, ...tracks.map((track) => ({ value: track.id, label: track.title }))]}
					class="w-full"
				/>
			</div>
			<div>
				<label for="lesson-module-select" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.module', 'Module')}</label>
				<AutocompleteSelect
					id="lesson-module-select"
					bind:value={newLesson.module_id}
					disabled={!newLesson.selectedTrackId}
					options={[{ value: '', label: t('app.academy.select_module', 'Select a module...') }, ...modulesForLesson.map((mod) => ({ value: mod.id, label: mod.title }))]}
					class="w-full"
				/>
			</div>
			<div>
				<label for="lesson-title" class="mb-1 block text-sm font-medium text-foreground">{t('academy.title')}</label>
				<input
					id="lesson-title"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.academy.placeholder_lesson_title', 'e.g. Introduction to Closures')}
					bind:value={newLesson.title}
					onblur={() => { if (!newLesson.slug) newLesson.slug = slugify(newLesson.title); }}
				/>
			</div>
			<div>
				<label for="lesson-slug" class="mb-1 block text-sm font-medium text-foreground">{t('academy.slug')}</label>
				<input
					id="lesson-slug"
					type="text"
					class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					placeholder={t('app.academy.placeholder_lesson_slug', 'e.g. introduction-to-closures')}
					bind:value={newLesson.slug}
				/>
			</div>
			<div>
				<label for="lesson-type" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.lesson_type', 'Lesson Type')}</label>
				<AutocompleteSelect
					id="lesson-type"
					bind:value={newLesson.lesson_type}
					options={lessonTypeOptions}
					class="w-full"
				/>
			</div>
			<div>
				<label for="lesson-content" class="mb-1 block text-sm font-medium text-foreground">{t('academy.content_markdown')}</label>
				<textarea
					id="lesson-content"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					rows="6"
					placeholder={t('app.academy.placeholder_lesson_body', 'Lesson content in Markdown...')}
					bind:value={newLesson.content}
				></textarea>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<label for="lesson-duration" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.duration_minutes', 'Duration (min)')}</label>
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
					<label for="lesson-sort-order" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.sort_order', 'Sort Order')}</label>
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

	{#snippet footer()}
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (showCreateSlideOver = false)}>{t('academy.cancel')}</Button>
			<Button
				onclick={activeTab === "tracks" ? createTrack : activeTab === "modules" ? createModule : createLesson}
				disabled={creating}
			>
				{#if creating}
					{t('app.academy.creating', 'Creating...')}
				{:else}
					{t('app.academy.create', 'Create')}
				{/if}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- ══════════════════════════════════════════════════════════════════════════
     EDIT MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<SlideOver
	bind:open={showEditSlideOver}
	title={editingItem?._type === "track"
		? t('app.academy.edit_track', 'Edit Track')
		: editingItem?._type === "module"
			? t('app.academy.edit_module', 'Edit Module')
			: t('app.academy.edit_lesson', 'Edit Lesson')}
	size="lg"
>
	{#if editingItem}
		{#if editingItem._type === "track"}
			<div class="space-y-4">
				<div>
					<label for="edit-track-title" class="mb-1 block text-sm font-medium text-foreground">{t('academy.title')}</label>
					<input
						id="edit-track-title"
						type="text"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.title}
					/>
				</div>
				<div>
					<label for="edit-track-description" class="mb-1 block text-sm font-medium text-foreground">{t('academy.description')}</label>
					<textarea
						id="edit-track-description"
						class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						rows="3"
						bind:value={editForm.description}
					></textarea>
				</div>
				<div>
					<label for="edit-track-difficulty" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.difficulty', 'Difficulty')}</label>
					<AutocompleteSelect
						id="edit-track-difficulty"
						bind:value={editForm.difficulty}
						options={difficultyOptions}
						class="w-full"
					/>
				</div>
				<div>
					<label for="edit-track-category" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.col_category', 'Category')}</label>
					<AutocompleteSelect
						id="edit-track-category"
						bind:value={editForm.category}
						options={[{ value: '', label: '—' }, ...categories.map((cat) => ({ value: cat.slug, label: cat.name }))]}
						class="w-full"
					/>
				</div>
				<div class="flex items-center gap-2">
					<input id="edit-track-subscription-eligible" type="checkbox" bind:checked={editForm.subscription_eligible} class="h-4 w-4 rounded" />
					<label for="edit-track-subscription-eligible" class="text-sm text-foreground">{t('app.academy.subscription_eligible', 'Subscription eligible')}</label>
					<span class="text-xs text-muted">{t('app.academy.subscription_eligible_hint', '(included in subscription packages)')}</span>
				</div>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="edit-track-sort-order" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.sort_order', 'Sort Order')}</label>
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
							<label for="edit-track-published" class="text-sm text-foreground">{t('app.academy.is_published', 'Published')}</label>
						</div>
					</div>
				</div>
			</div>
		{:else if editingItem._type === "module"}
			<div class="space-y-4">
				<div>
					<label for="edit-module-title" class="mb-1 block text-sm font-medium text-foreground">{t('academy.title')}</label>
					<input
						id="edit-module-title"
						type="text"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.title}
					/>
				</div>
				<div>
					<label for="edit-module-description" class="mb-1 block text-sm font-medium text-foreground">{t('academy.description')}</label>
					<textarea
						id="edit-module-description"
						class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						rows="3"
						bind:value={editForm.description}
					></textarea>
				</div>
				<div>
					<label for="edit-module-sort-order" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.sort_order', 'Sort Order')}</label>
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
					<label for="edit-lesson-title" class="mb-1 block text-sm font-medium text-foreground">{t('academy.title')}</label>
					<input
						id="edit-lesson-title"
						type="text"
						class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						bind:value={editForm.title}
					/>
				</div>
				<div>
					<label for="edit-lesson-type" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.lesson_type', 'Lesson Type')}</label>
					<AutocompleteSelect
						id="edit-lesson-type"
						bind:value={editForm.lesson_type}
						options={lessonTypeOptions}
						class="w-full"
					/>
				</div>
				<div>
					<label for="edit-lesson-content" class="mb-1 block text-sm font-medium text-foreground">{t('academy.content_markdown')}</label>
					<textarea
						id="edit-lesson-content"
						class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						rows="6"
						bind:value={editForm.content}
					></textarea>
				</div>
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="edit-lesson-duration" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.duration_minutes', 'Duration (min)')}</label>
						<input
							id="edit-lesson-duration"
							type="number"
							min="0"
							class="h-10 w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
							bind:value={editForm.duration_minutes}
						/>
					</div>
					<div>
						<label for="edit-lesson-sort-order" class="mb-1 block text-sm font-medium text-foreground">{t('app.academy.sort_order', 'Sort Order')}</label>
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
						<label for="edit-lesson-published" class="text-sm text-foreground">{t('app.academy.is_published', 'Published')}</label>
					</div>
				</div>
			</div>
		{/if}
	{/if}

	{#snippet footer()}
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (showEditSlideOver = false)}>{t('academy.cancel')}</Button>
			<Button onclick={saveEdit} disabled={savingEdit}>
				{#if savingEdit}
					{t('app.academy.saving', 'Saving...')}
				{:else}
					{t('app.academy.save', 'Save')}
				{/if}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- ══════════════════════════════════════════════════════════════════════════
     DELETE CONFIRMATION MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<Modal bind:open={showDeleteModal} title={t('app.academy.modal_delete_title', 'Delete Item')} description={t('app.academy.modal_delete_description', 'This action cannot be undone.')} size="sm">
	{#if deleteTarget}
		<div class="rounded-lg bg-danger-50 p-4">
			<p class="text-sm text-danger-700">
				{t('app.academy.delete_confirm', 'Are you sure you want to delete')} <strong>{deleteTarget.title}</strong>?
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('academy.cancel')}</Button>
		<Button variant="destructive" onclick={deleteItem} disabled={deleting}>
			{#if deleting}
				{t('app.academy.deleting', 'Deleting...')}
			{:else}
				{t('app.academy.delete', 'Delete')}
			{/if}
		</Button>
	</div>
</Modal>

<!-- ══════════════════════════════════════════════════════════════════════════
     PUBLISH CONFIRM MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<Modal bind:open={showPublishModal} title={t('app.academy.modal_publish_title', 'Publish Track')} description={t('app.academy.modal_publish_description', 'This will make the track visible to learners.')} size="sm">
	{#if publishTarget}
		<div class="rounded-lg bg-success-50 p-4">
			<p class="text-sm text-success-700">
				{t('app.academy.publish_confirm', 'Publish')} <strong>{publishTarget.title}</strong>?
			</p>
		</div>
	{/if}
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showPublishModal = false)}>{t('academy.cancel')}</Button>
		<Button onclick={async () => { if (publishTarget) { await publishTrack(publishTarget.id); }}} disabled={publishing}>
			{#if publishing}
				{t('app.academy.publishing', 'Publishing...')}
			{:else}
				{t('app.academy.publish', 'Publish')}
			{/if}
		</Button>
	</div>
</Modal>

<!-- ══════════════════════════════════════════════════════════════════════════
     ENROLLMENTS MODAL
     ══════════════════════════════════════════════════════════════════════════ -->
<Modal
	bind:open={showEnrollmentsModal}
	title={enrollmentsTrack ? `${t('app.academy.enrollments_for', 'Enrollments for')} ${enrollmentsTrack.title}` : t('app.academy.enrollments', 'Enrollments')}
	size="xl"
>
	{#if loadingEnrollments}
		<div class="space-y-3">
			{#each Array(3) as _, i (i)}
				<div class="h-10 animate-pulse rounded-lg bg-surface"></div>
			{/each}
		</div>
	{:else if academy_enrollments.length === 0}
		<div class="rounded-lg bg-surface p-8 text-center">
			<p class="text-sm text-muted">{t('app.academy.no_enrollments', 'No enrollments found for this track.')}</p>
		</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b">
						<th class="px-3 py-2 text-start font-medium text-muted">{t('app.academy.col_user', 'User')}</th>
						<th class="px-3 py-2 text-start font-medium text-muted">{t('app.academy.col_access_type', 'Access Type')}</th>
						<th class="px-3 py-2 text-start font-medium text-muted">{t('app.academy.col_progress', 'Progress')}</th>
						<th class="px-3 py-2 text-start font-medium text-muted">{t('app.academy.col_enrolled_at', 'Enrolled')}</th>
					</tr>
				</thead>
				<tbody>
					{#each academy_enrollments as enrollment (enrollment)}
						<tr class="border-b last:border-0">
							<td class="px-3 py-2">
							{#if enrollment.display_name}
								<div>
									<p class="font-medium text-foreground">{enrollment.display_name}</p>
									<p class="text-xs text-muted">@{enrollment.username}</p>
								</div>
							{:else if enrollment.username}
								<span class="font-medium text-foreground">@{enrollment.username}</span>
							{:else}
								<span class="font-mono text-xs text-muted">{enrollment.user_id}</span>
							{/if}
						</td>
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
		<Button variant="outline" onclick={() => (showEnrollmentsModal = false)}>{t('academy.close', 'Close')}</Button>
	</div>
</Modal>

