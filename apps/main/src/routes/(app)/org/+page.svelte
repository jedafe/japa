<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { enhance } from '$app/forms';
	import { t } from '$lib/i18n';
	import {
		Building2,
		Search,
		Users,
		ShieldX,
		ChevronLeft,
		ChevronRight,
		Loader2,
		GitCompare
	} from 'lucide-svelte';
	import { EmptyState, FollowButton, SearchInput, DirectoryCard, type SearchSuggestion, type CardMetaItem } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import type { PageData, ActionData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('organizations.title', 'Organizations — {site}'));
	const description = $derived(
		t('organizations.description', 'Communities, businesses, and professional groups on {site}.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Organizations',
			description,
			url: page.url.href
		})
	);

	let query = $state(untrack(() => data.q));
	let selectedOrgType = $state(untrack(() => data.orgType));
	// The backend default (param absent) is verified-only — reflect that in the
	// chip state so "Verified" is highlighted on the initial view.
	let selectedVerified = $state(untrack(() => data.verified || 'true'));
	const searchPlaceholder = $derived(t('organizations.search_placeholder', 'Search organizations…'));

	// BUG-022: Org type options come from the API only (Rule 16 — no
	// hardcoded option lists). When /organizations/meta is unavailable the
	// type filter chips are hidden rather than guessing stale values.
	const orgTypes = $derived(
		data.orgMeta?.org_types
			? [{ value: '', label: t('organizations.filter_all', 'All') }, ...data.orgMeta.org_types]
			: []
	);

	// "All" sends verified=all — the backend maps it to no filter. Omitting the
	// param entirely defaults to verified-only (Section 07 directory contract).
	const verifiedOptions = $derived([
		{ value: 'all', label: t('organizations.filter_all', 'All') },
		{ value: 'true', label: t('organizations.verified', 'Verified') },
		{ value: 'false', label: t('organizations.unverified', 'Unverified') }
	]);

	function buildHref(pageNum: number): string {
		const params = new URLSearchParams();
		if (data.q) params.set('q', data.q);
		if (data.orgType) params.set('org_type', data.orgType);
		if (data.verified) params.set('verified', data.verified);
		if (pageNum > 1) params.set('page', String(pageNum));
		const qs = params.toString();
		return qs ? `/org?${qs}` : '/org';
	}

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedOrgType) params.set('org_type', selectedOrgType);
		if (selectedVerified) params.set('verified', selectedVerified);
		const qs = params.toString();
		goto(qs ? `/org?${qs}` : '/org');
	}

	// GET /search returns { results: [{ type, item: { id, url, name|title,
	// excerpt, ... } }] } — api.get already unwraps the outer `data` envelope,
	// and per-type fields live on `item`, not on the result itself.
	async function orgSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{
			results: { type: string; item: { id: string; url: string; name?: string; title?: string; excerpt?: string } }[];
		}>(`/search?q=${encodeURIComponent(q)}&type=organization&limit=8`);
		if (res.error || !res.data) return [];
		return (res.data.results ?? []).map((r) => ({
			id: r.item.id,
			title: r.item.name ?? r.item.title ?? '',
			url: r.item.url,
			excerpt: r.item.excerpt,
			type: r.type
		}));
	}

	// JCE multi-select state
	let savingJCE = $state(false);
	let selectedOrgIds = $state<Set<string>>(new Set());

	const selectedOrgs = $derived(data.orgs.filter((o) => selectedOrgIds.has(o.id)));
	const canCompare = $derived(selectedOrgIds.size >= 2 && selectedOrgIds.size <= 4);

	function toggleOrgSelection(id: string) {
		const next = new Set(selectedOrgIds);
		if (next.has(id)) {
			next.delete(id);
		} else {
			if (next.size >= 4) return;
			next.add(id);
		}
		selectedOrgIds = next;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header + search + filters -->
	<div class="border-b border-[var(--color-border)] px-4 py-4" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Building2 size={20} /> {t('organizations.heading', 'Organizations')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('organizations.subtitle', 'Follow organizations to keep up with their activity.')}
		</p>

		<form class="mt-3" action="/org" method="get" role="search">
			<SearchInput
				bind:value={query}
				placeholder={searchPlaceholder}
				ariaLabel={t('organizations.heading', 'Organizations')}
				fetcher={orgSearchFetcher}
			/>
			{#if selectedOrgType}
				<input type="hidden" name="org_type" value={selectedOrgType} />
			{/if}
			{#if selectedVerified}
				<input type="hidden" name="verified" value={selectedVerified} />
			{/if}
		</form>

		<!-- Org type filter chips — hidden when /organizations/meta is
		     unavailable rather than rendering hardcoded options (Rule 16). -->
		{#if orgTypes.length > 0}
		<div class="mt-3 flex flex-wrap gap-2">
			{#each orgTypes as ot (ot.value)}
				<button
					type="button"
					onclick={() => { selectedOrgType = ot.value; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedOrgType === ot.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{ot.label}
				</button>
			{/each}
		</div>
		{/if}

		<!-- Verified filter chips -->
		<div class="mt-2 flex flex-wrap gap-2">
			{#each verifiedOptions as vo (vo.value)}
				<button
					type="button"
					onclick={() => { selectedVerified = vo.value; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedVerified === vo.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					{vo.label}
				</button>
			{/each}
		</div>
	</div>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if data.forbidden}
			<div class="flex flex-col items-center gap-3 py-16 text-center">
				<ShieldX size={40} class="text-[var(--color-muted)]" />
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('organizations.forbidden', 'You do not have permission to view this directory.')}
				</p>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('organizations.load_error', 'Failed to load organizations. Please try again later.')}
			</p>
		{:else if data.orgs.length === 0}
			<EmptyState icon={Building2} title={t('organizations.empty', 'No organizations yet.')} />
		{:else}
			<form
				method="POST"
				use:enhance={() => {
					savingJCE = true;
					return async ({ update }) => {
						savingJCE = false;
						await update();
					};
				}}
				class="contents"
			>
				{#if selectedOrgIds.size > 0}
					<div
						class="mb-4 flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<p class="text-sm text-[var(--color-muted)]">
							{t('organizations.selected', 'Selected')}: {selectedOrgIds.size} / 4
						</p>
						<button
							type="submit"
							disabled={!canCompare || !data.authed}
							title={data.authed
								? t('organizations.compare_in_jce_hint', 'Compare in JCE')
								: t('organizations.sign_in_to_compare', 'Sign in to compare')}
							class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:cursor-not-allowed disabled:opacity-50"
						>
							{#if savingJCE}
								<Loader2 size={16} class="animate-spin" />
							{:else}
								<GitCompare size={16} />
							{/if}
							{t('organizations.compare_in_jce', 'Compare in JCE')}
						</button>
					</div>
				{/if}

				{#if form?.error}
					<div
						class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
					>
						{form.message || t('organizations.jce_error', 'Could not create comparison')}
					</div>
				{/if}

				<p class="mb-3 text-xs text-[var(--color-muted)]">
					{data.total} {t('organizations.results', 'organizations')}
				</p>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each data.orgs as org (org.id)}
						<DirectoryCard
							href="/org/{org.slug}"
							title={org.name}
							subtitle="{org.org_type.replace(/_/g, ' ')}{org.country_code ? ` · ${org.country_code}` : ''}"
							description={org.tagline}
							fallback={org.name.charAt(0).toUpperCase()}
							hueSeed={org.id}
							verified={org.verified}
							meta={[
								{ icon: Users, text: t('profile.followers', '{n} followers').replace('{n}', String(org.follower_count)) },
								{ icon: Building2, text: t('meta.members', '{n} members').replace('{n}', String(org.member_count)) }
							] satisfies CardMetaItem[]}
							entityType="organization"
							entityId={org.id}
							entitySlug={org.slug}
							entityCount={org.follower_count}
						>
							{#snippet leading()}
								<input
									type="checkbox"
									name="org_id"
									value={org.id}
									checked={selectedOrgIds.has(org.id)}
									onchange={() => toggleOrgSelection(org.id)}
									disabled={selectedOrgIds.size >= 4 && !selectedOrgIds.has(org.id)}
									aria-label={t('organizations.select_for_compare', 'Select for comparison')}
									class="mt-1 h-4 w-4 shrink-0 accent-[var(--color-primary)]"
								/>
								{#if selectedOrgIds.has(org.id)}
									<input type="hidden" name="org_slug" value={org.slug} />
								{/if}
							{/snippet}
							{#snippet actions()}
								{#if page.data.user}
									<FollowButton entityType="organization" entityId={org.id} entitySlug={org.slug} compact />
								{/if}
							{/snippet}
						</DirectoryCard>
					{/each}
				</div>

				<!-- Pagination -->
				{#if data.totalPages > 1}
					<div class="mt-6 flex items-center justify-center gap-4">
						<a
							href={buildHref(data.page - 1)}
							class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page <= 1 ? 'pointer-events-none opacity-40' : ''}"
						>
							<ChevronLeft class="rtl:-scale-x-100" size={16} /> {t('common.prev', 'Prev')}
						</a>
						<span class="text-xs text-[var(--color-muted)]">
							{data.page} / {data.totalPages}
						</span>
						<a
							href={buildHref(data.page + 1)}
							class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page >= data.totalPages ? 'pointer-events-none opacity-40' : ''}"
						>
							{t('common.next', 'Next')} <ChevronRight class="rtl:-scale-x-100" size={16} />
						</a>
					</div>
				{/if}
			</form>
		{/if}
	</div>
</div>
