<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { DataTable } from "$lib/components/ui/data-table";
	import { TableSkeleton } from "$lib/components/ui/skeleton";
	import { RefreshCw } from "lucide-svelte";
	import { auditLogsAdminApi } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";

	type AuditLogEntry = {
		id: string;
		actor: string;
		action: string;
		entity_type: string;
		entity_id: string;
		details?: Record<string, unknown>;
		ip_address: string;
		created_at: string;
	};

	let logs = $state<AuditLogEntry[]>([]);
	let loading = $state(true);
	let error = $state("");

	const columns = [
		{ key: "created_at", label: t("gdpr.timestamp", "Timestamp"), sortable: true,
			render: (v: string) => new Date(v).toLocaleString()
		},
		{ key: "actor", label: t("gdpr.actor", "Actor"), sortable: true },
		{ key: "action", label: t("gdpr.action", "Action"), sortable: true },
		{ key: "entity_type", label: t("gdpr.entity_type", "Entity Type"), sortable: true },
		{ key: "entity_id", label: t("gdpr.entity_id", "Entity ID"), sortable: true },
		{ key: "ip_address", label: t("gdpr.ip", "IP Address"), sortable: true }
	];

	async function loadLogs() {
		loading = true;
		error = "";
		try {
			const result = await auditLogsAdminApi.list();
			if (result.ok) {
				logs = (result.data as AuditLogEntry[]) ?? [];
			} else {
				error = result.error ?? t("common.error", "Failed to load audit logs");
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t("common.error", "Failed to load audit logs");
		} finally {
			loading = false;
		}
	}

	loadLogs();
</script>

<svelte:head>
	<title>{t("gdpr.audit_logs", "Audit Logs")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("gdpr.audit_logs", "Audit Logs")}
	description={t("gdpr.audit_logs_desc", "System audit trail for compliance")}
	actions={[
		{ label: t("common.refresh", "Refresh"), icon: RefreshCw, variant: "ghost", onclick: loadLogs }
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
		{:else if logs.length === 0}
			<div class="py-12 text-center text-sm" style="color: var(--color-muted)">
				{t("gdpr.no_logs", "No audit logs found")}
			</div>
		{:else}
			<DataTable {columns} data={logs} pageSize={20} />
		{/if}
	</CardContent>
</Card>
