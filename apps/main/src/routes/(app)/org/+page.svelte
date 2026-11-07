<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { enhance } from '$app/forms';
	import { t } from '$lib/i18n';
	import {
		Building2,
		Check,
		Search,
		Users,
		ShieldX,
		ShieldQuestion,
		ChevronLeft,
		ChevronRight,
		Loader2,
		GitCompare,
		Filter
	} from 'lucide-svelte';
	import { EmptyState, EntityHero, FollowButton, GuideSlideOver, ListPageBar, Modal, SearchInput, UrlTabs, DirectoryCard, type GuideContent, type SearchSuggestion, type CardMetaItem } from '$lib/components/ui';
	import HeroArtOrganizations from '$lib/components/heroes/HeroArtOrganizations.svelte';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import { api } from '$lib/api/client';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form?: ActionData } = $props();

	const title = $derived(t('organizations.title', 'Organizations — {site}'));
	const description = $derived(
		t('organizations.description', 'Communities, businesses, and professional groups on {site}.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: t('organizations.heading', 'Organizations'),
			description,
			url: page.url.href
		})
	);

	let query = $state(untrack(() => data.q));
	let selectedOrgType = $state(untrack(() => data.orgType));
	// The backend default (param absent) is claimed-only — orgs with a real
	// operator behind the page. Reflect that in the chip state so "Claimed"
	// is highlighted on the initial view.
	let selectedVerified = $state(untrack(() => data.verified || 'claimed'));
	const searchPlaceholder = $derived(t('organizations.search_placeholder', 'Search organizations…'));

	// BUG-022: Org type options come from the API only (Rule 16 — no
	// hardcoded option lists). When /organizations/meta is unavailable the
	// type filter dropdown is hidden rather than guessing stale values.
	const orgTypes = $derived(
		data.orgMeta?.org_types
			? [{ value: '', label: t('organizations.filter_all', 'All') }, ...data.orgMeta.org_types]
			: []
	);

	// "All" sends verified=all — the backend maps it to no filter. Omitting
	// the param entirely defaults to claimed-only (orgs with an operator).
	const verifiedOptions = $derived([
		{ value: 'all', label: t('organizations.filter_all', 'All') },
		{ value: 'claimed', label: t('organizations.claimed', 'Claimed') },
		{ value: 'verified', label: t('organizations.verified', 'Verified') },
		{ value: 'unverified', label: t('organizations.unverified', 'Unverified') }
	]);

	// Right-side bar controls — each opens a modal rather than taking bar space.
	let typeModalOpen = $state(false);
	let searchModalOpen = $state(false);

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
	// id → slug registry for every org selected this session — the POST needs
	// a slug per selected id, but the card that supplied it may be on another
	// results page (unmounted) when the form submits.
	let selectedOrgSlugs = $state<Map<string, string>>(new Map());

	const canCompare = $derived(selectedOrgIds.size >= 2 && selectedOrgIds.size <= 4);

	function toggleOrgSelection(id: string, slug: string) {
		const next = new Set(selectedOrgIds);
		const slugs = new Map(selectedOrgSlugs);
		if (next.has(id)) {
			next.delete(id);
			slugs.delete(id);
		} else {
			if (next.size >= 4) return;
			next.add(id);
			slugs.set(id, slug);
		}
		selectedOrgIds = next;
		selectedOrgSlugs = slugs;
	}

	// "How it works" slide-over — markdown panel + CTA repeater persisted to
	// public system settings (organizations.guide) via PUT /admin/settings/general.
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);
	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['organizations.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('organizations.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { organizations_guide: v });
		if (res.error) return res.error.message || t('common.error', 'Something went wrong');
		guideOverride = v;
		return null;
	}
</script>

<SeoHead title={title} description={description} ogTitle={title} jsonLd={jsonLd} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={Building2}
		accent="var(--color-primary)"
		heading={t('organizations.heading', 'Organizations')}
		description={t('organizations.hero_desc', 'Discover communities, businesses, and professional groups — follow the ones building for the diaspora.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('organizations.heading', 'Organizations') }
		]}
		stats={[
			{ icon: Building2, value: formatMetric(data.total), label: t('organizations.stats_orgs', 'Organizations') },
			...(data.orgMeta?.org_types?.length
				? [{ icon: Users, value: formatMetric(data.orgMeta.org_types.length), label: t('organizations.stats_types', 'Org types') }]
				: [])
		]}
	>
		{#snippet art()}
			<HeroArtOrganizations accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
				<a href="#org-results" class="{c.primary} flex-1 justify-center whitespace-nowrap">
					<Search size={15} /> {t('organizations.find', 'Find organizations')}
				</a>
				<a href="/org/new" class="{c.secondary} flex-1 justify-center whitespace-nowrap">
					<Building2 size={15} /> {t('organizations.register', 'Register an organization')}
				</a>
				<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap">
					{t('common.how_it_works', 'How it works')}
					<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>

	<!-- Status tabs on the left; org-type dropdown + search on the right —
	     one ListPageBar row (the /scam-alerts, /spaces pattern). -->
	<ListPageBar>
		<UrlTabs
			tabs={verifiedOptions}
			value={selectedVerified}
			onValueChange={(v) => { selectedVerified = v; onFilterChange(); }}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			{#if orgTypes.length > 0}
				<button
					type="button"
					onclick={() => (typeModalOpen = true)}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('organizations.type', 'Type')}
					{#if selectedOrgType}
						<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
					{/if}
					<Filter size={16} />
				</button>
			{/if}
			<button
				type="button"
				onclick={() => (searchModalOpen = true)}
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('common.search', 'Search')}
				{#if query.trim()}
					<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
				{/if}
				<Search size={16} />
			</button>
		{/snippet}
	</ListPageBar>

	<!-- Org type picker modal — options from /organizations/meta only
	     (hidden when meta is unavailable, per BUG-022/Rule 16). -->
	{#if orgTypes.length > 0}
		<Modal
			bind:open={typeModalOpen}
			title={t('organizations.label_org_type', 'Organization type')}
			icon={Building2}
			size="sm"
		>
			{#snippet children(close)}
				<div role="listbox" aria-label={t('organizations.label_org_type', 'Organization type')} class="-m-1">
					{#each orgTypes as ot (ot.value)}
						<button
							type="button"
							role="option"
							aria-selected={selectedOrgType === ot.value}
							onclick={() => { selectedOrgType = ot.value; onFilterChange(); close(); }}
							class="flex w-full items-center justify-between gap-2 rounded-[var(--radius)] px-3 py-2.5 text-sm font-medium transition-colors {selectedOrgType === ot.value
								? 'text-[var(--color-text)]'
								: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
						>
							{ot.label}
							{#if selectedOrgType === ot.value}
								<Check size={16} class="text-[var(--color-primary)]" />
							{/if}
						</button>
					{/each}
				</div>
			{/snippet}
		</Modal>
	{/if}

	<!-- Search modal — same GET form as the previous inline bar search. -->
	<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
		{#snippet children()}
			<form action="/org" method="get" role="search" onsubmit={() => (searchModalOpen = false)}>
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
		{/snippet}
	</Modal>

	<div id="org-results" class="mx-auto w-full max-w-3xl p-4">
		{#if data.forbidden}
			<div class="flex flex-col items-center gap-3 py-16 text-center">
				<ShieldX size={40} class="text-[var(--color-muted)]" />
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('organizations.forbidden', "Your account doesn't have permission to view this directory — contact an administrator.")}
				</p>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('organizations.load_error', 'Failed to load organizations. Please try again later.')}
			</p>
		{:else if data.orgs.length === 0}
			<EmptyState
				icon={Building2}
				title={data.q || data.orgType || (data.verified && data.verified !== 'claimed')
					? t('organizations.no_match', 'No organizations match your search.')
					: t('organizations.empty', 'No organizations yet.')}
			/>
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

				{#each [...selectedOrgIds] as id (id)}
				{#if !data.orgs.some((o) => o.id === id)}
					<input type="hidden" name="org_id" value={id} />
					<input type="hidden" name="org_slug" value={selectedOrgSlugs.get(id) ?? ''} />
				{/if}
			{/each}
			{#if form?.error}
					<div
						class="mb-4 rounded-[var(--radius)] border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950 dark:text-red-300"
					>
						{form.message || t('organizations.jce_error', 'Could not create comparison')}
					</div>
				{/if}

				<p class="mb-3 text-xs text-[var(--color-muted)]">
					{data.total} {data.total === 1 ? t('organizations.results_one', 'organization') : t('organizations.results', 'organizations')}
				</p>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each data.orgs as org (org.id)}
						<DirectoryCard
							href="/org/{org.slug}"
							title={org.name}
							subtitle="{org.org_type.replace(/_/g, ' ')}{org.country_code ? ` · ${org.country_code}` : ''}"
							description={org.tagline}
							imageUrl={org.logo_url}
							fallback={org.name.charAt(0).toUpperCase()}
							hueSeed={org.id}
							verified={org.verified}
							meta={[
								{ icon: Users, text: (org.follower_count === 1 ? t('profile.followers_one', '{n} follower') : t('profile.followers', '{n} followers')).replace('{n}', String(org.follower_count)) },
								{ icon: Building2, text: (org.member_count === 1 ? t('profile.members_one', '{n} member') : t('profile.members', '{n} members')).replace('{n}', String(org.member_count)) },
								...(org.claimed ? [] : [{ icon: ShieldQuestion, text: t('organizations.unclaimed', 'Unclaimed') }])
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
									onchange={() => toggleOrgSelection(org.id, org.slug)}
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

<!-- "How it works" slide-over — GuideSlideOver; admins edit inline (organizations.guide). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>
