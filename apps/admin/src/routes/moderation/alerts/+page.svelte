<script lang="ts">
	import { onMount } from 'svelte';
	import Badge from '$lib/components/ui/Badge.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type ModerationAlert = {
		id: string;
		alert_type: 'content_report' | 'ai_flag' | 'security_scan' | 'spam_burst' | 'ban_request';
		severity: 'low' | 'medium' | 'high' | 'critical';
		title: string;
		description: string;
		entity_type?: string;
		entity_id?: string;
		triggered_by?: string;
		status: 'open' | 'acknowledged' | 'resolved';
		created_at: string;
		acknowledged_at?: string;
		resolved_at?: string;
	};

	let alerts = $state<ModerationAlert[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let filterSeverity = $state('');
	let filterStatus = $state('');

	function severityVariant(severity: string): 'danger' | 'warning' | 'default' {
		switch (severity) {
			case 'critical': return 'danger';
			case 'high': return 'danger';
			case 'medium': return 'warning';
			default: return 'default';
		}
	}

	function statusVariant(status: string): 'danger' | 'warning' | 'success' {
		switch (status) {
			case 'open': return 'danger';
			case 'acknowledged': return 'warning';
			case 'resolved': return 'success';
			default: return 'default' as any;
		}
	}

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterSeverity) params.set('severity', filterSeverity);
		if (filterStatus) params.set('status', filterStatus);
		try {
			const result = await api<{ alerts: ModerationAlert[] }>(`/admin/moderation/alerts?${params}`);
			if (result.ok) {
				alerts = result.data.alerts;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function acknowledgeAlert(id: string) {
		try {
			const result = await api(`/admin/moderation/alerts/${id}/acknowledge`, { method: 'POST' });
			if (result.ok) load();
			else errorMessage = result.error;
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function resolveAlert(id: string) {
		try {
			const result = await api(`/admin/moderation/alerts/${id}/resolve`, { method: 'POST' });
			if (result.ok) load();
			else errorMessage = result.error;
		} catch {
			errorMessage = t('common.error');
		}
	}

	function typeIcon(type: string) {
		switch (type) {
			case 'security_scan': return '\u{1F6E1}\uFE0F';
			case 'spam_burst': return '\u{1F6A8}';
			case 'ai_flag': return '\u{1F916}';
			case 'content_report': return '\u{1F4DD}';
			case 'ban_request': return '\u{1F6AB}';
			default: return '\u{26A0}\uFE0F';
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('moderation.alerts_title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('moderation.alerts_title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Real-time moderation alerts and security notifications</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex gap-2">
	<select
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterSeverity}
		onchange={load}
	>
		<option value="">All Severities</option>
		<option value="critical">Critical</option>
		<option value="high">High</option>
		<option value="medium">Medium</option>
		<option value="low">Low</option>
	</select>
	<select
		class="rounded-md border px-3 py-1.5 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		bind:value={filterStatus}
		onchange={load}
	>
		<option value="">All Statuses</option>
		<option value="open">Open</option>
		<option value="acknowledged">Acknowledged</option>
		<option value="resolved">Resolved</option>
	</select>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if alerts.length === 0}
	<EmptyState title="No moderation alerts" description="All clear — no alerts match your current filters." />
{:else}
	<div class="space-y-3">
		{#each alerts as alert}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<div class="flex items-start justify-between">
					<div class="flex items-start gap-3">
						<span class="text-lg">{typeIcon(alert.alert_type)}</span>
						<div>
							<div class="flex items-center gap-2">
								<h3 class="text-sm font-medium">{alert.title}</h3>
								<Badge variant={severityVariant(alert.severity)}>
									{alert.severity}
								</Badge>
								<Badge variant={statusVariant(alert.status)}>
									{alert.status}
								</Badge>
							</div>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">{alert.description}</p>
							<div class="mt-2 flex items-center gap-3 text-xs" style="color: var(--color-muted)">
								<span>{alert.alert_type.replace('_', ' ')}</span>
								{#if alert.entity_type}
									<span>&middot;</span>
									<span>{alert.entity_type}</span>
								{/if}
								<span>&middot;</span>
								<span>{alert.created_at}</span>
							</div>
						</div>
					</div>
					<div class="flex gap-1">
						{#if alert.status === 'open'}
							<button
								class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-warning)"
								onclick={() => acknowledgeAlert(alert.id)}
							>
								Acknowledge
							</button>
						{/if}
						{#if alert.status !== 'resolved'}
							<button
								class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-success)"
								onclick={() => resolveAlert(alert.id)}
							>
								Resolve
							</button>
						{/if}
					</div>
				</div>
			</div>
		{/each}
	</div>
{/if}
