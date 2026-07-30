<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { Button } from "$lib/components/ui/button";
	import { ArrowLeft, Save, RefreshCw, Lock } from "lucide-svelte";
	import { permissionPresetsApi, type PermissionPreset } from "$lib/api";
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

	const visibilityOptions = ["public", "members", "private"];
	const postingOptions = ["open", "members", "verified", "moderators", "admins"];
	const joiningOptions = ["open", "request", "invite"];

	async function loadPresets() {
		loading = true;
		error = "";
		try {
			const result = await permissionPresetsApi.list();
			if (result.ok && result.data?.data) {
				presets = result.data.data;
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
	});
</script>

<svelte:head>
	<title>{t("spaces.presets_title", "Permission Presets")} — JapaConnect Admin</title>
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
					<div class="flex items-center justify-between">
						<h3 class="text-lg font-medium" style="color: var(--color-foreground)">
							{p.name}
							{#if p.is_system}
								<span class="ml-2 inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-normal" style="background: var(--color-muted); color: var(--color-muted-foreground)">
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
								<Save class="mr-1 h-4 w-4" />
								{saving === p.name ? t("common.saving", "Saving...") : t("common.save", "Save")}
							</Button>
						{/if}
					</div>
				</CardHeader>
				<CardContent>
					{#if draft}
						<div class="grid grid-cols-1 gap-4 md:grid-cols-5">
							<div>
								<label class="mb-1 block text-xs font-medium" style="color: var(--color-muted-foreground)">
									{t("spaces.visibility", "Visibility")}
								
							<select
									class="w-full rounded-lg border px-2 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={draft.visibility}
									disabled={!canManage}
								>
									{#each visibilityOptions as opt}
										<option value={opt}>{opt}</option>
									{/each}
								</select></label>
							</div>
							<div>
								<label class="mb-1 block text-xs font-medium" style="color: var(--color-muted-foreground)">
									{t("spaces.posting", "Posting")}
								
							<select
									class="w-full rounded-lg border px-2 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={draft.posting_perm}
									disabled={!canManage}
								>
									{#each postingOptions as opt}
										<option value={opt}>{opt}</option>
									{/each}
								</select></label>
							</div>
							<div>
								<label class="mb-1 block text-xs font-medium" style="color: var(--color-muted-foreground)">
									{t("spaces.replying", "Replying")}
								
							<select
									class="w-full rounded-lg border px-2 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={draft.replying_perm}
									disabled={!canManage}
								>
									{#each postingOptions as opt}
										<option value={opt}>{opt}</option>
									{/each}
								</select></label>
							</div>
							<div>
								<label class="mb-1 block text-xs font-medium" style="color: var(--color-muted-foreground)">
									{t("spaces.commenting", "Commenting")}
								
							<select
									class="w-full rounded-lg border px-2 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={draft.commenting_perm}
									disabled={!canManage}
								>
									{#each postingOptions as opt}
										<option value={opt}>{opt}</option>
									{/each}
								</select></label>
							</div>
							<div>
								<label class="mb-1 block text-xs font-medium" style="color: var(--color-muted-foreground)">
									{t("spaces.joining", "Joining")}
								
							<select
									class="w-full rounded-lg border px-2 py-1.5 text-sm"
									style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
									bind:value={draft.joining_perm}
									disabled={!canManage}
								>
									{#each joiningOptions as opt}
										<option value={opt}>{opt}</option>
									{/each}
								</select></label>
							</div>
						</div>
					{/if}
				</CardContent>
			</Card>
		{/each}
	</div>
{/if}
