<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent, CardHeader } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { FilterBar } from "$lib/components/ui/filter-bar";
	import { Badge } from "$lib/components/ui/badge";
	import { Button } from "$lib/components/ui/button";
	import { Select } from "$lib/components/ui/select";
	import { Modal } from "$lib/components/ui/modal";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw, Plus, Trash2, Ban, MessageSquareOff, Users, Eye } from "lucide-svelte";
	import { spamApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { onMount } from "svelte";
	import { datetimeLocalToPostgresTimestamptz } from "$lib/utils/date";

	type IpBlock = {
		id: string;
		cidr: string;
		block_type: string;
		reason: string | null;
		expires_at?: string | null;
		created_at?: string;
	};
	type CensoredWord = {
		id: string;
		pattern: string;
		action: string;
		replacement: string | null;
		created_at?: string;
	};
	type UserBlock = {
		blocker_id: string;
		blocked_id: string;
		created_at?: string;
	};

	type Tab = "ip-blocks" | "censored-words" | "user-blocks";

	let activeTab = $state<Tab>("ip-blocks");
	let loading = $state(false);
	let submitting = $state(false);
	let error = $state("");
	let success = $state("");

	// View modal
	let showViewModal = $state(false);
	let viewTarget = $state<{ kind: string; row: any } | null>(null);

	const canManageSpam = $derived(hasPermission("spam.ip_block.manage"));
	
	const blockTypeOptions = $derived([
		{ value: "full", label: t("spam.block_type_full", "Full Block") },
		{ value: "registration", label: t("spam.block_type_registration", "Registration") },
		{ value: "posting", label: t("spam.block_type_posting", "Posting") },
		{ value: "api", label: t("spam.block_type_api", "API") }
	]);
	
	const actionOptions = $derived([
		{ value: "block", label: t("spam.action_block", "Block") },
		{ value: "replace", label: t("spam.action_replace", "Replace") },
		{ value: "flag", label: t("spam.action_flag", "Flag") }
	]);

	// IP Blocks
	let ipBlocks = $state<IpBlock[]>([]);
	let showIpModal = $state(false);
	let ipForm = $state({ cidr: "", block_type: "full", reason: "", expires_at: "" });
	let ipSearch = $state("");

	// Censored Words
	let censoredWords = $state<CensoredWord[]>([]);
	let showCwModal = $state(false);
	let cwForm = $state({ pattern: "", action: "block", replacement: "" });
	let cwSearch = $state("");

	// User Blocks
	let userBlocks = $state<UserBlock[]>([]);
	let ubSearch = $state("");

	const ipBlockFilterDefs = [
		{
			key: "block_type",
			label: t("spam.block_type", "Block Type"),
			options: [
				{ value: "full", label: t("spam.block_type_full", "Full Block") },
				{ value: "registration", label: t("spam.block_type_registration", "Registration") },
				{ value: "posting", label: t("spam.block_type_posting", "Posting") },
				{ value: "api", label: t("spam.block_type_api", "API") }
			]
		}
	];
	const cwFilterDefs = [
		{
			key: "action",
			label: t("spam.action", "Action"),
			options: [
				{ value: "block", label: t("spam.action_block", "Block") },
				{ value: "replace", label: t("spam.action_replace", "Replace") },
				{ value: "flag", label: t("spam.action_flag", "Flag") }
			]
		}
	];

	let ipFilterValues = $state<Record<string, string>>({});
	let cwFilterValues = $state<Record<string, string>>({});

	function handleIpSearch(values: Record<string, string>) {
		ipFilterValues = values;
		loadIpBlocks();
	}
	function handleIpClear() {
		ipFilterValues = {};
		loadIpBlocks();
	}
	function handleCwSearch(values: Record<string, string>) {
		cwFilterValues = values;
		loadCensoredWords();
	}
	function handleCwClear() {
		cwFilterValues = {};
		loadCensoredWords();
	}

	// Delete confirmation
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<{ type: string; id: string; secondaryId?: string } | null>(null);

	const tabs: { value: Tab; label: string; icon: any }[] = [
		{ value: "ip-blocks", label: t("spam.ip_blocks", "IP Blocks"), icon: Ban },
		{ value: "censored-words", label: t("spam.censored_words", "Censored Words"), icon: MessageSquareOff },
		{ value: "user-blocks", label: t("spam.user_blocks", "User Blocks"), icon: Users }
	];

	const ipColumns = [
		{ key: "cidr", label: t("spam.cidr", "IP / CIDR"), sortable: true },
		{ key: "block_type", label: t("spam.block_type", "Type"), sortable: true },
		{ key: "reason", label: t("spam.reason", "Reason"), sortable: true, render: (v: string) => v || "—" },
		{ key: "expires_at", label: t("spam.expires", "Expires"), sortable: true, render: (v: string) => v || t("spam.never", "Never") },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-24", componentSnippet: IpActionsCell }
	];

	const cwColumns = [
		{ key: "pattern", label: t("spam.pattern", "Pattern"), sortable: true },
		{ key: "action", label: t("spam.action", "Action"), sortable: true },
		{ key: "replacement", label: t("spam.replacement", "Replacement"), sortable: true, render: (v: string) => v || "—" },
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => v ? new Date(v).toLocaleDateString() : "—" },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-24", componentSnippet: CwActionsCell }
	];

	const ubColumns = [
		{ key: "blocker_id", label: t("spam.blocker", "Blocker"), sortable: true },
		{ key: "blocked_id", label: t("spam.blocked", "Blocked"), sortable: true },
		{ key: "created_at", label: t("common.created", "Created"), sortable: true, render: (v: string) => v ? new Date(v).toLocaleDateString() : "—" },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-24", componentSnippet: UbActionsCell }
	];

	async function loadIpBlocks() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams();
			if (ipFilterValues.block_type) params.set("block_type", ipFilterValues.block_type);
			const result = await spamApi.getIpBlocks(params.toString() || undefined);
			if (result.ok) {
				let items = (result.data as IpBlock[]) ?? [];
				if (ipSearch) {
					const q = ipSearch.toLowerCase();
					items = items.filter((b: any) => b.cidr?.toLowerCase().includes(q) || b.reason?.toLowerCase().includes(q));
				}
				ipBlocks = items;
			} else {
				error = result.error ?? t("common.error", "Failed to load IP blocks");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load IP blocks");
		} finally {
			loading = false;
		}
	}

	async function loadCensoredWords() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams();
			if (cwFilterValues.action) params.set("action", cwFilterValues.action);
			const result = await spamApi.getCensoredWords(params.toString() || undefined);
			if (result.ok) {
				let items = (result.data as CensoredWord[]) ?? [];
				if (cwSearch) {
					const q = cwSearch.toLowerCase();
					items = items.filter((w: any) => w.pattern?.toLowerCase().includes(q) || w.action?.toLowerCase().includes(q));
				}
				censoredWords = items;
			} else {
				error = result.error ?? t("common.error", "Failed to load censored words");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load censored words");
		} finally {
			loading = false;
		}
	}

	async function loadUserBlocks() {
		loading = true;
		error = "";
		try {
			const result = await spamApi.getUserBlocks();
			if (result.ok) {
				let items = (result.data as UserBlock[]) ?? [];
				if (ubSearch) {
					const q = ubSearch.toLowerCase();
					items = items.filter((b: any) => b.blocker_id?.toLowerCase().includes(q) || b.blocked_id?.toLowerCase().includes(q));
				}
				userBlocks = items;
			} else {
				error = result.error ?? t("common.error", "Failed to load user blocks");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load user blocks");
		} finally {
			loading = false;
		}
	}

	function switchTab(tab: Tab) {
		activeTab = tab;
		error = "";
		success = "";
		if (tab === "ip-blocks") loadIpBlocks();
		else if (tab === "censored-words") loadCensoredWords();
		else loadUserBlocks();
	}

	async function submitIpBlock() {
		if (!ipForm.cidr.trim() || submitting) return;
		submitting = true;
		error = "";
		try {
			const body: { cidr: string; block_type: string; reason?: string; expires_at?: string } = {
				cidr: ipForm.cidr.trim(),
				block_type: ipForm.block_type
			};
			if (ipForm.reason.trim()) body.reason = ipForm.reason.trim();
			if (ipForm.expires_at) body.expires_at = datetimeLocalToPostgresTimestamptz(ipForm.expires_at);
			const result = await spamApi.createIpBlock(body);
			if (result.ok) {
				showIpModal = false;
				ipForm = { cidr: "", block_type: "full", reason: "", expires_at: "" };
				success = t("spam.block_created", "IP block created");
				loadIpBlocks();
			} else {
				error = result.error ?? t("common.error", "Failed to create IP block");
			}
		} finally {
			submitting = false;
		}
	}

	async function submitCensoredWord() {
		if (!cwForm.pattern.trim() || submitting) return;
		submitting = true;
		error = "";
		try {
			const body: { pattern: string; action: string; replacement?: string } = {
				pattern: cwForm.pattern.trim(),
				action: cwForm.action
			};
			if (cwForm.replacement.trim()) body.replacement = cwForm.replacement.trim();
			const result = await spamApi.createCensoredWord(body);
			if (result.ok) {
				showCwModal = false;
				cwForm = { pattern: "", action: "block", replacement: "" };
				success = t("spam.pattern_created", "Censored pattern created");
				loadCensoredWords();
			} else {
				error = result.error ?? t("common.error", "Failed to create censored word");
			}
		} finally {
			submitting = false;
		}
	}

	function openView(kind: string, row: any) {
		viewTarget = { kind, row };
		showViewModal = true;
	}

	function confirmDelete(type: string, id: string, secondaryId?: string) {
		deleteTarget = { type, id, secondaryId };
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		error = "";
		const { type, id, secondaryId } = deleteTarget;
		let result;
		if (type === "ip-block") result = await spamApi.deleteIpBlock(id);
		else if (type === "censored-word") result = await spamApi.deleteCensoredWord(id);
		else result = await spamApi.deleteUserBlock(id, secondaryId ?? "");

		if (result.ok) {
			showDeleteConfirm = false;
			deleteTarget = null;
			success = t("spam.item_deleted", "Item deleted");
			if (type === "ip-block") loadIpBlocks();
			else if (type === "censored-word") loadCensoredWords();
			else loadUserBlocks();
		} else {
			error = result.error ?? t("common.error", "Failed to delete");
		}
	}

	onMount(() => { loadIpBlocks(); });
</script>

<svelte:head>
	<title>{t("spam.title", "Spam Management")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("spam.title", "Spam Management")}
	description={t("spam.description", "Manage IP blocks, censored words, and user blocks")}
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

<!-- Tab navigation -->
<div class="mb-4 flex flex-wrap gap-1 rounded-lg p-1" style="background: var(--color-surface)">
	{#each tabs as tab}
		{@const Icon = tab.icon}
		<button
			class="inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={activeTab === tab.value
				? "background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)"
				: "color: var(--color-muted)"}
			onclick={() => switchTab(tab.value)}
		>
			<Icon class="h-4 w-4 shrink-0" />
			<span class="whitespace-nowrap">{tab.label}</span>
		</button>
	{/each}
</div>

<!-- IP Blocks Tab -->
{#if activeTab === "ip-blocks"}
	<div class="mb-4 flex items-center justify-between">
		<p class="text-sm" style="color: var(--color-muted)">{ipBlocks.length} {t("spam.ip_blocks_count", "IP blocks configured")}</p>
		{#if canManageSpam}
			<Button onclick={() => { ipForm = { cidr: "", block_type: "full", reason: "", expires_at: "" }; showIpModal = true; }}>
				<Plus class="mr-2 h-4 w-4" /> {t("spam.add_ip_block", "Add IP Block")}
			</Button>
		{/if}
	</div>
	<Card>
		<CardHeader>
			<FilterBar filters={ipBlockFilterDefs} onsearch={handleIpSearch} onclear={handleIpClear} />
		</CardHeader>
		<CardContent>
			{#if loading}
				<TableSkeleton rows={5} columns={ipColumns.length} />
			{:else}
				<DataTable columns={ipColumns} data={ipBlocks} pageSize={10} searchable onsearch={(q) => { ipSearch = q; loadIpBlocks(); }} />
			{/if}
		</CardContent>
	</Card>
{/if}

<!-- Censored Words Tab -->
{#if activeTab === "censored-words"}
	<div class="mb-4 flex items-center justify-between">
		<p class="text-sm" style="color: var(--color-muted)">{censoredWords.length} {t("spam.patterns_count", "patterns configured")}</p>
	{#if canManageSpam}
			<Button onclick={() => { cwForm = { pattern: "", action: "block", replacement: "" }; showCwModal = true; }}>
				<Plus class="mr-2 h-4 w-4" /> {t("spam.add_pattern", "Add Pattern")}
			</Button>
		{/if}
	</div>
	<Card>
		<CardHeader>
			<FilterBar filters={cwFilterDefs} onsearch={handleCwSearch} onclear={handleCwClear} />
		</CardHeader>
		<CardContent>
			{#if loading}
				<TableSkeleton rows={5} columns={cwColumns.length} />
			{:else}
				<DataTable columns={cwColumns} data={censoredWords} pageSize={10} searchable onsearch={(q) => { cwSearch = q; loadCensoredWords(); }} />
			{/if}
		</CardContent>
	</Card>
{/if}

<!-- User Blocks Tab -->
{#if activeTab === "user-blocks"}
	<div class="mb-4 flex items-center justify-between">
		<p class="text-sm" style="color: var(--color-muted)">{userBlocks.length} {t("spam.user_blocks_count", "user blocks found")}</p>
	</div>
	<Card>
		<CardHeader />
		<CardContent>
			{#if loading}
				<TableSkeleton rows={5} columns={ubColumns.length} />
			{:else}
				<DataTable columns={ubColumns} data={userBlocks} pageSize={10} searchable emptyMessage={t("spam.no_user_blocks", "No user blocks found")} onsearch={(q) => { ubSearch = q; loadUserBlocks(); }} />
			{/if}
		</CardContent>
	</Card>
{/if}

<!-- Add IP Block Modal -->
<Modal bind:open={showIpModal} title={t("spam.add_ip_block", "Add IP Block")}>
	<div class="space-y-4">
		<div>
			<label for="ip-cidr" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spam.cidr", "IP / CIDR")}</label>
			<input id="ip-cidr" type="text" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="192.168.1.0/24" bind:value={ipForm.cidr} />
		</div>
		<div>
			<label for="ip-block-type" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spam.block_type", "Block Type")}</label>
			<Select options={blockTypeOptions} bind:value={ipForm.block_type} />
		</div>
		<div>
			<label for="ip-reason" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spam.reason", "Reason")}</label>
			<input id="ip-reason" type="text" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t("spam.reason_placeholder", "Spam source")} bind:value={ipForm.reason} />
		</div>
		<div>
			<label for="ip-expires" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spam.expires", "Expires At")} ({t("common.optional", "optional")})</label>
			<input id="ip-expires" type="datetime-local" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={ipForm.expires_at} />
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showIpModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitIpBlock}>{t("spam.add_block", "Add Block")}</Button>
	</div>
</Modal>

<!-- Add Censored Word Modal -->
<Modal bind:open={showCwModal} title={t("spam.add_pattern", "Add Censored Pattern")}>
	<div class="space-y-4">
		<div>
			<label for="cw-pattern" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spam.pattern", "Pattern")}</label>
			<input id="cw-pattern" type="text" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder={t("spam.pattern_placeholder", "badword")} bind:value={cwForm.pattern} />
		</div>
		<div>
			<label for="cw-action" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spam.action", "Action")}</label>
			<Select id="cw-action" options={actionOptions} bind:value={cwForm.action} />
		</div>
		<div>
			<label for="cw-replacement" class="block text-sm font-medium" style="color: var(--color-foreground)">{t("spam.replacement", "Replacement")} ({t("common.optional", "optional")})</label>
			<input id="cw-replacement" type="text" class="mt-1 w-full rounded-lg border p-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" placeholder="***" bind:value={cwForm.replacement} />
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showCwModal = false}>{t("common.cancel", "Cancel")}</Button>
		<Button onclick={submitCensoredWord}>{t("spam.add_pattern", "Add Pattern")}</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteConfirm} title={t("common.confirmDelete", "Confirm Delete")}>
	<p class="text-sm" style="color: var(--color-foreground)">
		{t("spam.delete_confirm", "Are you sure you want to delete this item?")}
	</p>
	<p class="mt-2 text-sm" style="color: var(--color-danger)">
		{t("common.cannotUndo", "This action cannot be undone.")}
	</p>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteConfirm = false}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t("common.details", "Details")} size="sm">
	{#if viewTarget}
		<div class="space-y-4">
			{#each Object.entries(viewTarget.row) as [k, v]}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{k}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{v === null || v === undefined || v === "" ? "—" : String(v)}</p>
				</div>
			{/each}
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<!-- Action Cells -->
{#snippet IpActionsCell({ value, row }: { value: any; row: IpBlock })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView("ip-block", row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageSpam}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("ip-block", row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

{#snippet CwActionsCell({ value, row }: { value: any; row: CensoredWord })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView("censored-word", row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageSpam}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("censored-word", row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

{#snippet UbActionsCell({ value, row }: { value: any; row: UserBlock })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView("user-block", row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageSpam}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete("user-block", row.blocker_id, row.blocked_id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
