<script lang="ts">
	// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
	import { page } from '$app/state';
	import { ArrowLeft, Pin, Bell, BellOff, TriangleAlert, ShieldCheck, Pencil, Link2, Check } from 'lucide-svelte';
	import { t, siteName} from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { relativeTime } from '$lib/feed/time';
	import { EmptyState } from '$lib/components/ui';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import { scamTypeLabel, scamPlatformLabel } from '$lib/enumOptions';
	import type { ScamReport, ScamPlatform, ScamStatus, ScamType } from '$lib/types';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { breadcrumbJsonLd } from '$lib/seo/breadcrumbs';

	let { data }: { data: PageData } = $props();

	const alert = $derived(data.alert);

	const title = $derived(
		alert
			? `${t('scam_alerts.detail_title', 'Scam Alert')} — ${siteName()}`
			: t('scam_alerts.list_title', 'Scam Alerts — {site}')
	);
	const description = $derived(alert ? alert.description.slice(0, 160) : '');
	// Section 26 — self-referencing canonical on the clean ID URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(alert ? `${page.url.origin}/scam-alerts/${alert.id}` : `${page.url.origin}/scam-alerts`);
	// SpecialAnnouncement JSON-LD (Section 26 — scam alerts are public-safety
	// notices; the Article fallback carries the full report text).
	const scamJsonLd = $derived(
		alert
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'SpecialAnnouncement',
					name: title,
					text: alert.description,
					datePosted: alert.created_at,
					url: canonical,
					category: alert.scam_type
				})
			: ''
	);

	let subscribed = $state(false);
	let subscribing = $state(false);
	let copied = $state(false);

	async function copyLink() {
		try {
			await navigator.clipboard.writeText(canonical);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// clipboard unavailable — no-op
		}
	}

	$effect(() => {
		subscribed = data.alert?.is_subscribed ?? false;
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
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	{#if scamJsonLd}
		{@html safeJsonLdScript(scamJsonLd)}
	{/if}
	{#if alert}
		{@html safeJsonLdScript(
			breadcrumbJsonLd([
				{ name: 'Home', path: page.url.origin },
				{ name: t('scam_alerts.heading', 'Scam Alerts'), path: `${page.url.origin}/scam-alerts` },
				{ name: title, path: canonical }
			])
		)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
	<div class="mb-5 flex items-center justify-between gap-3">
		<a
			href="/scam-alerts"
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft class="rtl:-scale-x-100" size={16} /> {t('scam_alerts.back', 'Back to scam alerts')}
		</a>
		<div class="flex items-center gap-2">
			<button
				type="button"
				onclick={copyLink}
				aria-label={t('scam_alerts.copy_link', 'Copy link')}
				class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
			>
				{#if copied}<Check size={14} class="text-emerald-600" />{:else}<Link2 size={14} />{/if}
				{copied ? t('scam_alerts.link_copied', 'Copied') : t('scam_alerts.copy_link', 'Copy link')}
			</button>
			{#if data.canEdit && alert}
				<a
					href="/scam-alerts/{alert.id}/edit"
					class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]/40 hover:text-[var(--color-primary)]"
				>
					<Pencil size={14} /> {t('common.edit', 'Edit')}
				</a>
			{/if}
		</div>
	</div>

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
					{scamTypeLabel(t, alert.scam_type as ScamType)}
				</span>
				{#if alert.is_pinned}
					<span class="inline-flex items-center gap-0.5 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-semibold text-amber-600">
						<Pin size={11} /> {t('scam_alerts.pinned', 'Pinned')}
					</span>
				{/if}
				<span class={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusClass(alert.status as ScamStatus)}`}>
					{statusLabel(alert.status as ScamStatus)}
				</span>
				<span class="ms-auto text-xs text-[var(--color-muted)]">
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
						<dd class="mt-0.5 text-sm text-[var(--color-text)]">{scamPlatformLabel(t, alert.platform_used as ScamPlatform)}</dd>
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
				{#if alert.moderator_note}
					<div class="sm:col-span-2">
						<dt class="text-xs font-medium text-[var(--color-text-subtle)]">{t('scam_alerts.moderator_note', 'Moderator note')}</dt>
						<dd class="mt-0.5 whitespace-pre-wrap text-sm text-[var(--color-text)]">{alert.moderator_note}</dd>
					</div>
				{/if}
			</dl>

			<!-- Reporter appeal — own report closed as debunked/duplicate (§22) -->
			{#if data.authed && alert.is_reporter && (alert.status === 'debunked' || alert.status === 'duplicate')}
				<div class="mt-6 flex items-center justify-between gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3">
					<p class="text-xs text-[var(--color-muted)]">
						{t('scam_alerts.appeal_hint', 'Think this classification is wrong? You can contest it.')}
					</p>
					<AppealButton
						entityType="scam_report"
						entityId={alert.id}
						label={t('appeals.appeal_decision', 'Appeal decision')}
						class="!px-3 !py-1.5 text-xs"
					/>
				</div>
			{/if}

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
