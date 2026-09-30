<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Users, ArrowLeft, MapPin, Globe, Mail, Navigation, BadgeCheck, Hand, Map as MapIcon } from 'lucide-svelte';
	import type { PageData } from './$types';

	/**
	 * Community Locator organisation detail page — Section 101, ADR-109.
	 *
	 * Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
	 */
	let { data }: { data: PageData } = $props();

	const org = $derived(data.org!);

	const title = $derived(`${org.name} — ${t('community_locator.title', 'Diaspora Community Locator')}`);
	const description = $derived(org.meeting_frequency ? `${t('community_locator.title', 'Diaspora community')} — ${org.meeting_frequency}` : t('community_locator.subtitle', 'Find diaspora organisations near your destination.'));

	function mapsUrl(lat: number, lng: number): string {
		return `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=16/${lat}/${lng}`;
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
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 capitalize">{org.org_type}</span>
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{org.origin_community}</span>
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

		{#if org.is_community_submitted}
			<p class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
				<Hand size={12} />
				{t('community_locator.community_submitted', 'Community-submitted listing')}
			</p>
		{/if}
	</div>
</div>
