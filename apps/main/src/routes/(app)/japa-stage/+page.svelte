<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
	// Japa Stage discovery hub. SSR member directory from GET /members/by-stage
	// enriched with stats (GET /japa-stage/stats), a GuideSlideOver fed by the
	// japa_stage.guide public setting, and routeSidebar widgets (ADR-058).
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import MessageUserDialog from '$lib/components/profile/MessageUserDialog.svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import RelativeTime from '$lib/components/ui/RelativeTime.svelte';
	import {
		Compass, Users, BadgeCheck, ChevronRight, Search, Filter, Check,
		MapPin, MessageCircle, TrendingUp, MessagesSquare, UserPlus,
		ClipboardList, BookOpen, Send, CheckCircle2, Plane, Home,
		Luggage, RotateCcw, CircleDot, ArrowRight
	} from 'lucide-svelte';
	import {
		EntityHero,
		FollowButton,
		GuideSlideOver,
		ListPageBar,
		Modal,
		SearchInput,
		UrlTabs,
		FilterToggle,
		type GuideContent,
		type SearchSuggestion,
		type UrlTab
	} from '$lib/components/ui';
	import HeroArtJapaStage from '$lib/components/heroes/HeroArtJapaStage.svelte';
	import { formatMetric } from '$lib/components/hub/charts/format';
	import { flagEmoji } from '$lib/utils/flags';
	import { api } from '$lib/api/client';
	import { japaStageColors, japaStageLabel, japaStageOptions, japaStagesForIntent, japaIntentLabel, japaIntentOptions } from '$lib/enumOptions';
	import type { JapaStage, JapaIntent, MemberStage, TrendingContent } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const intentOpts = $derived(japaIntentOptions(t, data.intents));

	// Local filter state, seeded from SSR-passed filters.
	let destFilter = $state(untrack(() => data.filters.dest));
	let intentFilter = $state(untrack(() => data.filters.intent));
	let query = $state(untrack(() => data.filters.q));
	let sort = $state(untrack(() => data.filters.sort));

	$effect(() => {
		destFilter = data.filters.dest;
		intentFilter = data.filters.intent;
		query = data.filters.q;
		sort = data.filters.sort;
	});

	function buildParams(over: Record<string, string> = {}) {
		const params = new URLSearchParams();
		const stage = over.stage ?? data.filters.stage;
		const intent = over.intent ?? intentFilter;
		const dest = over.dest ?? destFilter;
		const q = (over.q ?? query).trim();
		const s = over.sort ?? sort;
		if (stage) params.set('stage', stage);
		if (intent) params.set('intent', intent);
		if (dest) params.set('dest', dest);
		if (q) params.set('q', q);
		if (s && s !== 'recent') params.set('sort', s);
		return params;
	}

	function goWith(params: URLSearchParams) {
		goto(`/japa-stage${params.toString() ? `?${params.toString()}` : ''}`);
	}

	function applyFilters() { goWith(buildParams()); }
	function clearFilters() { goto('/japa-stage'); }
	function selectStage(v: string) {
		goWith(buildParams({ stage: v === 'all' ? '' : v, intent: data.filters.intent, dest: data.filters.dest }));
	}
	function changePage(p: number) {
		const params = buildParams({ intent: data.filters.intent, dest: data.filters.dest });
		params.set('page', String(p));
		goWith(params);
	}

	const title = $derived(t('japa_stage.directory_title', 'Japa Stage — Find Members by Stage'));
	const description = $derived(
		t('japa_stage.directory_description', 'Discover diaspora members by where they are in their japa journey.')
	);

	const hasFilters = $derived(!!data.filters.dest || !!data.filters.intent || !!data.filters.q || data.filters.sort !== 'recent');

	// Stage tabs narrow to the selected journey class — a stage outside the
	// class's applicable set can never match (ADR-122 class-scoped terminals).
	const stageOpts = $derived(
		japaStageOptions(t, japaStagesForIntent(data.stages, (data.filters.intent || intentFilter) as JapaIntent | ''))
	);

	const tabs = $derived<UrlTab[]>([
		{ value: 'all', label: t('japa_stage.all_stages', 'All stages') },
		...stageOpts.map((o) => ({ value: o.value, label: o.label }))
	]);

	// Stage strip icons — presentation-only map (icons are not data).
	const stageIcons: Record<string, typeof Compass> = {
		considering: CircleDot,
		planning: ClipboardList,
		preparing: BookOpen,
		applied: Send,
		approved: CheckCircle2,
		relocated: Plane,
		settled: Home,
		traveled: Luggage,
		returned: RotateCcw
	};
	const stageCount = $derived.by(() => {
		const map = new Map<string, number>();
		for (const s of data.stats?.stages ?? []) map.set(s.stage, s.count);
		return map;
	});

	const sortOptions = $derived<{ value: string; label: string }[]>([
		{ value: 'recent', label: t('japa_stage.sort.recent', 'Recently active') },
		{ value: 'stage_updated', label: t('japa_stage.sort.stage_updated', 'Stage updated') },
		{ value: 'name', label: t('japa_stage.sort.name', 'Name A–Z') }
	]);

	let showFilters = $state(false);
	let sortModalOpen = $state(false);
	let searchModalOpen = $state(false);
	$effect(() => {
		if (hasFilters) showFilters = true;
	});

	// Message dialog — same pattern as the profile page.
	let messageOpen = $state(false);
	let messageRecipient = $state<{ id: string; name: string }>({ id: '', name: '' });
	function openMessage(m: MemberStage) {
		messageRecipient = { id: m.user_id, name: m.display_name ?? m.username ?? '' };
		messageOpen = true;
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const allLabel = t('japa_stage.all_destinations', 'All destinations');
		const queryL = q.toLowerCase().trim();
		const allItem: AutocompleteItem = { id: '', label: allLabel };
		if (queryL === '') {
			return Promise.resolve([allItem, ...(data.countries ?? []).map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }))]);
		}
		const matchesAll = allLabel.toLowerCase().includes(queryL);
		const filtered = (data.countries ?? []).filter(
			(c) => c.name.toLowerCase().includes(queryL) || c.geo_code.toLowerCase().includes(queryL)
		);
		const items = filtered.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(matchesAll ? [allItem, ...items] : items);
	}
	const fetchDestinationCountriesInitial = () => fetchDestinationCountries('');
	const selectedDestinationLabel = $derived(
		destFilter ? (data.countries ?? []).find((c) => c.id === destFilter)?.name ?? null : null
	);

	// Member search typeahead — reuses the public autocomplete endpoint.
	async function memberSearchFetcher(q: string): Promise<SearchSuggestion[]> {
		const res = await api.get<{
			suggestions: { type: string; id: string; url?: string; username?: string; display_name?: string }[];
		}>(`/search/autocomplete?q=${encodeURIComponent(q)}&type=user&limit=8`);
		if (res.error || !res.data) return [];
		return (res.data.suggestions ?? []).map((s) => ({
			id: s.id,
			title: s.display_name || s.username || '',
			url: s.url ? s.url.replace(/^\/users\//, '/u/') : s.username ? `/u/${s.username}` : '/japa-stage',
			excerpt: s.username ? `@${s.username}` : undefined,
			type: s.type
		}));
	}

	// "How it works" slide-over — japa_stage.guide public setting, same
	// inline-edit pattern as /users (users.guide → users_guide).
	let guideOpen = $state(false);
	let guideOverride = $state<GuideContent | null>(null);
	const siteSettings = $derived((page.data.siteSettings ?? {}) as Record<string, unknown>);
	const storedGuide = $derived.by((): Partial<GuideContent> => {
		const raw = siteSettings['japa_stage.guide'];
		if (raw && typeof raw === 'object' && !Array.isArray(raw)) return raw as Partial<GuideContent>;
		return {};
	});
	const effectiveGuide = $derived<GuideContent>({
		title: guideOverride?.title ?? storedGuide.title ?? t('common.how_it_works', 'How it works'),
		body: guideOverride?.body ?? storedGuide.body ?? t('japa_stage.guide_body', ''),
		ctas: guideOverride?.ctas ?? storedGuide.ctas ?? [{ title: t('guide_panel.read_full_guide', 'Read the full guide'), url: '/guides' }]
	});

	async function saveGuide(v: GuideContent): Promise<string | null> {
		const res = await api.put('/admin/settings/general', { japa_stage_guide: v });
		if (res.error) return res.error.message || t('common.error', 'Something went wrong');
		guideOverride = v;
		return null;
	}

	// Trending discussion URLs — same mapping as the backend contentURL().
	function discussionUrl(item: TrendingContent): string {
		if (item.content_type === 'forum_thread') return `/forum/${item.slug}`;
		if (item.content_type === 'blog_post') return `/blog/${item.slug}`;
		return `/content/${item.slug}`;
	}
</script>

<SeoHead title={title} description={description} ogTitle={title} />

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<h1 class="sr-only">{t('japa_stage.heading', 'Japa Stage — Find Members by Stage')}</h1>

	<!-- Header — EntityHero directory panel -->
	<EntityHero
		class="mt-4"
		icon={Compass}
		accent="var(--color-primary)"
		heading={t('japa_stage.heading', 'Japa Stage')}
		description={t('japa_stage.hero_desc', 'Discover members at every step of their relocation journey. Connect. Learn. Grow.')}
		crumbs={[
			{ label: t('common.home', 'Home'), href: '/' },
			{ label: t('japa_stage.heading', 'Japa Stage') }
		]}
		stats={[
			{ icon: Users, value: formatMetric(data.stats?.total ?? data.total), label: t('japa_stage.stats_members', 'Members on their journey') }
		]}
	>
		{#snippet art()}
			<HeroArtJapaStage accent="var(--color-primary)" />
		{/snippet}
		{#snippet actions(c)}
			<div class="grid w-full grid-cols-2 gap-3 sm:flex sm:items-center">
				<a
					href={data.authed ? '/japa-stage/me' : '/login?redirect=/japa-stage/me'}
					class="{c.primary} flex-1 justify-center whitespace-nowrap"
				>
					<Compass size={15} />
					{data.authed ? t('japa_stage.update_my_stage', 'Update My Japa Stage') : t('japa_stage.set_my_stage', 'Set my stage')}
				</a>
				<button type="button" onclick={() => (guideOpen = true)} class="{c.ghost} flex-1 justify-center whitespace-nowrap col-span-2 sm:col-span-1">
					{t('common.how_it_works', 'How it works')}
					<ChevronRight size={14} class="rtl:-scale-x-100" />
				</button>
			</div>
		{/snippet}
	</EntityHero>

	<!-- Stage strip — live counts from GET /japa-stage/stats. -->
	<div class="mt-4 overflow-x-auto">
		<ul class="flex min-w-max items-stretch gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1" role="list">
			{#each stageOpts as o (o.value)}
				{@const Icon = stageIcons[o.value] ?? Compass}
				{@const active = data.filters.stage === o.value}
				{@const color = japaStageColors[o.value as JapaStage]}
				{@const count = stageCount.get(o.value)}
				<li>
					<button
						type="button"
						onclick={() => selectStage(o.value)}
						aria-pressed={active}
						class="flex h-full flex-col items-center gap-1 rounded-[var(--radius)] px-3.5 py-2 text-center transition-colors {active ? 'bg-[var(--color-surface-subtle)]' : 'hover:bg-[var(--color-surface-subtle)]'}"
						style={active ? `box-shadow: inset 0 -2px 0 ${color}` : ''}
					>
						<span class="grid h-7 w-7 place-items-center rounded-full" style="background-color: {color}1a; color: {color};">
							<Icon size={15} />
						</span>
						<span class="text-xs font-medium text-[var(--color-text)]">{o.label}</span>
						{#if count != null}
							<span class="text-[11px] tabular-nums text-[var(--color-muted)]">{formatMetric(count)}</span>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
	</div>

	<ListPageBar>
		<UrlTabs
			{tabs}
			value={data.filters.stage || 'all'}
			onValueChange={(v) => selectStage(v)}
			class="!border-b-0 !bg-transparent"
		/>
		{#snippet actions()}
			<FilterToggle bind:show={showFilters} active={hasFilters} controls="japa-stage-filter-panel" />
			<button
				type="button"
				onclick={() => (sortModalOpen = true)}
				class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{t('japa_stage.sort.label', 'Sort')}
				{#if sort !== 'recent'}
					<span class="ms-0.5 h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
				{/if}
				<Filter size={16} />
			</button>
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

	<!-- Sort picker modal — same pattern as /users. -->
	<Modal bind:open={sortModalOpen} title={t('japa_stage.sort.label', 'Sort')} icon={Filter} size="sm">
		{#snippet children(close)}
			<div role="listbox" aria-label={t('japa_stage.sort.label', 'Sort')} class="-m-1">
				{#each sortOptions as opt (opt.value)}
					<button
						type="button"
						role="option"
						aria-selected={sort === opt.value}
						onclick={() => { sort = opt.value; applyFilters(); close(); }}
						class="flex w-full items-center justify-between gap-2 rounded-[var(--radius)] px-3 py-2.5 text-sm font-medium transition-colors {sort === opt.value
							? 'text-[var(--color-text)]'
							: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
					>
						{opt.label}
						{#if sort === opt.value}
							<Check size={16} class="text-[var(--color-primary)]" />
						{/if}
					</button>
				{/each}
			</div>
		{/snippet}
	</Modal>

	<!-- Search modal — GET form preserving active filters. -->
	<Modal bind:open={searchModalOpen} title={t('common.search', 'Search')} icon={Search} size="sm">
		{#snippet children()}
			<form action="/japa-stage" method="get" role="search" onsubmit={() => (searchModalOpen = false)}>
				<SearchInput
					bind:value={query}
					placeholder={t('japa_stage.search_placeholder', 'Search members, skills, countries…')}
					ariaLabel={t('japa_stage.heading', 'Japa Stage')}
					fetcher={memberSearchFetcher}
				/>
				{#if data.filters.stage}
					<input type="hidden" name="stage" value={data.filters.stage} />
				{/if}
				{#if intentFilter}
					<input type="hidden" name="intent" value={intentFilter} />
				{/if}
				{#if destFilter}
					<input type="hidden" name="dest" value={destFilter} />
				{/if}
				{#if sort !== 'recent'}
					<input type="hidden" name="sort" value={sort} />
				{/if}
			</form>
		{/snippet}
	</Modal>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if showFilters}
		<!-- Filter panel -->
		<div id="japa-stage-filter-panel" class="mb-4 flex flex-wrap items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
			<div class="w-44">
				<label for="intent-filter" class="sr-only">{t('japa_intent.filter', 'Journey type')}</label>
				<AutocompleteSelect
					id="intent-filter"
					value={intentFilter}
					options={[
						{ value: '', label: t('japa_intent.all', 'All journey types') },
						...intentOpts
					]}
					onValueChange={(v) => {
						intentFilter = v;
						applyFilters();
					}}
				/>
			</div>
			<div class="w-52">
				<Autocomplete
					placeholder={t('japa_stage.filter_dest', 'Destination')}
					ariaLabel={t('japa_stage.filter_dest', 'Destination')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={destFilter || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => { destFilter = item.id; applyFilters(); }}
					onClear={() => { destFilter = ''; applyFilters(); }}
				/>
			</div>
			{#if hasFilters}
				<button
					type="button"
					onclick={clearFilters}
					class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
				>
					{t('common.clear', 'Clear')}
				</button>
			{/if}
		</div>
		{/if}

		<!-- Content -->
		{#if data.loadError}
			<EmptyState
				icon={Users}
				title={t('japa_stage.load_error', 'Could not load members')}
				body={t('japa_stage.load_error_body', 'Please try again later.')}
			/>
		{:else if data.members.length === 0}
			<EmptyState
				icon={Users}
				title={t('japa_stage.empty', 'No members found')}
				body={t('japa_stage.empty_body', 'Try adjusting your filters.')}
				action={data.authed
					? { label: t('japa_stage.set_my_stage', 'Set my stage'), href: '/japa-stage/me' }
					: undefined}
			/>
		{:else}
			<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">
				{t('japa_stage.members_heading', 'Members on their Japa Journey')}
				{#if data.total > 0}
					<span class="font-normal text-[var(--color-muted)]">— {t('japa_stage.showing', 'Showing {shown} of {total}').replace('{shown}', String(data.members.length)).replace('{total}', formatMetric(data.total))}</span>
				{/if}
			</h2>
			<ul class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each data.members as m (m.user_id)}
					{@render memberCard(m)}
				{/each}
			</ul>

			{#if data.totalPages > 1}
				<div class="mt-4 flex justify-center">
					<PaginationControls
						page={data.currentPage}
						totalPages={data.totalPages}
						onPageChange={changePage}
					/>
				</div>
			{/if}
		{/if}

		<!-- Bottom CTA -->
		<div class="mt-6 flex flex-col items-start gap-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 sm:flex-row sm:items-center sm:justify-between">
			<div>
				<h2 class="text-base font-semibold text-[var(--color-text)]">{t('japa_stage.cta_title', 'Where are you on your Japa Journey?')}</h2>
				<p class="mt-1 text-sm text-[var(--color-muted)]">{t('japa_stage.cta_body', 'Update your stage and help others find and connect with you.')}</p>
				<ul class="mt-2 space-y-1 text-sm text-[var(--color-text-subtle)]">
					<li class="flex items-center gap-2"><Check size={14} class="text-[var(--color-primary)]" />{t('japa_stage.cta_point_match', 'Get matched with the right people')}</li>
					<li class="flex items-center gap-2"><Check size={14} class="text-[var(--color-primary)]" />{t('japa_stage.cta_point_opps', 'Receive relevant opportunities')}</li>
					<li class="flex items-center gap-2"><Check size={14} class="text-[var(--color-primary)]" />{t('japa_stage.cta_point_progress', 'Track your progress')}</li>
				</ul>
			</div>
			<a
				href={data.authed ? '/japa-stage/me' : '/login?redirect=/japa-stage/me'}
				class="inline-flex shrink-0 items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
			>
				{t('japa_stage.update_my_stage', 'Update My Japa Stage')}
				<ArrowRight size={15} />
			</a>
		</div>
	</div>
</div>

<!-- Right-sidebar widgets (ADR-058 routeSidebar). -->
<SetSidebar>
	{#if (data.stats?.destinations?.length ?? 0) > 0}
		<section class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
				<TrendingUp size={15} class="text-[var(--color-primary)]" />
				{t('japa_stage.trending_destinations', 'Trending destinations')}
			</h2>
			<ul class="space-y-2.5">
				{#each data.stats?.destinations ?? [] as d (d.space_id)}
					{@const maxCount = Math.max(...(data.stats?.destinations ?? []).map((x) => x.count), 1)}
					<li>
						<a href="/japa-stage?dest={d.space_id}" class="group block">
							<span class="flex items-center justify-between gap-2 text-sm">
								<span class="flex min-w-0 items-center gap-2">
									<span aria-hidden="true">{flagEmoji(d.geo_code)}</span>
									<span class="truncate font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{d.name}</span>
								</span>
								<span class="shrink-0 text-xs tabular-nums text-[var(--color-muted)]">{formatMetric(d.count)}</span>
							</span>
							<span class="mt-1 block h-1 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
								<span class="block h-full rounded-full bg-[var(--color-primary)]" style="width: {Math.max(6, Math.round((d.count / maxCount) * 100))}%"></span>
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
	{#if data.discussions.length > 0}
		<section class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
				<MessagesSquare size={15} class="text-[var(--color-primary)]" />
				{t('japa_stage.popular_discussions', 'Popular discussions')}
			</h2>
			<ul class="space-y-3">
				{#each data.discussions as item (item.id)}
					<li>
						<a href={discussionUrl(item)} class="group block">
							<span class="line-clamp-2 text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{item.title}</span>
							<span class="mt-0.5 flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
								<span class="capitalize">{item.content_type.replaceAll('_', ' ')}</span>
								{#if item.view_count > 0}
									· {formatMetric(item.view_count)} {t('japa_stage.views', 'views')}
								{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
			<a href="/forum" class="mt-3 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:underline">
				{t('common.view_all', 'View all')}
				<ArrowRight size={12} />
			</a>
		</section>
	{/if}
</SetSidebar>

{#snippet memberCard(m: MemberStage)}
	{@const color = m.stage ? japaStageColors[m.stage as JapaStage] : undefined}
	<li class="overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-colors hover:bg-[var(--color-surface-subtle)]">
		<!-- Cover -->
		<div class="relative h-16 w-full" style="background: {color ? `linear-gradient(120deg, ${color}26, ${color}0d)` : 'var(--color-surface-subtle)'}">
			{#if m.cover_url}
				<img src={m.cover_url} alt="" class="h-full w-full object-cover" loading="lazy" />
			{/if}
			{#if m.stage}
				<span
					class="absolute right-2 top-2 inline-flex items-center gap-1.5 rounded-full bg-[var(--color-surface)]/90 px-2 py-0.5 text-xs font-medium"
					style={`color: ${color};`}
				>
					<span class="inline-block h-1.5 w-1.5 rounded-full" style={`background-color: ${color};`}></span>
					{japaStageLabel(t, m.stage as JapaStage)}
				</span>
			{/if}
		</div>

		<div class="px-3 pb-3">
			<!-- Avatar + name -->
			<div class="-mt-6 flex items-end gap-2.5">
				<UserHoverCard
					id={m.user_id}
					slug={m.username}
					name={m.display_name ?? m.username ?? ''}
					avatar={m.avatar_url}
					class="shrink-0"
				>
					{#snippet trigger()}
						<span class="grid h-12 w-12 place-items-center overflow-hidden rounded-full border-2 border-[var(--color-surface)] bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
							{#if m.avatar_url}
								<img src={m.avatar_url} alt={m.username} class="h-full w-full object-cover" loading="lazy" />
							{:else}
								<Users size={20} />
							{/if}
						</span>
					{/snippet}
				</UserHoverCard>
				<div class="min-w-0 pb-0.5">
					<a href="/u/{m.username}" class="flex items-center gap-1 truncate text-sm font-semibold text-[var(--color-text)] hover:underline">
						<span class="truncate">{m.display_name ?? `@${m.username}`}</span>
						{#if m.verified}
							<BadgeCheck size={14} class="shrink-0 text-[var(--color-primary)]" aria-label={t('japa_stage.verified', 'Verified')} />
						{/if}
					</a>
					{#if m.display_name && m.username}
						<p class="truncate text-xs text-[var(--color-muted)]">@{m.username}</p>
					{/if}
				</div>
			</div>

			<!-- Destination + bio -->
			{#if m.destination_space_name}
				<p class="mt-2 flex items-center gap-1.5 text-xs text-[var(--color-text-subtle)]">
					<span aria-hidden="true">{flagEmoji(m.destination_space_code)}</span>
					<MapPin size={12} class="shrink-0" />
					{m.destination_space_name}
				</p>
			{/if}
			{#if m.bio}
				<p class="mt-1.5 line-clamp-2 text-xs text-[var(--color-text-subtle)]">{m.bio}</p>
			{/if}

			<!-- Profession + interest chips -->
			{#if m.profession || (m.interests && m.interests.length > 0) || m.intent}
				<div class="mt-2 flex flex-wrap items-center gap-1.5">
					{#if m.profession}
						<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">{m.profession}</span>
					{/if}
					{#each (m.interests ?? []).slice(0, 2) as interest (interest)}
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">{interest}</span>
					{/each}
					{#if (m.interests?.length ?? 0) > 2}
						<span class="text-xs text-[var(--color-muted)]">+{(m.interests?.length ?? 0) - 2}</span>
					{/if}
					{#if m.intent}
						<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
							{japaIntentLabel(t, m.intent as JapaIntent)}
						</span>
					{/if}
				</div>
			{/if}

			<!-- Footer: activity + actions -->
			<div class="mt-3 flex items-center justify-between border-t border-[var(--color-border)] pt-2.5">
				<span class="text-xs text-[var(--color-muted)]">
					{#if m.last_active_at}
						{t('japa_stage.active', 'Active')} <RelativeTime iso={m.last_active_at} />
					{:else if m.updated_at}
						{t('japa_stage.updated', 'Updated')} <RelativeTime iso={m.updated_at} />
					{/if}
				</span>
				{#if data.authed && page.data.user?.id !== m.user_id}
					<span class="flex items-center gap-1">
						<button
							type="button"
							onclick={() => openMessage(m)}
							aria-label={t('japa_stage.message', 'Message')}
							class="grid h-7 w-7 place-items-center rounded-full text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface)] hover:text-[var(--color-primary)]"
						>
							<MessageCircle size={15} />
						</button>
						<FollowButton entityType="user" entityId={m.user_id} compact />
					</span>
				{:else if !data.authed}
					<a href="/login?redirect=/japa-stage" class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:underline">
						<UserPlus size={13} />
						{t('japa_stage.connect', 'Connect')}
					</a>
				{/if}
			</div>
		</div>
	</li>
{/snippet}

<!-- "How it works" slide-over — GuideSlideOver; admins edit inline (japa_stage.guide). -->
<GuideSlideOver
	bind:open={guideOpen}
	title={effectiveGuide.title}
	body={effectiveGuide.body}
	ctas={effectiveGuide.ctas}
	canManage={data.canManage}
	onSave={saveGuide}
/>

<MessageUserDialog bind:open={messageOpen} recipientId={messageRecipient.id} recipientName={messageRecipient.name} authed={data.authed} />
