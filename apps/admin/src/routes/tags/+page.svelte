<script lang="ts">
	// /tags — migrated to the EntityPage scaffold (Task 9).
	//
	// The rich tag form (logo/cover media pickers, custom field definitions) and
	// the tag-moderators sub-flow cannot be expressed by the basic EntityForm, so
	// they are rendered via the `formSnippet` escape hatch and page-local state.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from "$lib/components/entity";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import UserAutocomplete from "$lib/components/ui/UserAutocomplete.svelte";
	import MediaPickerModal from "$lib/components/ui/MediaPickerModal.svelte";
	import { tagsAdminApi, tagsModeratorsAdminApi, api } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Shield, Trash2, Image as ImageIcon, X } from "lucide-svelte";

	type Tag = {
		id: string;
		name: string;
		slug: string;
		description?: string;
		rules?: string;
		color?: string;
		logo_url?: string;
		logo_media_id?: string;
		cover_url?: string;
		cover_media_id?: string;
		field_values?: Record<string, unknown>;
		post_count: number;
		follower_count: number;
		is_system: boolean;
		created_at: string;
	};

	type TagFieldDef = {
		id: string;
		key: string;
		label: string;
		field_type: string;
		placeholder?: string;
		is_required: boolean;
	};

	type TagModerator = {
		user_id: string;
		username: string;
		created_at: string;
	};

	const canManageTags = $derived(hasPermission("tags.manage"));

	// mapTag normalizes the API response: content_count → post_count for display.
	function mapTag(raw: any): Tag {
		return {
			...raw,
			post_count: raw.content_count ?? raw.post_count ?? 0,
			follower_count: raw.follower_count ?? 0,
			is_system: raw.is_system ?? false,
			logo_url: raw.logo_url ?? "",
			logo_media_id: raw.logo_media_id ?? "",
			cover_url: raw.cover_url ?? "",
			cover_media_id: raw.cover_media_id ?? "",
			rules: raw.rules ?? "",
			field_values: raw.field_values ?? {},
		};
	}

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
	}

	// Tag custom field definitions — fetched from /custom-field-groups?entity_class=tag
	// so the edit form can render the spec-defined fields without hardcoding them.
	let tagFieldDefs = $state<TagFieldDef[]>([]);

	async function loadTagFieldDefs() {
		try {
			const res = await api<{ id: string; fields?: TagFieldDef[] }[]>(
				"/custom-field-groups?entity_class=tag"
			);
			if (!res.ok) return;
			const fields: TagFieldDef[] = [];
			for (const g of res.data ?? []) {
				for (const f of g.fields ?? []) {
					fields.push({
						id: f.id,
						key: f.key,
						label: f.label,
						field_type: f.field_type,
						placeholder: f.placeholder,
						is_required: f.is_required,
					});
				}
			}
			tagFieldDefs = fields;
		} catch {
			tagFieldDefs = [];
		}
	}

	$effect(() => {
		void loadTagFieldDefs();
	});

	// ── Tag moderators sub-flow (page-local SlideOver) ───────────────────────
	let showModeratorsModal = $state(false);
	let moderatorTag = $state<Tag | null>(null);
	let tagModerators = $state<TagModerator[]>([]);
	let loadingModerators = $state(false);
	let newModeratorUsername = $state("");
	let addingModerator = $state(false);
	let moderatorMessage = $state("");
	let removeModeratorTarget = $state<TagModerator | null>(null);
	let showRemoveModeratorConfirm = $state(false);

	async function openModerators(tag: Tag) {
		moderatorTag = tag;
		showModeratorsModal = true;
		loadingModerators = true;
		newModeratorUsername = "";
		moderatorMessage = "";
		try {
			const result = await tagsAdminApi.getModerators(tag.id);
			tagModerators = result.ok ? ((result.data as TagModerator[]) ?? []) : [];
		} catch {
			tagModerators = [];
		} finally {
			loadingModerators = false;
		}
	}

	async function addTagModerator() {
		if (!moderatorTag || !newModeratorUsername.trim()) return;
		addingModerator = true;
		moderatorMessage = "";
		try {
			const result = await tagsModeratorsAdminApi.add(moderatorTag.id, {
				username: newModeratorUsername.trim(),
			});
			if (result.ok) {
				newModeratorUsername = "";
				moderatorMessage = t("tags.moderator_added", "Moderator added");
				await openModerators(moderatorTag);
			} else {
				moderatorMessage = result.error ?? t("common.error", "Failed to add moderator");
			}
		} catch (e) {
			moderatorMessage = e instanceof Error ? e.message : t("common.error", "Failed to add moderator");
		} finally {
			addingModerator = false;
		}
	}

	async function confirmRemoveModerator() {
		if (!moderatorTag || !removeModeratorTarget) return;
		const result = await tagsModeratorsAdminApi.remove(
			moderatorTag.id,
			removeModeratorTarget.user_id
		);
		if (result.ok) {
			showRemoveModeratorConfirm = false;
			removeModeratorTarget = null;
			moderatorMessage = t("tags.moderator_removed", "Moderator removed");
			await openModerators(moderatorTag);
		} else {
			moderatorMessage = result.error ?? t("common.error", "Failed to remove moderator");
		}
	}

	// ── Bulk delete ─────────────────────────────────────────────────────────
	// EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state("");
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return;
		bulkDeleting = true;
		pageError = "";
		try {
			await Promise.all(idArr.map((id) => tagsAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t("common.error", "Failed to delete tags");
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Form body normalization (mirrors the legacy submit() logic) ──────────
	function normalizeBody(formValue: Record<string, any>, isUpdate: boolean): Record<string, unknown> {
		const body: Record<string, unknown> = {
			name: String(formValue.name ?? "").trim(),
			slug: String(formValue.slug ?? "").trim() || slugify(String(formValue.name ?? "")),
		};
		const desc = String(formValue.description ?? "").trim();
		if (desc) body.description = desc;
		const rules = String(formValue.rules ?? "").trim();
		if (rules) body.rules = rules;
		const color = String(formValue.color ?? "").trim();
		if (color) body.color = color;
		if (formValue.logo_media_id) body.logo_media_id = formValue.logo_media_id;
		if (formValue.cover_media_id) body.cover_media_id = formValue.cover_media_id;
		if (!isUpdate) body.is_system = !!formValue.is_system;
		if (tagFieldDefs.length > 0) {
			const fv: Record<string, string> = {};
			const raw =
				formValue.field_values && typeof formValue.field_values === "object"
					? formValue.field_values
					: {};
			for (const def of tagFieldDefs) {
				const v = String(raw[def.key] ?? "").trim();
				if (v) fv[def.key] = v;
			}
			body.field_values = fv;
		}
		return body;
	}

	// `fields` is used by EntityPage to seed the form value on edit; the actual
	// inputs are rendered by the `formSnippet` below (the rich tag form).
	const fields: FieldDef[] = [
		{ key: "name", label: t("tags.name", "Name"), type: "text" },
		{ key: "slug", label: t("tags.slug", "Slug"), type: "text" },
		{ key: "description", label: t("tags.description", "Description"), type: "text" },
		{ key: "rules", label: t("tags.rules", "Rules"), type: "text" },
		{ key: "color", label: t("tags.color", "Color"), type: "text" },
		{ key: "logo_media_id", label: "Logo Media ID", type: "text" },
		{ key: "logo_url", label: "Logo URL", type: "text" },
		{ key: "cover_media_id", label: "Cover Media ID", type: "text" },
		{ key: "cover_url", label: "Cover URL", type: "text" },
		{ key: "is_system", label: t("tags.is_system", "System tag"), type: "checkbox" },
		{ key: "field_values", label: "Field Values", type: "text" },
	];

	const config: EntityPageConfig = {
		title: t("tags.title", "Tags"),
		description: t("tags.description", "Manage platform tags"),
		createLabel: t("tags.create", "Create Tag"),
		createPermission: "tags.manage",
		pageSize: 25,
		columns: [
			{ key: "logo_url", label: t("tags.logo", "Logo"), class: "w-16", componentSnippet: TagLogoCell },
			{ key: "name", label: t("tags.name", "Name"), sortable: true },
			{ key: "slug", label: t("tags.slug", "Slug"), sortable: true },
			{
				key: "post_count",
				label: t("tags.posts", "Posts"),
				sortable: true,
				render: (v: number) => (v ?? 0).toLocaleString(),
			},
			{
				key: "follower_count",
				label: t("tags.followers", "Followers"),
				sortable: true,
				render: (v: number) => (v ?? 0).toLocaleString(),
			},
			{
				key: "created_at",
				label: t("common.created", "Created"),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString(),
			},
			{ key: "moderators", label: "", class: "w-12", componentSnippet: ModeratorsCell },
		],
		filters: [
			{
				key: "post_count",
				label: t("tags.min_posts", "Min Posts"),
				options: [
					{ value: "0", label: "Any" },
					{ value: "10", label: "10+" },
					{ value: "100", label: "100+" },
					{ value: "1000", label: "1000+" },
				],
			},
		],
		bulkActions: [
			{
				label: t("common.deleteSelected", "Delete Selected"),
				icon: Trash2,
				variant: "destructive",
				onclick: bulkDelete,
			},
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page: String(page),
				limit: String(pageSize),
			});
			if (q) params.set("q", q);
			if (filter.post_count) params.set("min_content_count", filter.post_count);
			// EntityPage emits sort as "key:dir"; the legacy API expects the bare key.
			const sortKey = sort ? sort.split(":")[0] : "";
			if (sortKey) params.set("sort", sortKey);
			const result = await tagsAdminApi.list(params.toString());
			if (result.ok) {
				const d = result.data as
					| { data: Tag[]; total: number; total_pages: number }
					| Tag[];
				if (Array.isArray(d)) {
					return { data: d.map(mapTag), total: d.length, totalPages: 1 };
				} else if (d.data) {
					return {
						data: d.data.map(mapTag),
						total: d.total ?? d.data.length,
						totalPages: d.total_pages ?? 1,
					};
				}
				return { data: [], total: 0, totalPages: 1 };
			}
			throw new Error(result.error ?? t("common.error", "Failed to load tags"));
		},
		create: async (body) => {
			const res = await tagsAdminApi.create(normalizeBody(body, false) as any);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await tagsAdminApi.update(id, normalizeBody(body, true) as any);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await tagsAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: TagView,
		formSnippet: TagForm,
	};
</script>

<svelte:head>
	<title>{t("tags.title", "Tags")} — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Tag Moderators SlideOver (page-local sub-flow) -->
<SlideOver bind:open={showModeratorsModal} title={t("tags.moderators", "Tag Moderators")} size="lg">
	{#if moderatorTag}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t("tags.tag", "Tag")}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					#{moderatorTag.name}
				</p>
			</div>

			{#if moderatorMessage}
				<div
					class="rounded-lg p-2 text-sm"
					style="background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)"
				>
					{moderatorMessage}
				</div>
			{/if}

			{#if canManageTags}
				<div class="flex gap-2">
					<div class="flex-1">
						<UserAutocomplete
							bind:value={newModeratorUsername}
							placeholder={t("tags.moderator_username", "Username")}
						/>
					</div>
					<Button
						onclick={addTagModerator}
						disabled={addingModerator || !newModeratorUsername.trim()}
					>
						{addingModerator ? t("common.add", "Adding...") : t("common.add", "Add")}
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
						<div
							class="flex items-center justify-between rounded-lg border p-3"
							style="border-color: var(--color-border)"
						>
							<span class="text-sm font-medium" style="color: var(--color-foreground)">
								{mod.username || "—"}
							</span>
							{#if canManageTags}
								<Button
									variant="ghost"
									size="sm"
									onclick={() => {
										removeModeratorTarget = mod;
										showRemoveModeratorConfirm = true;
									}}
									style="color: var(--color-danger)"
								>
									<Trash2 class="h-3.5 w-3.5" />
								</Button>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		<Button variant="outline" onclick={() => (showModeratorsModal = false)}>
			{t("common.close", "Close")}
		</Button>
	{/snippet}
</SlideOver>

<!-- Remove Tag Moderator Confirm -->
<Modal bind:open={showRemoveModeratorConfirm} title={t("common.confirmDelete", "Confirm Remove")}>
	{#if removeModeratorTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("tags.remove_moderator_confirm", "Remove")} <strong>{removeModeratorTarget.username}</strong>
			{t("tags.as_moderator", "as moderator?")}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRemoveModeratorConfirm = false)}>
			{t("common.cancel", "Cancel")}
		</Button>
		<Button variant="destructive" onclick={confirmRemoveModerator}>
			{t("common.remove", "Remove")}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- Tag logo thumbnail or hash placeholder -->
{#snippet TagLogoCell({ row }: { row: Tag })}
	{#if row.logo_url}
		<img src={row.logo_url} alt={row.name} class="h-8 w-8 rounded-lg object-cover" loading="lazy" />
	{:else}
		<div
			class="grid h-8 w-8 place-items-center rounded-lg"
			style="background: var(--color-surface); color: var(--color-muted)"
		>
			<span class="text-xs font-semibold">#</span>
		</div>
	{/if}
{/snippet}

<!-- Manage-moderators row action (kept as a column because EntityPage does not
     expose its internal openView/openEdit/openDelete, so a custom `actions`
     array cannot trigger the scaffold's built-in slide-overs) -->
{#snippet ModeratorsCell({ row }: { row: Tag })}
	{#if canManageTags}
		<Button
			variant="ghost"
			size="sm"
			onclick={() => openModerators(row)}
			style="color: var(--color-primary)"
			aria-label={t("tags.moderators", "Tag Moderators")}
		>
			<Shield class="h-3.5 w-3.5" />
		</Button>
	{/if}
{/snippet}

<!-- View slide-over body -->
{#snippet TagView({ row }: { row: Tag })}
	<div class="space-y-4">
		{#if row.cover_url}
			<div class="overflow-hidden rounded-lg border" style="border-color: var(--color-border)">
				<img src={row.cover_url} alt={row.name} class="h-24 w-full object-cover" />
			</div>
		{/if}
		{#if row.logo_url}
			<div class="flex justify-center">
				<img
					src={row.logo_url}
					alt={row.name}
					class="h-16 w-16 rounded-xl object-cover border"
					style="border-color: var(--color-border)"
				/>
			</div>
		{/if}
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t("tags.name", "Name")}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t("tags.slug", "Slug")}
				</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t("tags.posts", "Posts")}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{(row.post_count ?? 0).toLocaleString()}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t("tags.followers", "Followers")}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{(row.follower_count ?? 0).toLocaleString()}
				</p>
			</div>
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t("common.created", "Created")}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{new Date(row.created_at).toLocaleString()}
				</p>
			</div>
			{#if row.description}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("tags.description", "Description")}
					</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
				</div>
			{/if}
			{#if row.rules}
				<div class="col-span-2">
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t("tags.rules", "Rules")}
					</p>
					<p class="mt-1 whitespace-pre-line text-sm" style="color: var(--color-foreground)">
						{row.rules}
					</p>
				</div>
			{/if}
		</div>
	</div>
{/snippet}

<!-- Create/Edit form body (rich form: media pickers + custom fields) -->
{#snippet TagForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-4">
		<div>
			<label for="tag-name" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t("tags.name", "Name")}
			</label>
			<input
				id="tag-name"
				type="text"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="e.g. javascript"
				bind:value={value.name}
				oninput={() => {
					if (!editing) value.slug = slugify(String(value.name ?? ""));
				}}
			/>
		</div>
		<div>
			<label for="tag-slug" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t("tags.slug", "Slug")}
			</label>
			<input
				id="tag-slug"
				type="text"
				class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="javascript"
				bind:value={value.slug}
			/>
		</div>
		<div>
			<label
				for="tag-description"
				class="block text-sm font-medium"
				style="color: var(--color-foreground)"
			>
				{t("tags.description", "Description")}
			</label>
			<textarea
				id="tag-description"
				rows="2"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="Optional description..."
				bind:value={value.description}
			></textarea>
		</div>
		<div>
			<label for="tag-rules" class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t("tags.rules", "Rules")}
			</label>
			<textarea
				id="tag-rules"
				rows="3"
				class="mt-1 w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				placeholder="Optional community rules/guidelines for this tag..."
				bind:value={value.rules}
			></textarea>
		</div>
		<div>
			<span class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t("tags.color", "Color")}
			</span>
			<div class="mt-1 flex items-center gap-2">
				<input
					id="tag-color"
					type="color"
					class="h-9 w-12 cursor-pointer rounded border"
					style="border-color: var(--color-border); background: var(--color-surface)"
					bind:value={value.color}
				/>
				<input
					type="text"
					class="w-full rounded-lg border p-2 text-sm font-mono"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					placeholder="#3b82f6"
					bind:value={value.color}
				/>
			</div>
		</div>

		<!-- Cover image -->
		<div>
			<span class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t("tags.cover", "Cover Image")}
			</span>
			<div class="mt-1 flex items-center gap-3">
				{#if value.cover_url}
					<div class="relative">
						<img
							src={value.cover_url}
							alt="Tag cover"
							class="h-12 w-20 rounded-lg border object-cover"
							style="border-color: var(--color-border)"
						/>
						<button
							type="button"
							onclick={() => {
								value.cover_media_id = "";
								value.cover_url = "";
							}}
							class="absolute -right-1 -top-1 rounded-full p-0.5"
							style="background: var(--color-danger); color: white"
							aria-label={t("common.remove", "Remove")}
						>
							<X class="h-3 w-3" />
						</button>
					</div>
				{:else}
					<div
						class="grid h-12 w-20 place-items-center rounded-lg border"
						style="border-color: var(--color-border); background: var(--color-surface)"
					>
						<ImageIcon class="h-5 w-5" style="color: var(--color-muted)" />
					</div>
				{/if}
				<Button type="button" variant="outline" size="sm" onclick={() => (value._coverPickerOpen = true)}>
					<ImageIcon class="mr-1 h-4 w-4" />
					{t("tags.choose_cover", "Choose Cover")}
				</Button>
			</div>
		</div>

		<!-- Logo -->
		<div>
			<span class="block text-sm font-medium" style="color: var(--color-foreground)">
				{t("tags.logo", "Logo")}
			</span>
			<div class="mt-1 flex items-center gap-3">
				{#if value.logo_url}
					<div class="relative">
						<img
							src={value.logo_url}
							alt="Tag logo"
							class="h-12 w-12 rounded-lg border object-cover"
							style="border-color: var(--color-border)"
						/>
						<button
							type="button"
							onclick={() => {
								value.logo_media_id = "";
								value.logo_url = "";
							}}
							class="absolute -right-1 -top-1 rounded-full p-0.5"
							style="background: var(--color-danger); color: white"
							aria-label={t("common.remove", "Remove")}
						>
							<X class="h-3 w-3" />
						</button>
					</div>
				{:else}
					<div
						class="grid h-12 w-12 place-items-center rounded-lg border"
						style="border-color: var(--color-border); background: var(--color-surface)"
					>
						<ImageIcon class="h-5 w-5" style="color: var(--color-muted)" />
					</div>
				{/if}
				<Button type="button" variant="outline" size="sm" onclick={() => (value._logoPickerOpen = true)}>
					<ImageIcon class="mr-1 h-4 w-4" />
					{t("tags.choose_logo", "Choose Logo")}
				</Button>
			</div>
		</div>

		<!-- Custom field definitions -->
		{#if tagFieldDefs.length > 0}
			{@const fv =
				value.field_values && typeof value.field_values === "object"
					? value.field_values
					: (value.field_values = {})}
			<div class="border-t pt-3" style="border-color: var(--color-border)">
				<p class="mb-2 text-sm font-medium" style="color: var(--color-foreground)">
					{t("tags.custom_fields", "Custom Fields")}
				</p>
				{#each tagFieldDefs as field (field.key)}
					<div class="mb-3">
						<label
							for="tag-field-{field.key}"
							class="block text-xs font-medium"
							style="color: var(--color-muted)"
						>
							{field.label}{#if field.is_required} <span style="color: var(--color-danger)">*</span>{/if}
						</label>
						{#if field.field_type === "rich_text" || field.field_type === "textarea"}
							<textarea
								id="tag-field-{field.key}"
								rows="3"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								placeholder={field.placeholder ?? ""}
								bind:value={fv[field.key]}
							></textarea>
						{:else}
							<input
								id="tag-field-{field.key}"
								type={field.field_type === "url" ? "url" : "text"}
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								placeholder={field.placeholder ?? ""}
								bind:value={fv[field.key]}
							/>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		{#if !editing}
			<div class="flex items-center gap-2">
				<input id="tag-is-system" type="checkbox" class="h-4 w-4 rounded" bind:checked={value.is_system} />
				<label for="tag-is-system" class="text-sm font-medium" style="color: var(--color-foreground)">
					{t("tags.is_system", "System tag (cannot be deleted)")}
				</label>
			</div>
		{/if}
	</div>

	<!-- Media pickers (portal to body; open state lives on the form value so it
	     resets each time the form is reopened) -->
	<MediaPickerModal
		bind:open={value._logoPickerOpen}
		onselect={(media: { id: string; url: string }) => {
			value.logo_media_id = media.id;
			value.logo_url = media.url;
		}}
	/>
	<MediaPickerModal
		bind:open={value._coverPickerOpen}
		onselect={(media: { id: string; url: string }) => {
			value.cover_media_id = media.id;
			value.cover_url = media.url;
		}}
	/>
{/snippet}
