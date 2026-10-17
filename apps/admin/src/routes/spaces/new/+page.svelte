<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Button } from "$lib/components/ui/button";
	import Autocomplete, { type AutocompleteItem } from "$lib/components/ui/Autocomplete.svelte";
	import { ArrowLeft, Save } from "lucide-svelte";
	import { api, spacesAdminApi, spaceTypesApi, permissionPresetsApi, adminCatalogApi, type CatalogOption, type PermissionPreset } from "$lib/api";
	import { goto } from "$app/navigation";
	import { page } from "$app/state";
	import { t } from "$lib/stores/i18n.svelte";

	// Read level and parent_id from URL query params so the /geo page can
	// deep-link here with a pre-selected level (e.g. /spaces/new?level=state&parent_id=...).
	let name = $state("");
	let slug = $state("");
	let slugEdited = $state(false);
	let description = $state("");
	let level = $state(page.url.searchParams.get("level") ?? "custom");
	let spaceTypeId = $state("");
	let parentId = $state(page.url.searchParams.get("parent_id") ?? "");
	let parentLabel = $state("");
	let presetName = $state("");
	let submitting = $state(false);
	let error = $state("");

	type SpaceType = { id: string; name: string; slug: string };
	let spaceTypes = $state<SpaceType[]>([]);
	let presets = $state<PermissionPreset[]>([]);

	let levels = $state<CatalogOption[]>([
		{ value: "country", label: t("spaces.level_country", "Country") },
		{ value: "state", label: t("spaces.level_state", "State") },
		{ value: "city", label: t("spaces.level_city", "City") },
		{ value: "custom", label: t("spaces.level_custom", "Custom") }
	]);

	// Local-list fetchers — every option set is loaded from the API once and
	// filtered client-side so typing narrows the list (Rule 16: no
	// hardcoded option lists where a catalog exists).
	const toItems = (opts: { value: string; label: string }[]): AutocompleteItem[] =>
		opts.map((o) => ({ id: o.value, label: o.label }));

	const filterItems = (opts: { value: string; label: string }[], q: string): AutocompleteItem[] =>
		toItems(opts.filter((o) => o.label.toLowerCase().includes(q.toLowerCase()) || o.value.toLowerCase().includes(q.toLowerCase())));

	const levelFetcher = async (q: string) => filterItems(levels, q);
	const levelInitial = async () => toItems(levels);

	const typeFetcher = async (q: string) =>
		spaceTypes
			.filter((st) => st.name.toLowerCase().includes(q.toLowerCase()) || st.slug.toLowerCase().includes(q.toLowerCase()))
			.map((st) => ({ id: st.id, label: st.name, sublabel: st.slug }));
	const typeInitial = async () => spaceTypes.map((st) => ({ id: st.id, label: st.name, sublabel: st.slug }));

	const presetFetcher = async (q: string) =>
		presets.filter((p) => p.name.toLowerCase().includes(q.toLowerCase())).map((p) => ({ id: p.name, label: p.name }));
	const presetInitial = async () => presets.map((p) => ({ id: p.name, label: p.name }));

	// Parent space lookup via the dedicated suggest endpoint (primary
	// spaces only — the backend rejects non-primary parents anyway).
	async function parentFetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await api<{ id: string; name: string; slug: string }[]>(
			`/spaces/suggest-parents?q=${encodeURIComponent(q)}`
		);
		if (!res.ok || !Array.isArray(res.data)) return [];
		return res.data.map((s) => ({ id: s.id, label: s.name, sublabel: s.slug }));
	}

	function generateSlug(s: string): string {
		return s
			.toLowerCase()
			.replace(/[^a-z0-9\s-]/g, "")
			.replace(/\s+/g, "-")
			.replace(/-+/g, "-")
			.replace(/^-|-$/g, "");
	}

	$effect(() => {
		// Auto-slug until the user edits the field manually — after that,
		// keep their value even if the name changes again.
		if (!slugEdited) slug = generateSlug(name);
	});

	async function loadSpaceTypes() {
		try {
			const result = await spaceTypesApi.list();
			if (result.ok) {
				const d = result.data as SpaceType[] | { items?: SpaceType[] };
				spaceTypes = Array.isArray(d) ? d : (d.items ?? []);
			}
		} catch {
			// non-critical
		}
	}

	async function loadPresets() {
		try {
			const result = await permissionPresetsApi.list();
			if (result.ok && Array.isArray(result.data)) {
				presets = result.data;
			}
		} catch {
			// non-critical
		}
	}

	async function handleSubmit() {
		if (!name.trim() || !slug.trim() || !spaceTypeId) {
			error = t("spaces.fill_required", "Name, slug, and space type are required");
			return;
		}
		submitting = true;
		error = "";
		try {
			const result = await spacesAdminApi.create({
				name: name.trim(),
				slug: slug.trim(),
				space_type_id: spaceTypeId,
				level,
				description: description.trim() || undefined,
				parent_id: parentId.trim() || undefined,
				preset_name: presetName.trim() || undefined
			});
			if (result.ok) {
				goto(page.url.searchParams.get("from") ?? "/spaces");
			} else {
				error = result.error ?? t("common.error", "Failed to create space");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to create space");
		} finally {
			submitting = false;
		}
	}

	async function loadLevels() {
		try {
			const res = await adminCatalogApi.get("spaces.level");
			if (res.ok && res.data?.length) levels = res.data;
		} catch {
			// non-critical — fallback levels stay in place
		}
	}

	// Deep-linked ?parent_id arrives as a bare UUID — resolve its display
	// name so the Autocomplete chip doesn't show the raw id.
	async function resolveParentLabel() {
		if (!parentId) return;
		try {
			const res = await spacesAdminApi.get(parentId);
			const d = res.ok ? (res.data as { space?: { name?: string; slug?: string } }) : null;
			parentLabel = d?.space?.name ?? parentId;
		} catch {
			parentLabel = parentId;
		}
	}

	$effect(() => {
		loadSpaceTypes();
		loadPresets();
		loadLevels();
		resolveParentLabel();
	});
</script>

<svelte:head>
	<title>{t("spaces.create", "Create Space")} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t("spaces.create", "Create Space")}
	description={t("spaces.create_desc", "Create a new platform space")}
	actions={[
		{ label: t("common.back", "Back"), icon: ArrowLeft, variant: "ghost", onclick: () => goto(page.url.searchParams.get("from") ?? "/spaces") }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}

<Card>
	<CardHeader>
		<h3 class="text-lg font-medium" style="color: var(--color-foreground)">{t("spaces.details", "Space Details")}</h3>
	</CardHeader>
	<CardContent>
		<form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="space-y-4">
			<div class="grid grid-cols-1 gap-4 md:grid-cols-2">
				<div>
					<label for="name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.name", "Name")} <span style="color: var(--color-danger)">*</span>
					</label>
					<input
						id="name"
						type="text"
						class="w-full rounded-lg border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={name}
						placeholder={t("spaces.name_placeholder", "Enter space name")}
						required
					/>
				</div>
				<div>
					<label for="slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.slug", "Slug")} <span style="color: var(--color-danger)">*</span>
					</label>
					<input
						id="slug"
						type="text"
						class="w-full rounded-lg border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={slug}
						oninput={() => (slugEdited = true)}
						placeholder={t("spaces.slug_placeholder", "auto-generated-from-name")}
						required
					/>
				</div>
				<div>
					<label for="space-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.space_type", "Space Type")} <span style="color: var(--color-danger)">*</span>
					</label>
					<Autocomplete
						id="space-type"
						placeholder={t("spaces.select_type", "Select a type")}
						emptyMessage={t("common.no_data", "No matching types")}
						fetcher={typeFetcher}
						initialFetcher={typeInitial}
						selectedId={spaceTypeId || null}
						selectedLabel={spaceTypes.find((st) => st.id === spaceTypeId)?.name ?? ""}
						onSelect={(item: AutocompleteItem) => (spaceTypeId = item.id)}
						onClear={() => (spaceTypeId = "")}
					/>
				</div>
				<div>
					<label for="level" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.level", "Level")} <span style="color: var(--color-danger)">*</span>
					</label>
					<Autocomplete
						id="level"
						placeholder={t("spaces.level", "Level")}
						emptyMessage={t("common.no_data", "No matching levels")}
						fetcher={levelFetcher}
						initialFetcher={levelInitial}
						selectedId={level || null}
						selectedLabel={levels.find((l) => l.value === level)?.label ?? level}
						onSelect={(item: AutocompleteItem) => (level = item.id)}
						onClear={() => (level = "")}
					/>
				</div>
				<div>
					<label for="parent-id" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.parent_id", "Parent Space")}
					</label>
					<Autocomplete
						id="parent-id"
						placeholder={t("spaces.parent_placeholder", "Search primary spaces")}
						emptyMessage={t("common.no_data", "No matching spaces")}
						fetcher={parentFetcher}
						selectedId={parentId || null}
						selectedLabel={parentLabel}
						onSelect={(item: AutocompleteItem) => { parentId = item.id; parentLabel = item.label; }}
						onClear={() => { parentId = ""; parentLabel = ""; }}
					/>
				</div>
				<div>
					<label for="preset-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.preset_name", "Permission Preset")}
					</label>
					<Autocomplete
						id="preset-name"
						placeholder={t("spaces.preset_none", "None (use defaults)")}
						emptyMessage={t("common.no_data", "No matching presets")}
						fetcher={presetFetcher}
						initialFetcher={presetInitial}
						selectedId={presetName || null}
						selectedLabel={presetName}
						onSelect={(item: AutocompleteItem) => (presetName = item.id)}
						onClear={() => (presetName = "")}
					/>
				</div>
			</div>
			<div>
				<label for="description" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
					{t("spaces.description", "Description")}
				</label>
				<textarea
					id="description"
					class="w-full rounded-lg border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					rows="3"
					bind:value={description}
					placeholder={t("spaces.description_placeholder", "Optional description")}
				></textarea>
			</div>
			<div class="flex justify-end gap-2 pt-4">
				<Button variant="ghost" type="button" onclick={() => goto(page.url.searchParams.get("from") ?? "/spaces")}>{t("common.cancel", "Cancel")}</Button>
				<Button type="submit" disabled={submitting}>
					<Save class="mr-1 h-4 w-4" />
					{submitting ? t("common.creating", "Creating...") : t("spaces.create", "Create Space")}
				</Button>
			</div>
		</form>
	</CardContent>
</Card>
