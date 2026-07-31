<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Button } from "$lib/components/ui/button";
	import { Modal } from "$lib/components/ui/modal";
	import Pagination from "$lib/components/ui/Pagination.svelte";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Plus, Pencil, Trash2, RefreshCw, Eye, Shield, Image as ImageIcon, X } from "lucide-svelte";
	import UserAutocomplete from "$lib/components/ui/UserAutocomplete.svelte";
	import MediaPickerModal from "$lib/components/ui/MediaPickerModal.svelte";
	import { tagsAdminApi, tagsModeratorsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { page } from "$app/state";
	import { goto } from "$app/navigation";

	type Tag = {
		id: string;
		name: string;
		slug: string;
		description?: string;
		color?: string;
		logo_url?: string;
		logo_media_id?: string;
		content_count: number;
		post_count: number;
		follower_count: number;
		is_system: boolean;
		created_at: string;
	};

	type TagModerator = {
		user_id: string;
		username: string;
		created_at: string;
	};

	let tags = $state<Tag[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let currentPage = $state(0);
	let totalItems = $state(0);
	let pageSize = $state(25);
	let totalPages = $state(1);
	let searchQuery = $state(page.url.searchParams.get("q") ?? "");
	let filterValues = $state<Record<string, string>>(
		page.url.searchParams.get("filter") ? { post_count: page.url.searchParams.get("filter")! } : {}
	);
	let selectedIds = $state<Set<string>>(new Set());
	let sortKey = $state(page.url.searchParams.get("sort") ?? "");

	// mapTag normalizes the API response: content_count → post_count for display.
	function mapTag(raw: any): Tag {
		return {
			...raw,
			content_count: raw.content_count ?? 0,
			post_count: raw.content_count ?? raw.post_count ?? 0,
			is_system: raw.is_system ?? false,
			follower_count: raw.follower_count ?? 0,
			logo_url: raw.logo_url ?? '',
			logo_media_id: raw.logo_media_id ?? ''
		};
	}

	// Modals
	let showViewModal = $state(false);
	let viewTarget = $state<Tag | null>(null);
	let showModal = $state(false);
	let editingTag = $state<Tag | null>(null);
	let formName = $state("");
	let formSlug = $state("");
	let formDescription = $state("");
	let formColor = $state("");
	let formIsSystem = $state(false);
	let formLogoMediaId = $state("");
	let formLogoUrl = $state("");
	let saving = $state(false);
	let showMediaPicker = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Tag | null>(null);

	const canManageTags = $derived(hasPermission("tags.manage"));

	const filterDefs = [
		{
			key: "post_count",
			label: t("tags.min_posts", "Min Posts"),
			options: [
				{ value: "0", label: "Any" },
				{ value: "10", label: "10+" },
				{ value: "100", label: "100+" },
				{ value: "1000", label: "1000+" }
			]
		}
	];

	const columns = [
		{ key: "logo_url", label: t("tags.logo", "Logo"), sortable: false, class: "w-16", componentSnippet: TagLogoCell },
		{ key: "name", label: t("tags.name", "Name"), sortable: true },
		{ key: "slug", label: t("tags.slug", "Slug"), sortable: true },
		{ key: "post_count", label: t("tags.posts", "Posts"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
		{ key: "follower_count", label: t("tags.followers", "Followers"), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => new Date(v).toLocaleDateString() },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-36", componentSnippet: TagActionsCell }
	];

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
	}

	async function loadTags() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams({
				page: String(currentPage),
				limit: String(pageSize)
			});
			if (searchQuery) {
				params.set("q", searchQuery);
			}
			if (filterValues.post_count) {
				params.set("min_content_count", filterValues.post_count);
			}
			if (sortKey) {
				params.set("sort", sortKey);
			}

			const result = await tagsAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as { data: Tag[]; total: number; page: number; total_pages: number } | Tag[];
				if (Array.isArray(d)) {
					tags = d.map(mapTag);
					totalItems = d.length;
					totalPages = 1;
				} else if (d.data) {
					tags = d.data.map(mapTag);
					totalItems = d.total ?? d.data.length;
					totalPages = d.total_pages ?? 1;
				} else {
					tags = [];
					totalItems = 0;
					totalPages = 1;
				}
			} else {
				error = result.error ?? t("common.error", "Failed to load tags");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load tags");
		} finally {
			loading = false;
		}
	}

	function syncUrl() {
		const params = new URLSearchParams(page.url.searchParams.toString());
		if (searchQuery) params.set("q", searchQuery); else params.delete("q");
		if (filterValues.post_count) params.set("filter", filterValues.post_count); else params.delete("filter");
		if (sortKey) params.set("sort", sortKey); else params.delete("sort");
		goto(`?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	function handleSort(key: string) {
		sortKey = sortKey === key ? "" : key;
		currentPage = 0;
		syncUrl();
		loadTags();
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? "";
		filterValues = values.filter ? { post_count: values.filter } : {};
		currentPage = 0;
		syncUrl();
	}

	function handleClear() {
		searchQuery = "";
		filterValues = {};
		currentPage = 0;
		syncUrl();
	}

	function openView(tag: Tag) {
		viewTarget = tag;
		showViewModal = true;
	}

	function openCreate() {
		editingTag = null;
		formName = "";
		formSlug = "";
		formDescription = "";
		formColor = "";
		formIsSystem = false;
		formLogoMediaId = "";
		formLogoUrl = "";
		showModal = true;
	}

	function openEdit(tag: Tag) {
		editingTag = tag;
		formName = tag.name;
		formSlug = tag.slug;
		formDescription = tag.description ?? "";
		formColor = tag.color ?? "";
		formIsSystem = tag.is_system ?? false;
		formLogoMediaId = tag.logo_media_id ?? "";
		formLogoUrl = tag.logo_url ?? "";
		showModal = true;
	}

	async function submit() {
		if (!formName.trim()) return;
		saving = true;
		error = "";
		const body: { name: string; slug: string; description?: string; color?: string; is_system?: boolean; logo_media_id?: string } = {
			name: formName.trim(),
			slug: formSlug.trim() || slugify(formName)
		};
		if (formDescription.trim()) body.description = formDescription.trim();
		if (formColor.trim()) body.color = formColor.trim();
		if (formLogoMediaId) body.logo_media_id = formLogoMediaId;
		if (!editingTag) body.is_system = formIsSystem;
		const result = editingTag
			? await tagsAdminApi.update(editingTag.id, body)
			: await tagsAdminApi.create(body);
		if (result.ok) {
			showModal = false;
			success = editingTag ? t("tags.updated", "Tag updated") : t("tags.created", "Tag created");
			loadTags();
		} else {
			error = result.error ?? t("common.error", "Failed to save tag");
		}
		saving = false;
	}

	function confirmDelete(tag: Tag) {
		deleteTarget = tag;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		const result = await tagsAdminApi.delete(deleteTarget.id);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTarget = null;
			success = t("tags.deleted", "Tag deleted");
			loadTags();
		} else {
			error = result.error ?? t("common.error", "Failed to delete tag");
		}
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		saving = true;
		try {
			await Promise.all(ids.map((id) => tagsAdminApi.delete(id)));
			selectedIds = new Set();
			loadTags();
		} catch {
			error = t("common.error", "Failed to delete tags");
		} finally {
			saving = false;
		}
	}

	// Tag moderators
	let showModeratorsModal = $state(false);
	let moderatorTag = $state<Tag | null>(null);
	let tagModerators = $state<TagModerator[]>([]);
	let loadingModerators = $state(false);
	let newModeratorUsername = $state("");
	let addingModerator = $state(false);
	let removeModeratorTarget = $state<TagModerator | null>(null);
	let showRemoveModeratorConfirm = $state(false);

	async function openModerators(tag: Tag) {
		moderatorTag = tag;
		showModeratorsModal = true;
		loadingModerators = true;
		newModeratorUsername = "";
		try {
			const result = await tagsAdminApi.getModerators(tag.id);
			if (result.ok) {
				tagModerators = (result.data as TagModerator[]) ?? [];
			} else {
				tagModerators = [];
			}
		} catch {
			tagModerators = [];
		} finally {
			loadingModerators = false;
		}
	}

	async function addTagModerator() {
		if (!moderatorTag || !newModeratorUsername.trim()) return;
		addingModerator = true;
		error = "";
		try {
			const result = await tagsModeratorsAdminApi.add(moderatorTag.id, { username: newModeratorUsername.trim() });
			if (result.ok) {
				newModeratorUsername = "";
				success = t("tags.moderator_added", "Moderator added");
				await openModerators(moderatorTag);
			} else {
				error = result.error ?? t("common.error", "Failed to add moderator");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to add moderator");
		} finally {
			addingModerator = false;
		}
	}

	async function confirmRemoveModerator() {
		if (!moderatorTag || !removeModeratorTarget) return;
		const result = await tagsModeratorsAdminApi.remove(moderatorTag.id, removeModeratorTarget.user_id);
		if (result.ok) {
			showRemoveModeratorConfirm = false;
			removeModeratorTarget = null;
			success = t("tags.moderator_removed", "Moderator removed");
			await openModerators(moderatorTag);
		} else {
			error = result.error ?? t("common.error", "Failed to remove moderator");
		}
	}

	function handleLogoSelect(media: { id: string; url: string; filename: string }) {
		formLogoMediaId = media.id;
		formLogoUrl = media.url;
	}

	function clearLogo() {
		formLogoMediaId = "";
		formLogoUrl = "";
	}

	$effect(() => {
		void currentPage;
		void pageSize;
		void searchQuery;
		void filterValues;
		loadTags();
	});
</script>

<svelte:head>
	<title>{t("tags.title", "Tags")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("tags.title", "Tags")}
	description={t("tags.description", "Manage platform tags")}
	actions={[
		...(canManageTags ? [{ label: t("tags.create", "Create Tag"), icon: Plus, onclick: openCreate }] : []),
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadTags }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}
{#if success}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
		{success}
	</div>
{/if}

{#if selectedIds.size > 0}
	<div class="mb-4 flex items-center gap-3 rounded-lg p-3" style="background: var(--color-primary); color: white">
		<span class="text-sm font-medium">{selectedIds.size} selected</span>
		{#if canManageTags}
			<Button variant="ghost" size="sm" onclick={bulkDelete} disabled={saving}>
				<Trash2 class="mr-1 h-3.5 w-3.5" /> Delete Selected
			</Button>
		{/if}
	</div>
{/if}

<Card>
	<CardHeader>
		<FilterBar
			filters={filterDefs}
			onsearch={handleSearch}
			onclear={handleClear}
		/>
	</CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable
					{columns}
					data={tags}
					selectable={canManageTags}
					bind:selectedIds
					urlSync
					showPagination={false}
				/>
			</div>
			<Pagination
				bind:page={currentPage}
				{totalPages}
				{totalItems}
				bind:pageSize
			/>
		{/if}
	</CardContent>
</Card>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t("tags.details", "Tag Details")} size="sm">
	{#if viewTarget}
		<div class="space-y-4">
			{#if viewTarget.logo_url}
				<div class="flex justify-center">
					<img src={viewTarget.logo_url} alt={viewTarget.name} class="h-16 w-16 rounded-xl object-cover border" style="border-color: var(--color-border)" />
				</div>
			{/if}
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tags.name')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tags.slug')}</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.slug}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tags.posts')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.post_count ?? 0).toLocaleString()}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tags.followers')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{(viewTarget.follower_count ?? 0).toLocaleString()}</p>
				</div>
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('tags.created')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
				</div>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('tags.close')}</Button>
	</div>
</Modal>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={editingTag ? t("tags.edit", "Edit Tag") : t("tags.create", "Create Tag")}>
	<div class="space-y-4">
		<div>
			<label for="tag-name" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("tags.name", "Name")}</label>
			<input id="tag-name" type="text" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="e.g. javascript" bind:value={formName} oninput={() => { if (!editingTag) formSlug = slugify(formName); }} />
		</div>
		<div>
			<label for="tag-slug" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("tags.slug", "Slug")}</label>
			<input id="tag-slug" type="text" class="mt-1 w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="javascript" bind:value={formSlug} />
		</div>
		<div>
			<label for="tag-description" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("tags.description", "Description")}</label>
			<textarea id="tag-description" rows="2" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="Optional description..." bind:value={formDescription}></textarea>
		</div>
		<div>
			<label for="tag-color" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("tags.color", "Color")}</label>
			<div class="mt-1 flex items-center gap-2">
				<input id="tag-color" type="color" class="h-9 w-12 cursor-pointer rounded border" style="border-color: var(--color-border); background: var(--color-surface)" bind:value={formColor} />
				<input type="text" class="w-full rounded-lg border p-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="#3b82f6" bind:value={formColor} />
			</div>
		</div>
		<div>
			<span class="block text-sm font-medium" style="color: var(--color-foreground)">{t("tags.logo", "Logo")}</span>
			<div class="mt-1 flex items-center gap-3">
				{#if formLogoUrl}
					<div class="relative">
						<img src={formLogoUrl} alt="Tag logo" class="h-12 w-12 rounded-lg border object-cover" style="border-color: var(--color-border)" />
						<button type="button" onclick={clearLogo} class="absolute -right-1 -top-1 rounded-full p-0.5" style="background: var(--color-danger); color: white" aria-label={t("common.remove", "Remove")}>
							<X class="h-3 w-3" />
						</button>
					</div>
				{:else}
					<div class="grid h-12 w-12 place-items-center rounded-lg border" style="border-color: var(--color-border); background: var(--color-surface)">
						<ImageIcon class="h-5 w-5" style="color: var(--color-muted)" />
					</div>
				{/if}
				<Button type="button" variant="outline" size="sm" onclick={() => (showMediaPicker = true)}>
					<ImageIcon class="mr-1 h-4 w-4" />
					{t("tags.choose_logo", "Choose Logo")}
				</Button>
			</div>
		</div>
		{#if !editingTag}
			<div class="flex items-center gap-2">
				<input id="tag-is-system" type="checkbox" class="h-4 w-4 rounded" bind:checked={formIsSystem} />
				<label for="tag-is-system" class="text-sm font-medium" style="color: var(--color-foreground)">{t("tags.is_system", "System tag (cannot be deleted)")}</label>
			</div>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submit} disabled={saving}>{saving ? t("common.saving", "Saving...") : t("common.save", "Save")}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("tags.delete_confirm", "Delete tag")} <strong>#{deleteTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t("common.cannotUndo", "This action cannot be undone.")}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- Tag Moderators Modal -->
<Modal bind:open={showModeratorsModal} title={t("tags.moderators", "Tag Moderators")} size="lg">
	{#if moderatorTag}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("tags.tag", "Tag")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">#{moderatorTag.name}</p>
			</div>

			{#if canManageTags}
				<div class="flex gap-2">
					<div class="flex-1">
						<UserAutocomplete bind:value={newModeratorUsername} placeholder={t("tags.moderator_username", "Username")} />
					</div>
					<Button onclick={addTagModerator} disabled={addingModerator || !newModeratorUsername.trim()}>
						{addingModerator ? t("common.adding", "Adding...") : t("common.add", "Add")}
					</Button>
				</div>
			{/if}

			{#if loadingModerators}
				<TableSkeleton rows={3} columns={2} />
			{:else if tagModerators.length === 0}
				<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
					{t("tags.no_moderators", "No moderators assigned")}
				</div>
			{:else}
				<div class="space-y-2">
					{#each tagModerators as mod (mod.user_id)}
						<div class="flex items-center justify-between rounded-lg border p-3" style="border-color: var(--color-border)">
							<span class="text-sm font-medium" style="color: var(--color-foreground)">{mod.username || '—'}</span>
							{#if canManageTags}
								<Button variant="ghost" size="sm" onclick={() => { removeModeratorTarget = mod; showRemoveModeratorConfirm = true; }} style="color: var(--color-danger)">
									<Trash2 class="h-3.5 w-3.5" />
								</Button>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showModeratorsModal = false)}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<!-- Remove Tag Moderator Confirm -->
<Modal bind:open={showRemoveModeratorConfirm} title={t("common.confirmDelete", "Confirm Remove")}>
	{#if removeModeratorTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("tags.remove_moderator_confirm", "Remove")} <strong>{removeModeratorTarget.username}</strong> {t("tags.as_moderator", "as moderator?")}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showRemoveModeratorConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmRemoveModerator}>{t("common.remove", "Remove")}</Button>
	</div>
</Modal>

<!-- TagActionsCell component -->
{#snippet TagActionsCell({ value, row }: { value: any; row: Tag })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageTags}
			<Button variant="ghost" size="sm" onclick={() => openModerators(row)} style="color: var(--color-primary)">
				<Shield class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

<!-- TagLogoCell component — shows the tag logo thumbnail or a hash placeholder -->
{#snippet TagLogoCell({ row }: { row: Tag })}
	{#if row.logo_url}
		<img src={row.logo_url} alt={row.name} class="h-8 w-8 rounded-lg object-cover" loading="lazy" />
	{:else}
		<div class="grid h-8 w-8 place-items-center rounded-lg" style="background: var(--color-surface); color: var(--color-muted)">
			<span class="text-xs font-semibold">#</span>
		</div>
	{/if}
{/snippet}

<!-- Media Picker Modal for logo selection -->
<MediaPickerModal bind:open={showMediaPicker} onselect={handleLogoSelect} />
