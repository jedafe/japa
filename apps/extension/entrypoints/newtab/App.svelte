<script lang="ts">
	import { onMount } from 'svelte';
	import { ApiError, getDigest, getFeed } from '../../lib/api';
	import { contentHref, siteUrl } from '../../lib/links';
	import {
		getCredential,
		getValidAccessToken,
		login,
		logout,
		type Credential,
	} from '../../lib/oauth';
	import {
		clearAuth,
		getCachedDigest,
		getOAuthUser,
		setApiKey,
		setCachedDigest,
		type DigestCache,
		type OAuthUser,
	} from '../../lib/storage';
	import type { Digest, FeedItem } from '../../lib/types';
	import FeedCard from './FeedCard.svelte';
	import SettingsPanel from './SettingsPanel.svelte';

	// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
	// (Browser-Extension Digest) — anonymous users see popular + a connect
	// card; connected users get the aggregate digest (one call per tab open).
	// §120 v1.4: "connected" is either an OAuth bearer session (primary) or
	// a pasted extension API key (fallback).

	let credential = $state<Credential>(null);
	let user = $state<OAuthUser | null>(null);
	let digest = $state<Digest | null>(null);
	let anonFeed = $state<FeedItem[]>([]);
	let stale = $state(false);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let showSettings = $state(false);

	const communityTabs = ['popular', 'latest', 'rising'] as const;
	let communityTab = $state<(typeof communityTabs)[number]>('popular');
	let communityItems = $state<FeedItem[]>([]);
	let communityLoading = $state(false);

	async function refresh(retried = false) {
		error = null;
		if (!credential) {
			// Anonymous: only feeds/popular is reachable without a credential.
			try {
				anonFeed = await getFeed('popular', null);
			} catch (e) {
				error = e instanceof ApiError ? e.message : 'Could not load feed';
			} finally {
				loading = false;
			}
			return;
		}

		// Stale-while-revalidate: paint the cached digest instantly, then
		// refresh from the API.
		const cached: DigestCache | null = await getCachedDigest();
		if (cached) {
			digest = cached.digest;
			stale = true;
			loading = false;
		}

		try {
			digest = await getDigest(credential);
			await setCachedDigest(digest);
			stale = false;
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) {
				// OAuth: force one refresh + retry — the access token may have
				// been revoked/expired since the session was stored.
				if (!retried && credential?.kind === 'oauth') {
					const token = await getValidAccessToken(true);
					if (token) {
						credential = { kind: 'oauth', token };
						return refresh(true);
					}
				}
				// Dead credential (revoked grant or key) — drop it and fall
				// back to anonymous.
				await clearAuth();
				credential = null;
				user = null;
				digest = null;
				loading = true;
				return refresh();
			}
			if (!cached) error = e instanceof ApiError ? e.message : 'Could not load digest';
		} finally {
			loading = false;
		}
	}

	async function selectCommunityTab(tab: (typeof communityTabs)[number]) {
		communityTab = tab;
		if (tab === 'popular' && digest) {
			communityItems = digest.community.popular;
			return;
		}
		communityLoading = true;
		try {
			communityItems = await getFeed(tab, credential);
		} catch {
			communityItems = [];
		} finally {
			communityLoading = false;
		}
	}

	async function onConnected(key: string) {
		await setApiKey(key);
		credential = { kind: 'apikey', token: key };
		showSettings = false;
		loading = true;
		await refresh();
	}

	async function onLogin() {
		await login(); // throws LoginCancelledError on abort — the panel filters it
		credential = await getCredential();
		user = await getOAuthUser();
		showSettings = false;
		loading = true;
		await refresh();
	}

	async function onDisconnected() {
		await logout(); // revokes the refresh token + clears OAuth state
		await clearAuth(); // also drops a pasted API key, if any
		credential = null;
		user = null;
		digest = null;
		showSettings = false;
		loading = true;
		await refresh();
	}

	onMount(async () => {
		credential = await getCredential();
		if (credential?.kind === 'oauth') user = await getOAuthUser();
		await refresh();
		if (digest) communityItems = digest.community.popular;
	});
</script>

<div class="page">
	<header class="topbar">
		<div class="brand">
			<span class="brand-mark">J</span>
			<span class="brand-name">JapaConnect</span>
		</div>
		<div class="topbar-actions">
			{#if stale}
				<span class="badge-stale">offline cache</span>
			{/if}
			<button class="ghost" onclick={() => refresh()} aria-label="Refresh feed">Refresh</button>
			<button class="ghost" onclick={() => (showSettings = !showSettings)} aria-label="Settings">
				{credential ? 'Settings' : 'Connect'}
			</button>
		</div>
	</header>

	{#if showSettings}
		<SettingsPanel
			connected={credential !== null}
			{user}
			{onConnected}
			{onLogin}
			{onDisconnected}
			onClose={() => (showSettings = false)}
		/>
	{/if}

	{#if loading}
		<div class="skeleton-grid" aria-busy="true" aria-label="Loading feed">
			{#each Array(6) as _}
				<div class="skeleton-card"></div>
			{/each}
		</div>
	{:else if credential === null}
		<!-- Anonymous: connect card + public popular feed -->
		<section class="connect-hero">
			<h1>Your migration feed, on every new tab</h1>
			<p>
				Visa updates, destination intel, scam alerts, and community activity for where you're
				headed — personalized once you connect your JapaConnect account.
			</p>
			<button class="primary" onclick={() => (showSettings = true)}>Connect your account</button>
		</section>
		<section>
			<h2>Popular in the community</h2>
			{#if error}
				<p class="error" role="alert">{error} <button class="ghost" onclick={() => refresh()}>Retry</button></p>
			{:else}
				<div class="card-grid">
					{#each anonFeed as item (item.id)}
						<FeedCard {item} />
					{/each}
				</div>
			{/if}
		</section>
	{:else if digest}
		{#if error}
			<p class="error" role="alert">{error} <button class="ghost" onclick={() => refresh()}>Retry</button></p>
		{/if}

		<div class="sections">
			<section class="main-col">
				<h2>For you</h2>
				<div class="card-grid">
					{#each digest.for_you.items as item (item.id)}
						<FeedCard {item} />
					{:else}
						<p class="empty">Nothing personalized yet — follow a destination space on japa.to.</p>
					{/each}
				</div>

				<h2>
					Community
					<span class="tabs" role="tablist">
						{#each communityTabs as tab}
							<button
								role="tab"
								aria-selected={communityTab === tab}
								class:active={communityTab === tab}
								onclick={() => selectCommunityTab(tab)}>{tab}</button
							>
						{/each}
					</span>
				</h2>
				{#if communityLoading}
					<div class="skeleton-grid">
						{#each Array(3) as _}
							<div class="skeleton-card"></div>
						{/each}
					</div>
				{:else}
					<div class="card-grid">
						{#each communityItems as item (item.id)}
							<FeedCard {item} />
						{:else}
							<p class="empty">No community posts right now.</p>
						{/each}
					</div>
				{/if}
			</section>

			{#if digest.destination}
				<aside class="side-col">
					<h2>
						<a href={siteUrl('/spaces/' + digest.destination.space.slug)}>
							{digest.destination.space.name}
						</a>
					</h2>

					{#if digest.destination.intel.length > 0}
						<h3>Destination intel</h3>
						{#each digest.destination.intel as item (item.id)}
							<a class="intel-card" href={siteUrl('/radar')}>
								<span class="intel-kind">{item.kind === 'visa_route' ? 'Visa route' : 'Job'}</span>
								<span class="intel-title">{item.title}</span>
								{#if item.summary}<span class="intel-summary">{item.summary}</span>{/if}
							</a>
						{/each}
					{/if}

					{#if digest.destination.scam_alerts.length > 0}
						<h3>Scam alerts</h3>
						{#each digest.destination.scam_alerts as alert (alert.id)}
							<a class="scam-card" href={siteUrl('/scam-alerts')}>
								<span class="scam-type">{alert.scam_type.replaceAll('_', ' ')}</span>
								<span class="scam-desc">{alert.description}</span>
								{#if alert.platform_used || alert.approximate_date}
									<span class="scam-meta">
										{[alert.platform_used, alert.approximate_date].filter(Boolean).join(' · ')}
									</span>
								{/if}
							</a>
						{/each}
					{/if}
				</aside>
			{/if}
		</div>
	{:else}
		<p class="error" role="alert">{error ?? 'Could not load your digest.'} <button class="ghost" onclick={() => refresh()}>Retry</button></p>
	{/if}
</div>
