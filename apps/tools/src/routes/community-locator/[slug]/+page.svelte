<script lang="ts">
	import { untrack } from 'svelte';
	import { buildSeoMeta, mainAppUrl } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { t, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import Icon from '$lib/components/ui/Icon.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import type { CommunityOrgWithAi } from '$lib/types';

	/**
	 * Community organisation detail page — §101, ADR-109.
	 * Renders name, type, origin, address, website, meeting frequency, contact,
	 * map link, claim action, and AI/HITL metadata.
	 */
	let { data } = $props();

	let org = $state<CommunityOrgWithAi>(untrack(() => data.org));
	let user = $state(untrack(() => data.user));

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

	async function claimOrg() {
		if (!user) {
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

	const seo = $derived(
		buildSeoMeta({
			title: `${org.name} — ${t('community_locator.seo_title', 'Diaspora Community Locator — {site} Tools')}`,
			description: `Diaspora community organisation in ${org.destination_space_name ?? ''}: ${org.org_type}, ${org.origin_community}.`,
			path: `/community-locator/${page.params.slug}`,
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-community-locator.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'Organization',
				name: org.name,
				description: `Diaspora community organisation in ${org.destination_space_name ?? ''}: ${org.org_type}.`,
				address: org.address ?? undefined,
				url: org.website_url ?? `${page.url.origin}/community-locator/${page.params.slug}`
			}
		})
	);

	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(i18n.locale, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	function mapsUrl(lat: number, lng: number): string {
		// ADR-109: No Google Maps dependency — use OpenStreetMap.
		return `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=16/${lat}/${lng}`;
	}
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/community-locator"
		class="inline-flex items-center gap-1.5 text-sm text-muted hover:text-text transition-colors mb-6"
	>
		<Icon name="arrow-left" size={16} />
		{t('community_locator.back_to_list', 'Back to community locator')}
	</a>

	<div class="p-6 bg-surface border border-border rounded-lg space-y-4">
		<div>
			<h1 class="text-2xl font-bold text-text">{org.name}</h1>
			<p class="text-sm text-text-subtle mt-1">
				{orgTypeLabel(org.org_type)}
				{#if org.origin_community}
					· {org.origin_community}
				{/if}
			</p>
		</div>

		<div class="flex flex-wrap gap-2">
			{#if org.is_verified}
				<span class="inline-flex items-center gap-1 text-xs px-2 py-0.5 bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] rounded">
					<Icon name="badge-check" size={12} />
					{t('community_locator.verified', 'Verified')}
				</span>
			{/if}
			{#if org.is_community_submitted}
				<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">
					{t('community_locator.community_submitted', 'Community')}
				</span>
			{/if}
			<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">
				{t('community_locator.claim_status', 'Claim status')}:
				{claimStatusLabel(org.claim_status)}
			</span>
		</div>
	</div>

	<div class="mt-6 p-6 bg-surface border border-border rounded-lg space-y-3">
		{#if org.destination_space_name || org.city_space_name}
			<div class="flex items-start gap-2 text-sm">
				<Icon name="map-pin" size={16} class="text-muted flex-shrink-0 mt-0.5" />
				<span class="text-text">
					{[org.city_space_name, org.destination_space_name].filter(Boolean).join(', ')}
				</span>
			</div>
		{/if}

		{#if org.address}
			<div class="flex items-start gap-2 text-sm">
				<Icon name="map-pin" size={16} class="text-muted flex-shrink-0 mt-0.5" />
				<span class="text-text">{org.address}</span>
			</div>
		{/if}

		{#if org.website_url}
			<div class="flex items-start gap-2 text-sm">
				<Icon name="globe" size={16} class="text-muted flex-shrink-0 mt-0.5" />
				<a
					href={org.website_url}
					target="_blank"
					rel="noopener noreferrer"
					class="text-[var(--tool-accent)] hover:underline break-all"
				>
					{org.website_url}
				</a>
			</div>
		{/if}

		{#if org.meeting_frequency}
			<div class="flex items-start gap-2 text-sm">
				<Icon name="calendar" size={16} class="text-muted flex-shrink-0 mt-0.5" />
				<span class="text-text">
					{t('community_locator.meets', 'Meets:')} {org.meeting_frequency}
				</span>
			</div>
		{/if}

		{#if org.contact_info}
			<div class="flex items-start gap-2 text-sm">
				<Icon name="mail" size={16} class="text-muted flex-shrink-0 mt-0.5" />
				<span class="text-text">{org.contact_info}</span>
			</div>
		{/if}

		{#if org.latitude != null && org.longitude != null}
			<div class="flex items-start gap-2 text-sm">
				<Icon name="navigation" size={16} class="text-muted flex-shrink-0 mt-0.5" />
				<a
					href={mapsUrl(org.latitude, org.longitude)}
					target="_blank"
					rel="noopener noreferrer"
					class="text-[var(--tool-accent)] hover:underline"
				>
					{t('community_locator.view_on_map', 'View on map')}
				</a>
			</div>
		{/if}

		{#if org.japaconnect_space_slug}
			<div class="pt-2 border-t border-border">
				<a
					href={mainAppUrl(`/spaces/${org.japaconnect_space_slug}`)}
					class="inline-flex items-center gap-1.5 text-sm text-[var(--tool-accent)] hover:underline"
				>
					<Icon name="users" size={14} />
					{t('community_locator.view_space', 'View Space — posts, members & events')}
				</a>
			</div>
		{/if}

		<!-- Claim action — only meaningful while unclaimed -->
		<div class="pt-2 border-t border-border">
			{#if claimSubmitted}
				<span class="inline-flex items-center gap-1.5 text-sm text-[var(--tool-accent)]">
					<Icon name="check-circle" size={16} />
					{t('community_locator.claim_submitted', 'Claim submitted')}
				</span>
			{:else if org.claim_status === 'claimed'}
				<span class="inline-flex items-center gap-1.5 text-sm text-muted">
					<Icon name="check-circle" size={16} />
					{t('community_locator.claim_status_claimed', 'Claimed')}
				</span>
			{:else if org.claim_status === 'pending'}
				<span class="inline-flex items-center gap-1.5 text-sm text-muted">
					<Icon name="check-circle" size={16} />
					{t('community_locator.claim_status_pending', 'Claim pending review')}
				</span>
			{:else if user}
				<button
					onclick={claimOrg}
					disabled={claiming}
					class="inline-flex items-center gap-1.5 px-3 py-1.5 border border-border rounded-lg text-sm font-medium text-text hover:border-[var(--tool-accent)] hover:text-[var(--tool-accent)] transition-colors disabled:opacity-50"
				>
					{#if claiming}
						<Icon name="loader-2" size={14} class="animate-spin" />
						{t('community_locator.claiming', 'Claiming...')}
					{:else}
						<Icon name="hand" size={14} />
						{t('community_locator.claim', 'Claim this organisation')}
					{/if}
				</button>
				{#if claimError}
					<span class="text-xs text-red-500 ms-2">{claimError}</span>
				{/if}
			{:else}
				<button
					onclick={() => (showRegisterPrompt = true)}
					class="inline-flex items-center gap-1.5 px-3 py-1.5 border border-border rounded-lg text-sm font-medium text-text hover:border-[var(--tool-accent)] hover:text-[var(--tool-accent)] transition-colors"
				>
					<Icon name="hand" size={14} />
					{t('community_locator.claim', 'Claim this organisation')}
				</button>
			{/if}
		</div>
	</div>

	{#if org.ai_generated}
		<div class="mt-6 p-4 bg-amber-500/5 border border-amber-500/20 rounded-lg flex items-start gap-2">
			<Icon name="alert-triangle" size={14} class="text-amber-600 flex-shrink-0 mt-0.5" />
			<div class="text-xs text-text leading-relaxed">
				<p>
					{t('common.ai_generated', 'AI-generated content')}
					{#if org.ai_model}({org.ai_model}){/if}
				</p>
				{#if org.hitl_reviewed_by}
					<p class="mt-1 text-muted">
						{t('common.reviewed_by', 'Reviewed by')} {org.hitl_reviewed_by}
						{#if org.hitl_reviewed_at}
							· {formatDate(org.hitl_reviewed_at)}
						{/if}
					</p>
				{:else}
					<p class="mt-1 text-amber-600">
						{t(
							'common.hitl_pending',
							'This content has not been human-reviewed yet. Verify with official sources.'
						)}
					</p>
				{/if}
			</div>
		</div>
	{/if}

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="users"
			title={t('register.community_locator', 'Join the community')}
			description={t(
				'register.community_locator_desc',
				'Create a free account to submit new diaspora organisations and claim existing ones.'
			)}
		/>
	{/if}
</div>
