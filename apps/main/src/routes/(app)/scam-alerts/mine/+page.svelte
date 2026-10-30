<script lang="ts">
	// Ref: japaconnect-engineering/sections/81-scam-alert-board.md — BUG-016/BUG-019
	import { ArrowLeft, BellOff, TriangleAlert, Pin } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { relativeTime } from '$lib/feed/time';
	import { scamTypeLabel } from '$lib/enumOptions';
	import { EmptyState } from '$lib/components/ui';
	import type { ScamReport, ScamType, ScamStatus, ScamAlertSubscription } from '$lib/types';
	import type { PageData } from './$types';

	import { untrack } from 'svelte';
	let { data }: { data: PageData } = $props();
	let reports = $state<ScamReport[]>(untrack(() => data.reports));
	let subs = $state<ScamAlertSubscription[]>(untrack(() => data.subscriptions));
	let busy = $state('');

	function statusClass(status: ScamStatus): string {
		if (status === 'confirmed') return 'bg-red-500/10 text-red-600';
		if (status === 'under_review') return 'bg-amber-500/10 text-amber-600';
		return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
	}
	function excerpt(text: string, max = 140): string {
		return text.length <= max ? text : text.slice(0, max).trimEnd() + '…';
	}
	async function unsubscribe(spaceId: string) {
		if (busy) return;
		busy = spaceId;
		const res = await api.delete(`/me/scam-alert-subscriptions/${spaceId}`);
		if (!res.error) subs = subs.filter((s) => s.destination_space_id !== spaceId);
		busy = '';
	}
</script>

<svelte:head>
	<title>{t('scam_alerts.my_reports_title', 'My Scam Reports — {site}')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
	<div class="mb-5 flex items-center gap-3">
		<a href="/scam-alerts" class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] hover:text-[var(--color-text)]">
			<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('scam_alerts.back', 'Back to scam alerts')}
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('scam_alerts.my_reports', 'My reports')}</h1>
	</div>

	{#if reports.length === 0}
		<EmptyState
			icon={TriangleAlert}
			title={t('scam_alerts.my_empty_title', 'No reports yet')}
			body={t('scam_alerts.my_empty_body', 'Reports you submit appear here so you can track their review.')}
			action={{ label: t('scam_alerts.report_button', 'Report a Scam'), href: '/scam-alerts/new' }}
		/>
	{:else}
		<div class="space-y-3">
			{#each reports as alert (alert.id)}
				<a href="/scam-alerts/{alert.id}" class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40">
					<div class="flex flex-wrap items-center gap-2">
						<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-semibold text-[var(--color-primary)]">{scamTypeLabel(t, alert.scam_type as ScamType)}</span>
						{#if alert.is_pinned}<span class="inline-flex items-center gap-0.5 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-semibold text-amber-600"><Pin size={11} /> {t('scam_alerts.pinned', 'Pinned')}</span>{/if}
						<span class={`rounded-full px-2 py-0.5 text-xs font-semibold ${statusClass(alert.status as ScamStatus)}`}>{t(`scam_alerts.status.${alert.status}`, alert.status.replace(/_/g, ' '))}</span>
						<span class="ms-auto text-xs text-[var(--color-muted)]">{relativeTime(alert.created_at, new Date(), i18n.locale)}</span>
					</div>
					<p class="mt-2 text-sm text-[var(--color-text)]">{excerpt(alert.description)}</p>
				</a>
			{/each}
		</div>
	{/if}

	{#if subs.length > 0}
		<h2 class="mt-8 mb-3 text-sm font-semibold text-[var(--color-text)]">{t('scam_alerts.my_subscriptions', 'Alert subscriptions')}</h2>
		<div class="space-y-2">
			{#each subs as sub (sub.destination_space_id)}
				<div class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5">
					<span class="text-sm text-[var(--color-text)]">{sub.destination_space_name ?? sub.destination_space_code ?? sub.destination_space_id}</span>
					<button type="button" disabled={busy === sub.destination_space_id} onclick={() => unsubscribe(sub.destination_space_id)} class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-text-subtle)] hover:text-[var(--color-danger)] disabled:opacity-50">
						<BellOff size={13} /> {t('scam_alerts.unsubscribe', 'Unsubscribe')}
					</button>
				</div>
			{/each}
		</div>
	{/if}
</div>
