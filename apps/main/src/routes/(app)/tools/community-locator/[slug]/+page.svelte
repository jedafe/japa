<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Users, ArrowLeft, MapPin, Globe, Mail, Navigation, BadgeCheck, Hand, CheckCircle2, Loader2 } from 'lucide-svelte';
	import { Modal } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import type { PageData } from './$types';

	/**
	 * Community Locator organisation detail page — Section 101, ADR-109.
	 *
	 * Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
	 */
	let { data }: { data: PageData } = $props();

	const org = $derived(data.org!);
	const authed = $derived(data.authed);

	const title = $derived(`${org.name} — ${t('community_locator.title', 'Diaspora Community Locator')}`);
	const description = $derived(org.meeting_frequency ? `${t('community_locator.title', 'Diaspora community')} — ${org.meeting_frequency}` : t('community_locator.subtitle', 'Find diaspora organisations near your destination.'));

	// Claim state — mirrors the list page (claim only while unclaimed).
	let claiming = $state(false);
	let claimError = $state<string | null>(null);
	let claimSubmitted = $state(false);
	let showRegisterPrompt = $state(false);

	function prettifyEnum(value: string): string {
		return value.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}
	function orgTypeLabel(value: string): string {
		return t(`community_locator.type_${value}`, prettifyEnum(value));
	}
	function claimStatusLabel(value: string): string {
		return t(`community_locator.claim_status_${value}`, prettifyEnum(value));
	}

	function mapsUrl(lat: number, lng: number): string {
		return `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=16/${lat}/${lng}`;
	}

	async function claimOrg() {
		if (!authed) {
			showRegisterPrompt = true;
			return;
		}
		claiming = true;
		claimError = null;
		const res = await api.post<{ claim_status: string }>(`/community-locator/${org.id}/claim`);
		claiming = false;
		if (res.error) {
			claimError = res.error.message;
			return;
		}
		claimSubmitted = true;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/tools/community-locator"
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('community_locator.back', 'Back to community locator')}
		</a>
	</div>

	<div class="p-4 space-y-4">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<div class="flex items-start gap-3">
				<div class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
					<Users size={20} />
				</div>
				<div class="min-w-0 flex-1">
					<div class="flex flex-wrap items-center gap-2">
						<h1 class="text-lg font-semibold text-[var(--color-text)]">{org.name}</h1>
						{#if org.is_verified}
							<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">
								<BadgeCheck size={12} />
								{t('community_locator.verified', 'Verified')}
							</span>
						{/if}
					</div>
					<div class="mt-1 flex flex-wrap gap-2 text-xs text-[var(--color-muted)]">
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 capitalize">{orgTypeLabel(org.org_type)}</span>
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{org.origin_community}</span>
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">
							{t('community_locator.claim_status', 'Claim status:')} {claimStatusLabel(org.claim_status)}
						</span>
					</div>
					{#if org.destination_space_name}
						<p class="mt-2 flex items-center gap-1 text-sm text-[var(--color-text-subtle)]">
							<MapPin size={14} />
							{org.destination_space_name}{#if org.city_space_name}, {org.city_space_name}{/if}
						</p>
					{/if}
				</div>
			</div>
		</div>

		{#if org.address}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('community_locator.address', 'Address')}</h2>
				<p class="text-sm text-[var(--color-text-subtle)]">{org.address}</p>
				{#if org.latitude != null && org.longitude != null}
					<a
						href={mapsUrl(org.latitude, org.longitude)}
						target="_blank"
						rel="noopener noreferrer"
						class="mt-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
					>
						<Navigation size={14} />
						{t('community_locator.view_map', 'View on map')}
					</a>
				{/if}
			</div>
		{/if}

		{#if org.website_url}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('community_locator.website', 'Website')}</h2>
				<a
					href={org.website_url}
					target="_blank"
					rel="noopener noreferrer"
					class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
				>
					<Globe size={14} />
					{org.website_url}
				</a>
			</div>
		{/if}

		{#if org.meeting_frequency}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('community_locator.meeting_frequency', 'Meeting frequency')}</h2>
				<p class="text-sm text-[var(--color-text-subtle)]">{org.meeting_frequency}</p>
			</div>
		{/if}

		{#if org.contact_info}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('community_locator.contact_info', 'Contact info')}</h2>
				<p class="text-sm text-[var(--color-text-subtle)] whitespace-pre-line">{org.contact_info}</p>
			</div>
		{/if}

		{#if org.claim_rejected}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<p class="mb-2 text-sm text-[var(--color-muted)]">
					{t('community_locator.claim_rejected', 'Your claim was rejected.')}
				</p>
				<AppealButton entityType="community_org_claim" entityId={org.id} />
			</div>
		{/if}

		{#if org.is_submitter && org.is_removed}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<AppealButton entityType="community_org" entityId={org.id} />
			</div>
		{/if}

		{#if org.claim_status === 'claimed' && (org.japaconnect_space_slug || org.japaconnect_space_id)}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">
					{t('community_locator.space_integration', 'JapaConnect Space')}
				</h2>
				<a
					href={`/spaces/${org.japaconnect_space_slug || org.japaconnect_space_id}`}
					class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
				>
					<Users size={14} />
					{t('community_locator.view_space', 'View Space — posts, members & events')}
				</a>
			</div>
		{/if}

		<!-- Claim action — only meaningful while unclaimed -->
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			{#if claimSubmitted}
				<span class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)]">
					<CheckCircle2 size={16} />
					{t('community_locator.claimed_locally', 'Claim submitted — pending review')}
				</span>
			{:else if org.claim_status === 'claimed'}
				<span class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
					<CheckCircle2 size={16} />
					{t('community_locator.claim_status_claimed', 'Claimed')}
				</span>
			{:else if org.claim_status === 'pending'}
				<span class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)]">
					<CheckCircle2 size={16} />
					{t('community_locator.claim_status_pending', 'Claim pending review')}
				</span>
			{:else}
				<button
					type="button"
					onclick={claimOrg}
					disabled={claiming}
					class="inline-flex items-center gap-1.5 px-3 py-1.5 border border-[var(--color-border)] rounded-[var(--radius)] text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)] transition-colors disabled:opacity-50"
				>
					{#if claiming}
						<Loader2 size={14} class="animate-spin" />
						{t('community_locator.claiming', 'Claiming...')}
					{:else}
						<Hand size={14} />
						{t('community_locator.claim', 'Claim this organisation')}
					{/if}
				</button>
				{#if claimError}
					<p class="text-xs text-red-500 mt-2">{claimError}</p>
				{/if}
			{/if}
		</div>

		{#if org.is_community_submitted}
			<p class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
				<Hand size={12} />
				{t('community_locator.community_submitted', 'Community-submitted listing')}
			</p>
		{/if}
	</div>
</div>

<!-- Register prompt for unauthenticated claim attempts -->
<Modal bind:open={showRegisterPrompt} title={t('community_locator.register_prompt_title', 'Create a free account')}>
	<p class="text-sm text-[var(--color-muted)]">
		{t(
			'community_locator.register_prompt_body',
			'You need an account to submit or claim organisations.'
		)}
	</p>
	<div class="mt-4 flex items-center justify-end gap-2">
		<a
			href="/login"
			class="px-4 py-2 text-sm font-medium border border-[var(--color-border)] text-[var(--color-text)] rounded-[var(--radius)] hover:bg-[var(--color-surface-subtle)]"
		>
			{t('nav.login', 'Log in')}
		</a>
		<a
			href="/register"
			class="px-4 py-2 text-sm font-medium bg-[var(--color-primary)] text-white rounded-[var(--radius)] hover:bg-[var(--color-primary-hover)]"
		>
			{t('nav.register', 'Register')}
		</a>
	</div>
</Modal>
