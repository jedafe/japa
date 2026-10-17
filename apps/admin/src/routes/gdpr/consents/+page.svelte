<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import type { ActionItem, FilterDef } from "$lib/components/ui/data-table";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { SlideOver } from "$lib/components/ui/slide-over";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { RefreshCw, Eye, Trash2 } from "lucide-svelte";
	import { gdprAdminApiExtendedV2, api } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { page } from "$app/state";

	type Consent = {
		id: string;
		user_id: string;
		username?: string;
		display_name?: string;
		consent_type: string;
		granted: boolean;
		granted_at?: string;
		revoked_at?: string;
		ip_address?: string;
	};

	let consents = $state<Consent[]>([]);
	let loading = $state(true);
	let error = $state("");
	let success = $state("");
	let searchQuery = $state(""); // bound to the DataTable search box; forwarded to the API

	let consentTypeOptions = $state<{ value: string; label: string }[]>([
		{ value: "marketing", label: t("app.gdpr.consents.option_marketing", "Marketing") },
		{ value: "analytics", label: t("app.gdpr.consents.option_analytics", "Analytics") },
		{ value: "functional", label: t("app.gdpr.consents.option_functional", "Functional") },
		{ value: "necessary", label: t("app.gdpr.consents.option_necessary", "Necessary") }
	]);

	$effect(() => {
		void (async () => {
			const res = await api<{ consent_types: string[]; policy_version: string }>('/consent/config');
			if (res.ok && res.data?.consent_types) {
				consentTypeOptions = res.data.consent_types.map((ct) => ({
					value: ct,
					label: ct
						.split('_')
						.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
						.join(' ')
				}));
			}
		})();
	});

	const filterDefs = $derived<FilterDef[]>([
		{ key: "consent_type", label: t("app.gdpr.consents.filter_consent_type", "Consent Type"), options: consentTypeOptions },
		{ key: "granted", label: t("app.gdpr.consents.filter_status", "Status"), options: [
			{ value: "true", label: t("app.gdpr.consents.option_granted", "Granted") },
			{ value: "false", label: t("app.gdpr.consents.option_revoked", "Revoked") }
		]}
	]);
	let filterValues = $state<Record<string, string>>({});

	function handleFilterApply() {
		searchQuery = filterValues.search ?? "";
		loadConsents();
	}

	function handleFilterClear() {
		filterValues = {};
		searchQuery = "";
		loadConsents();
	}

	const canDeleteConsent = $derived(hasPermission("gdpr.consents.delete"));
	let showDeleteModal = $state(false);
	let deleteTarget = $state<Consent | null>(null);

	// Bulk delete state
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	let showViewSlideOver = $state(false);
	let viewTarget = $state<Consent | null>(null);

	function openView(consent: Consent) {
		viewTarget = consent;
		showViewSlideOver = true;
	}

	function openDelete(consent: Consent) {
		deleteTarget = consent;
		showDeleteModal = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		const result = await gdprAdminApiExtendedV2.deleteConsent(deleteTarget.id);
		if (result.ok) {
			showDeleteModal = false;
			success = t("gdpr.consent_deleted", "Consent deleted successfully");
			loadConsents();
		} else {
			error = result.error ?? t("common.error", "Failed to delete consent");
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const id of selectedIds) {
				await gdprAdminApiExtendedV2.deleteConsent(id);
			}
			selectedIds = new Set();
			success = t("gdpr.consents_bulk_deleted", "Selected consents deleted");
			loadConsents();
		} catch {
			error = t("common.error", "Failed to delete consents");
		} finally {
			bulkDeleting = false;
		}
	}

	const columns = [
		{ key: "display_name", label: t("gdpr.user", "User"), sortable: true,
			render: (v: string, row: Consent) => v || row.username || 'anonymous' },
		{ key: "consent_type", label: t("gdpr.consent_type", "Consent Type"), sortable: true },
		{ key: "granted", label: t("gdpr.granted", "Granted"), sortable: true,
			render: (v: boolean) => v ? t("common.yes", "Yes") : t("common.no", "No")
		},
		{ key: "granted_at", label: t("gdpr.granted_at", "Granted At"), sortable: true,
			render: (v: string) => v ? new Date(v).toLocaleString() : "—"
		},
		{ key: "revoked_at", label: t("gdpr.revoked_at", "Revoked At"), sortable: true,
			render: (v: string) => v ? new Date(v).toLocaleString() : "—"
		},
		{ key: "ip_address", label: t("gdpr.ip", "IP Address"), sortable: true }
	];

	const rowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Consent) => openView(row) },
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: Consent) => openDelete(row),
			show: () => canDeleteConsent
		}
	];

	async function loadConsents() {
		loading = true;
		error = "";
		try {
			const params = new URLSearchParams();
			if (searchQuery) params.set("q", searchQuery);
			if (filterValues.consent_type) params.set("consent_type", filterValues.consent_type);
			if (filterValues.granted) params.set("granted", filterValues.granted);
			const qs = params.toString();
			const result = await gdprAdminApiExtendedV2.listConsents(qs || undefined);
			if (result.ok) {
				consents = (result.data as Consent[]) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load consents");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load consents");
		} finally {
			loading = false;
		}
	}

	// Ref: jc-admin-develop — reactive $effect so URL param changes re-fetch
	$effect(() => {
		// Restore the in-progress query from the URL so the table matches the search box on reload.
		searchQuery = page.url.searchParams.get("q") ?? "";
		loadConsents();
	});
</script>

<svelte:head>
	<title>{t("gdpr.consents", "Consents")} — {siteName()} Admin</title>
</svelte:head>

<PageHeader
	title={t("gdpr.consents", "Consent Records")}
	description={t("gdpr.consents_desc", "View user consent records for GDPR compliance")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadConsents }
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

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={10} columns={columns.length} />
		{:else if consents.length === 0}
			<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
				{t("gdpr.no_consents", "No consent records found")}
			</div>
		{:else}
			<DataTable
				{columns}
				data={consents}
				filters={filterDefs}
				bind:filterValue={filterValues}
				onfilterapply={handleFilterApply}
				onfilterclear={handleFilterClear}
				actions={rowActions}
				pageSize={20}
				searchable
				onsearch={(q) => { searchQuery = q; loadConsents(); }}
				selectable={canDeleteConsent}
				bind:selectedIds
			>
				{#snippet bulkActions()}
					{#if selectedIds.size > 0 && canDeleteConsent}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting}>
							<Trash2 class="mr-1 h-3.5 w-3.5" />
							{t("app.gdpr.consents.delete_selected", "Delete Selected")} ({selectedIds.size})
						</Button>
					{/if}
				{/snippet}
			</DataTable>
		{/if}
	</CardContent>
</Card>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t("gdpr.consent_details", "Consent Details")}>
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("gdpr.user", "User ID")}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.user_id}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("gdpr.consent_type", "Consent Type")}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.consent_type}</p>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("gdpr.granted", "Granted")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.granted ? t("common.yes", "Yes") : t("common.no", "No")}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("gdpr.ip", "IP Address")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.ip_address || "—"}</p>
				</div>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("gdpr.granted_at", "Granted At")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.granted_at ? new Date(viewTarget.granted_at).toLocaleString() : "—"}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t("gdpr.revoked_at", "Revoked At")}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.revoked_at ? new Date(viewTarget.revoked_at).toLocaleString() : "—"}</p>
				</div>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end">
			<Button variant="outline" onclick={() => (showViewSlideOver = false)}>{t("common.close", "Close")}</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Modal -->
<Modal bind:open={showDeleteModal} title={t("gdpr.consent_delete_title", "Delete Consent")}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t("gdpr.consent_delete_confirm", "Permanently delete this consent record?")}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t("common.cancel", "Cancel")}</Button>
		<Button variant="destructive" onclick={confirmDelete}>{t("common.delete", "Delete")}</Button>
	</div>
</Modal>
