<script lang="ts">
	// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
	import { page } from '$app/state';
	import { ArrowLeft, Pin, Bell, BellOff, TriangleAlert, ShieldCheck } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { relativeTime } from '$lib/feed/time';
	import { EmptyState } from '$lib/components/ui';
	import type { ScamReport, ScamStatus, ScamType } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const alert = $derived(data.alert);

	const title = $derived(
		alert
			? `${t('scam_alerts.detail_title', 'Scam Alert')} — JapaConnect`
			: t('scam_alerts.list_title', 'Scam Alerts — JapaConnect')
	);
	const description = $derived(alert ? alert.description.slice(0, 160) : '');

	let subscribed = $state(false);
	let subscribing = $state(false);

	$effect(() => {
		subscribed = false;
	});

	async function toggleSubscribe() {
		if (!data.authed || subscribing || !alert) return;
		subscribing = true;
		const wasSubscribed = subscribed;
		subscribed = !wasSubscribed;
		const res = wasSubscribed
			? await api.delete(`/scam-alerts/${alert.id}/subscribe`)
			: await api.post(`/scam-alerts/${alert.id}/subscribe`);
		if (res.error) subscribed = wasSubscribed;
		subscribing = false;
	}

	function scamTypeLabel(type: ScamType): string {
		return t(`scam_alerts.type.${type}`, type.replace(/_/g, ' '));
	}

	function statusClass(status: ScamStatus): string {
		if (status === 'confirmed') return 'bg-red-500/10 text-red-600';
		if (status === 'under_review') return 'bg-amber-500/10 text-amber-600';
		return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
	}

	function statusLabel(status: ScamStatus): string {
		return t(`scam_alerts.status.${status}`, status.replace(/_/g, ' '));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full px-4 py-6">
	<a
		href="/scam-alerts"
		class="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft size={16} /> {t('scam_alerts.back', 'Back to scam alerts')}
	</a>

	{#if data.loadError || !alert}
		<EmptyState
			icon={TriangleAlert}
			title={t('scam_alerts.load_error_title', 'Failed to load alert')}
			body={t('scam_alerts.load_error_body', 'Please try again later.')}
		/>
	{:else}
		<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6">
			<!-- Badges -->
			<div class="flex flex-wrap items-center gap-2">
				<span class="rounded-full bg-[var(--color-primary)]/10 px-2.5 py-0.5 text-xs font-semibold text-[var(--color-primary)]">
					{scamTypeLabel(alert.scam_type as ScamType)}
				</span>
				{#if alert.is_pinned}
					<span class="inline-flex items-center gap-0.5 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-semibold text-amber-600">
						<Pin size={11} /> {t('scam_alerts.pinned', 'Pinned')}
					</span>
				{/if}
				<span class={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusClass(alert.status as ScamStatus)}`}>
					{statusLabel(alert.status as ScamStatus)}
				</span>
				<span class="ml-auto text-xs text-[var(--color-muted)]">
					{relativeTime(alert.created_at, new Date(), i18n.locale)}
				</span>
			</div>

			<!-- Description -->
			<div class="prose mt-4 max-w-none text-[var(--color-text)]">
				<p class="whitespace-pre-wrap">{alert.description}</p>
			</div>

			<!-- Metadata grid -->
			<dl class="mt-6 grid grid-cols-1 gap-x-6 gap-y-3 border-t border-[var(--color-border)] pt-4 sm:grid-cols-2">
				{#if alert.destination_space_name}
					<div>
						<dt class="text-xs font-medium text-[var(--color-text-subtle)]">{t('scam_alerts.destination', 'Destination')}</dt>
						<dd class="mt-0.5 text-sm text-[var(--color-text)]">{alert.destination_space_name}</dd>
					</div>
				{/if}
				{#if alert.origin_space_name}
					<div>
						<dt class="text-xs font-medium text-[var(--color-text-subtle)]">{t('scam_alerts.origin_country', 'Origin country')}</dt>
						<dd class="mt-0.5 text-sm text-[var(--color-text)]">{alert.origin_space_name}</dd>
					</div>
				{/if}
				{#if alert.origin_community}
					<div>
						<dt class="text-xs font-medium text-[var(--color-text-subtle)]">{t('scam_alerts.origin_community', 'Origin community')}</dt>
						<dd class="mt-0.5 text-sm text-[var(--color-text)]">{alert.origin_community}</dd>
					</div>
				{/if}
				{#if alert.platform_used}
					<div>
						<dt class="text-xs font-medium text-[var(--color-text-subtle)]">{t('scam_alerts.platform', 'Platform')}</dt>
						<dd class="mt-0.5 text-sm text-[var(--color-text)]">{alert.platform_used}</dd>
					</div>
				{/if}
				{#if alert.approximate_date}
					<div>
						<dt class="text-xs font-medium text-[var(--color-text-subtle)]">{t('scam_alerts.date', 'Approximate date')}</dt>
						<dd class="mt-0.5 text-sm text-[var(--color-text)]">{alert.approximate_date}</dd>
					</div>
				{/if}
				{#if alert.amount_range}
					<div>
						<dt class="text-xs font-medium text-[var(--color-text-subtle)]">{t('scam_alerts.amount', 'Amount range')}</dt>
						<dd class="mt-0.5 text-sm text-[var(--color-text)]">{alert.amount_range}</dd>
					</div>
				{/if}
				<div>
					<dt class="text-xs font-medium text-[var(--color-text-subtle)]">{t('scam_alerts.reported_to_authorities', 'Reported to authorities')}</dt>
					<dd class="mt-0.5 flex items-center gap-1 text-sm text-[var(--color-text)]">
						{#if alert.reported_to_authorities}
							<ShieldCheck size={14} class="text-green-600" /> {t('common.yes', 'Yes')}
						{:else}
							{t('common.no', 'No')}
						{/if}
					</dd>
				</div>
			</dl>

			<!-- Subscribe action -->
			<div class="mt-6 flex items-center justify-end border-t border-[var(--color-border)] pt-4">
				{#if data.authed}
					<button
						type="button"
						onclick={toggleSubscribe}
						disabled={subscribing}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						{#if subscribed}
							<BellOff size={14} /> {t('scam_alerts.unsubscribe', 'Unsubscribe')}
						{:else}
							<Bell size={14} /> {t('scam_alerts.subscribe', 'Subscribe to updates')}
						{/if}
					</button>
				{:else}
					<a
						href="/login?redirect=/scam-alerts/{alert.id}"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<Bell size={14} /> {t('scam_alerts.login_to_subscribe', 'Sign in to subscribe')}
					</a>
				{/if}
			</div>
		</article>
	{/if}
</div>
