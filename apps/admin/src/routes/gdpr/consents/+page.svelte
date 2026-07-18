<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { Modal } from "$lib/components/ui/modal";
	import { Button } from "$lib/components/ui/button";
	import { RefreshCw, Eye } from "lucide-svelte";
	import { gdprAdminApiExtendedV2 } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";

	type Consent = {
		id: string;
		user_id: string;
		consent_type: string;
		granted: boolean;
		granted_at?: string;
		revoked_at?: string;
		ip_address?: string;
	};

	let consents = $state<Consent[]>([]);
	let loading = $state(true);
	let error = $state("");

	let showViewModal = $state(false);
	let viewTarget = $state<Consent | null>(null);

	function openView(consent: Consent) {
		viewTarget = consent;
		showViewModal = true;
	}

	const columns = [
		{ key: "user_id", label: t("gdpr.user", "User ID"), sortable: true },
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
		{ key: "ip_address", label: t("gdpr.ip", "IP Address"), sortable: true },
		{ key: "actions", label: t("common.actions", "Actions"), class: "w-24", component: ConsentActionsCell }
	];

	async function loadConsents() {
		loading = true;
		error = "";
		try {
			const result = await gdprAdminApiExtendedV2.listConsents();
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

	loadConsents();
</script>

<svelte:head>
	<title>{t("gdpr.consents", "Consents")} — JapaConnect Admin</title>
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

<Card>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={10} columns={columns.length} />
		{:else if consents.length === 0}
			<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
				{t("gdpr.no_consents", "No consent records found")}
			</div>
		{:else}
			<DataTable {columns} data={consents} pageSize={20} />
		{/if}
	</CardContent>
</Card>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t("gdpr.consent_details", "Consent Details")} size="sm">
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
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t("common.close", "Close")}</Button>
	</div>
</Modal>

<!-- ConsentActionsCell component -->
{#snippet ConsentActionsCell(row: Consent)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
	</div>
{/snippet}
