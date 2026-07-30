<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Button } from "$lib/components/ui/button";
	import { ArrowLeft, Save } from "lucide-svelte";
	import { spacesAdminApi, spaceTypesApi, permissionPresetsApi, type PermissionPreset } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";

	let name = $state("");
	let slug = $state("");
	let description = $state("");
	let level = $state("custom");
	let spaceTypeId = $state("");
	let parentId = $state("");
	let presetName = $state("");
	let submitting = $state(false);
	let error = $state("");

	type SpaceType = { id: string; name: string; slug: string };
	let spaceTypes = $state<SpaceType[]>([]);
	let presets = $state<PermissionPreset[]>([]);

	const levels = [
		{ value: "country", label: t("spaces.level_country", "Country") },
		{ value: "state", label: t("spaces.level_state", "State") },
		{ value: "city", label: t("spaces.level_city", "City") },
		{ value: "custom", label: t("spaces.level_custom", "Custom") }
	];

	function generateSlug(s: string): string {
		return s
			.toLowerCase()
			.replace(/[^a-z0-9\s-]/g, "")
			.replace(/\s+/g, "-")
			.replace(/-+/g, "-")
			.replace(/^-|-$/g, "");
	}

	$effect(() => {
		slug = generateSlug(name);
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
			if (result.ok && result.data?.data) {
				presets = result.data.data;
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
				goto("/spaces");
			} else {
				error = result.error ?? t("common.error", "Failed to create space");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to create space");
		} finally {
			submitting = false;
		}
	}

	$effect(() => {
		loadSpaceTypes();
		loadPresets();
	});
</script>

<svelte:head>
	<title>{t("spaces.create", "Create Space")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("spaces.create", "Create Space")}
	description={t("spaces.create_desc", "Create a new platform space")}
	actions={[
		{ label: t("common.back", "Back"), icon: ArrowLeft, variant: "ghost", onclick: () => goto("/spaces") }
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
						placeholder={t("spaces.slug_placeholder", "auto-generated-from-name")}
						required
					/>
				</div>
				<div>
					<label for="space-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.space_type", "Space Type")} <span style="color: var(--color-danger)">*</span>
					</label>
					<select
						id="space-type"
						class="w-full rounded-lg border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={spaceTypeId}
						required
					>
						<option value="">{t("spaces.select_type", "Select a type")}</option>
						{#each spaceTypes as st}
							<option value={st.id}>{st.name}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="level" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.level", "Level")} <span style="color: var(--color-danger)">*</span>
					</label>
					<select
						id="level"
						class="w-full rounded-lg border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={level}
						required
					>
						{#each levels as l}
							<option value={l.value}>{l.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="parent-id" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.parent_id", "Parent Space ID")}
					</label>
					<input
						id="parent-id"
						type="text"
						class="w-full rounded-lg border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={parentId}
						placeholder={t("spaces.parent_placeholder", "Optional UUID")}
					/>
				</div>
				<div>
					<label for="preset-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{t("spaces.preset_name", "Permission Preset")}
					</label>
					<select
						id="preset-name"
						class="w-full rounded-lg border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						bind:value={presetName}
					>
						<option value="">{t("spaces.preset_none", "None (use defaults)")}</option>
						{#each presets as p (p.id)}
							<option value={p.name}>{p.name}</option>
						{/each}
					</select>
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
				<Button variant="ghost" type="button" onclick={() => goto("/spaces")}>{t("common.cancel", "Cancel")}</Button>
				<Button type="submit" disabled={submitting}>
					<Save class="mr-1 h-4 w-4" />
					{submitting ? t("common.creating", "Creating...") : t("spaces.create", "Create Space")}
				</Button>
			</div>
		</form>
	</CardContent>
</Card>
