<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Button } from "$lib/components/ui/button";
	import Autocomplete, { type AutocompleteItem } from "$lib/components/ui/Autocomplete.svelte";
	import { ArrowLeft, Save, RefreshCw, Lock } from "lucide-svelte";
	import { permissionPresetsApi, adminCatalogApi, type CatalogOption, type PermissionPreset } from "$lib/api";
	import { goto } from "$app/navigation";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";

	let presets = $state<PermissionPreset[]>([]);
	let loading = $state(true);
	let error = $state("");
	let saving = $state<string>("");
	let successMsg = $state("");

	// Editable copies keyed by preset name so we don't mutate the source
	// until the user saves.
	let drafts = $state<Record<string, PermissionPreset>>({});

	const canManage = $derived(hasPermission("spaces.admin"));

	// Axis option sets come from the admin catalogs — same values the
	// backend validates against (spaces.go ValidPermissionAxes).
	let visibilityOptions = $state<CatalogOption[]>([]);
	let postingOptions = $state<CatalogOption[]>([]);
	let joiningOptions = $state<CatalogOption[]>([]);

	// permLabel resolves a permission axis value to a human-readable i18n label.
	function permLabel(opt: string): string {
		return t(`spaces.perm.${opt}`, opt.charAt(0).toUpperCase() + opt.slice(1));
	}

	const axisItems = (opts: CatalogOption[], q: string): AutocompleteItem[] =>
		opts
			.filter((o) => permLabel(o.value).toLowerCase().includes(q.toLowerCase()) || o.value.includes(q.toLowerCase()))
			.map((o) => ({ id: o.value, label: permLabel(o.value) }));

	async function loadAxisOptions() {
		const [vis, post, join] = await Promise.all([
			adminCatalogApi.get("spaces.visibility"),
			adminCatalogApi.get("spaces.posting_perm"),
			adminCatalogApi.get("spaces.joining_perm")
		]);
		if (vis.ok && vis.data?.length) visibilityOptions = vis.data;
		if (post.ok && post.data?.length) postingOptions = post.data;
		if (join.ok && join.data?.length) joiningOptions = join.data;
	}

	async function loadPresets() {
		loading = true;
		error = "";
		try {
			const result = await permissionPresetsApi.list();
			if (result.ok && Array.isArray(result.data)) {
				presets = result.data;
				drafts = {};
				for (const p of presets) {
					drafts[p.name] = { ...p };
				}
			} else if (!result.ok) {
				error = result.error ?? t("common.error", "Failed to load presets");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load presets");
		} finally {
			loading = false;
		}
	}

	async function savePreset(name: string) {
		const draft = drafts[name];
		if (!draft) return;
		saving = name;
		successMsg = "";
		error = "";
		try {
			const result = await permissionPresetsApi.update(name, {
				visibility: draft.visibility,
				posting_perm: draft.posting_perm,
				replying_perm: draft.replying_perm,
				commenting_perm: draft.commenting_perm,
				joining_perm: draft.joining_perm
			});
			if (result.ok) {
				successMsg = t("spaces.preset_saved", "Preset saved");
				await loadPresets();
			} else {
				error = result.error ?? t("common.error", "Failed to save preset");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to save preset");
		} finally {
			saving = "";
		}
	}

	$effect(() => {
		loadPresets();
		loadAxisOptions();
	});
</script>

<!-- One axis picker — renders Autocomplete when editable, read-only text
     for viewers without spaces.admin. -->
{#snippet AxisField({ draft, key, label, options }: { draft: PermissionPreset; key: "visibility" | "posting_perm" | "replying_perm" | "commenting_perm" | "joining_perm"; label: string; options: CatalogOption[] })}
	<div>
		<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted-foreground)">{label}</span>
		{#if canManage}
			<Autocomplete
				id={`axis-${draft.name}-${key}`}
				placeholder={label}
				emptyMessage={t("common.no_data", "No options")}
				fetcher={async (q: string) => axisItems(options, q)}
				initialFetcher={async () => axisItems(options, "")}
				selectedId={draft[key] || null}
				selectedLabel={permLabel(draft[key])}
				onSelect={(item: AutocompleteItem) => { draft[key] = item.id; }}
			/>
		{:else}
			<p class="rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); color: var(--color-foreground)">{permLabel(draft[key])}</p>
		{/if}
	</div>
{/snippet}

<svelte:head>
	<title>{t("spaces.presets_title", "Permission Presets")} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t("spaces.presets_title", "Permission Presets")}
	description={t("spaces.presets_desc", "Manage the default permission presets applied to new spaces")}
	actions={[
		{ label: t("common.back", "Back"), icon: ArrowLeft, variant: "ghost", onclick: () => goto("/spaces") },
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadPresets }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}

{#if successMsg}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)">
		{successMsg}
	</div>
{/if}

{#if loading}
	<Card>
		<CardContent>
			<p class="py-8 text-center text-sm" style="color: var(--color-muted-foreground)">
				{t("common.loading", "Loading...")}
			</p>
		</CardContent>
	</Card>
{:else}
	<div class="space-y-4">
		{#each presets as p (p.id)}
			{@const draft = drafts[p.name]}
			<Card>
				<CardHeader>
					<div class="flex items-center justify-between gap-2 flex-wrap">
						<h3 class="text-lg font-medium" style="color: var(--color-foreground)">
							{p.name}
							{#if p.is_system}
								<span class="ms-2 inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-normal" style="background: var(--color-muted); color: var(--color-muted-foreground)">
									<Lock class="h-3 w-3" />
									{t("spaces.preset_system", "System")}
								</span>
							{/if}
						</h3>
						{#if canManage}
							<Button
								variant="default"
								size="sm"
								disabled={saving === p.name}
								onclick={() => savePreset(p.name)}
							>
								<Save class="me-1 h-4 w-4" />
								{saving === p.name ? t("common.saving", "Saving...") : t("common.save", "Save")}
							</Button>
						{/if}
					</div>
				</CardHeader>
				<CardContent>
					{#if draft}
						<div class="grid grid-cols-1 gap-4 md:grid-cols-5">
							{@render AxisField({ draft, key: "visibility", label: t("spaces.visibility", "Visibility"), options: visibilityOptions })}
							{@render AxisField({ draft, key: "posting_perm", label: t("spaces.posting", "Posting"), options: postingOptions })}
							{@render AxisField({ draft, key: "replying_perm", label: t("spaces.replying", "Replying"), options: postingOptions })}
							{@render AxisField({ draft, key: "commenting_perm", label: t("spaces.commenting", "Commenting"), options: postingOptions })}
							{@render AxisField({ draft, key: "joining_perm", label: t("spaces.joining", "Joining"), options: joiningOptions })}
						</div>
					{/if}
				</CardContent>
			</Card>
		{:else}
			<Card>
				<CardContent>
					<p class="py-8 text-center text-sm" style="color: var(--color-muted-foreground)">
						{t("spaces.presets_empty", "No permission presets found.")}
					</p>
				</CardContent>
			</Card>
		{/each}
	</div>
{/if}
