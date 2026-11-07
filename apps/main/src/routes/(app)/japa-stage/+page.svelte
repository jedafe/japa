<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/87-japa-stage-profiles.md
	// Japa Stage discovery hub. SSR member directory from GET /members/by-stage
	// enriched with stats (GET /japa-stage/stats), a GuideSlideOver fed by the
	// japa_stage.guide public setting, and inline trending/popular sections
	// (wide no-right-sidebar route — see gate.isNoRightSidebarRoute).
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import MessageUserDialog from '$lib/components/profile/MessageUserDialog.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PaginationControls from '$lib/components/ui/PaginationControls.svelte';
	import RelativeTime from '$lib/components/ui/RelativeTime.svelte';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { getViewerState, updateViewerState } from '$lib/api/viewerState';
	import {
		Compass, Users, UserPlus, BadgeCheck, Check,
		MapPin, MessageCircle, MessagesSquare, Bookmark,
		ClipboardList, BookOpen, Send, CheckCircle2, Home,
		Luggage, RotateCcw, CircleQuestionMark, Camera, ArrowRight,
		SlidersHorizontal, Lightbulb, CirclePlay, Eye, Earth
	} from 'lucide-svelte';
	import {
		FollowButton,
		GuideSlideOver,
		Modal,
		SearchInput,
		type GuideContent,
		type SearchSuggestion
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
	let genderFilter = $state(untrack(() => data.filters.gender));
	let query = $state(untrack(() => data.filters.q));
	let sort = $state(untrack(() => data.filters.sort));

	$effect(() => {
		destFilter = data.filters.dest;
		intentFilter = data.filters.intent;
		genderFilter = data.filters.gender;
		query = data.filters.q;
		sort = data.filters.sort;
	});

	function buildParams(over: Record<string, string> = {}) {
		const params = new URLSearchParams();
		const stage = over.stage ?? data.filters.stage;
		const intent = over.intent ?? intentFilter;
		const dest = over.dest ?? destFilter;
		const gender = over.gender ?? genderFilter;
		const q = (over.q ?? query).trim();
		const s = over.sort ?? sort;
		if (stage) params.set('stage', stage);
		if (intent) params.set('intent', intent);
		if (dest) params.set('dest', dest);
		if (gender) params.set('gender', gender);
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
		goWith(buildParams({ stage: v === 'all' ? '' : v, intent: data.filters.intent, dest: data.filters.dest, gender: data.filters.gender }));
	}
	function changePage(p: number) {
		const params = buildParams({ intent: data.filters.intent, dest: data.filters.dest, gender: data.filters.gender });
		params.set('page', String(p));
		goWith(params);
	}

	const title = $derived(t('japa_stage.directory_title', 'Japa Stage — Find Members by Stage'));
	const description = $derived(
		t('japa_stage.directory_description', 'Discover diaspora members by where they are in their japa journey.')
	);

	const hasFilters = $derived(!!data.filters.dest || !!data.filters.intent || !!data.filters.gender || !!data.filters.q || data.filters.sort !== 'recent');

	// Stage strip narrows to the selected journey class — a stage outside the
	// class's applicable set can never match (ADR-122 class-scoped terminals).
	const stageOpts = $derived(
		japaStageOptions(t, japaStagesForIntent(data.stages, (data.filters.intent || intentFilter) as JapaIntent | ''))
	);

	// Stage strip icons — presentation-only map (icons are not data).
	const stageIcons: Record<string, typeof Compass> = {
		considering: CircleQuestionMark,
		planning: ClipboardList,
		preparing: BookOpen,
		applied: Send,
		approved: CheckCircle2,
		relocated: Luggage,
		settled: Home,
		traveled: Camera,
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

	const genderOptions = $derived<{ value: string; label: string }[]>([
		{ value: '', label: t('japa_stage.all_genders', 'All genders') },
		{ value: 'male', label: t('settings.profile.gender_male', 'Male') },
		{ value: 'female', label: t('settings.profile.gender_female', 'Female') },
		{ value: 'non_binary', label: t('settings.profile.gender_non_binary', 'Non-binary') },
		{ value: 'other', label: t('settings.profile.gender_other', 'Other') }
	]);

	let filtersModalOpen = $state(false);

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

	// Discussion bookmark state — hydrated from the batched viewer-states
	// endpoint (authed only), optimistic toggle with rollback.
	let bookmarkedIds = $state<Set<string>>(new Set());
	let bookmarkPending = $state<Set<string>>(new Set());

	$effect(() => {
		if (!data.authed) return;
		for (const d of data.discussions) {
			void getViewerState(d.id_2).then((s) => {
				if (s.bookmarked) bookmarkedIds = new Set([...bookmarkedIds, d.id_2]);
			});
		}
	});

	async function toggleDiscussionBookmark(item: TrendingContent) {
		if (!(await requireAuthOrRedirect())) return;
		const id = item.id_2;
		if (bookmarkPending.has(id)) return;
		const was = bookmarkedIds.has(id);
		bookmarkPending = new Set([...bookmarkPending, id]);
		bookmarkedIds = was ? new Set([...bookmarkedIds].filter((x) => x !== id)) : new Set([...bookmarkedIds, id]);
		const res = was ? await api.delete(`/content/${id}/bookmark`) : await api.post(`/content/${id}/bookmark`, {});
		if (res.error) {
			bookmarkedIds = was ? new Set([...bookmarkedIds, id]) : new Set([...bookmarkedIds].filter((x) => x !== id));
		} else {
			updateViewerState(id, { bookmarked: !was });
		}
		bookmarkPending = new Set([...bookmarkPending].filter((x) => x !== id));
	}

	// Hero member-count card — avatar stack from real directory members.
	const heroAvatars = $derived(
		data.members
			.filter((m) => m.avatar_url)
			.slice(0, 4)
	);
	const totalMembers = $derived(data.stats?.total ?? data.total);

	// Member-card chip palette — decorative tints only (mockup's pastel tags).
	const chipPalette = ['#8b5cf6', '#0d9488', '#d97706', '#2563eb', '#db2777', '#16a34a'];
	function chipStyle(i: number) {
		const c = chipPalette[i % chipPalette.length];
		return `background-color: color-mix(in srgb, ${c} 14%, transparent); color: ${c};`;
	}
</script>

<SeoHead title={title} description={description} ogTitle={title} />

<div class="mx-auto w-full max-w-7xl px-4 pb-10 sm:px-6">
	<h1 class="sr-only">{t('japa_stage.heading', 'Japa Stage — Find Members by Stage')}</h1>

	<!-- Hero — headline, feature bullets, CTAs + map/team art and the live
	     member-count card. -->
	<section
		class="japa-hero relative mt-4 overflow-hidden rounded-2xl border border-[var(--color-border)]"
		aria-labelledby="japa-hero-heading"
	>
		<!-- Full-bleed decorative scene (desktop): the dotted world map,
		     flight arc and pins span the whole panel, the team photo
		     anchors the panel's bottom-right, and the member-count card
		     floats over the photo's lower edge — desktop only. -->
		<div class="pointer-events-none absolute inset-0 hidden lg:block" aria-hidden="true">
			<HeroArtJapaStage />
			<div class="absolute bottom-5 flex items-center gap-3 rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 shadow-md" style="right: 3%;">
				{#if heroAvatars.length > 0}
					<span class="flex -space-x-3.5">
						{#each heroAvatars as m (m.user_id)}
							<img
								src={m.avatar_url}
								alt=""
								class="h-8 w-8 rounded-full border-2 border-[var(--color-surface)] object-cover"
								loading="lazy"
							/>
						{/each}
					</span>
				{:else}
					<span class="grid h-8 w-8 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
						<Users size={15} />
					</span>
				{/if}
				<span class="text-xs leading-snug text-[var(--color-text)]">
					<strong class="block font-semibold">{formatMetric(totalMembers)}+ {t('japa_stage.members_word', 'members')}</strong>
					{t('japa_stage.on_their_journey', 'on their Japa journey')}
				</span>
			</div>
		</div>

		<div class="relative px-6 py-9 sm:px-12 sm:py-12 lg:max-w-[60%]">
			<h2 id="japa-hero-heading" class="text-4xl font-bold tracking-tight text-[var(--color-text)] sm:text-5xl lg:text-7xl lg:leading-[1.05]">
				{t('japa_stage.heading', 'Japa Stage')}
			</h2>
			<p class="mt-3 max-w-md text-base leading-relaxed text-[var(--color-text-subtle)]">
				{t('japa_stage.hero_desc', 'Discover members at every step of their relocation journey. Connect. Learn. Grow.')}
			</p>

			<ul class="mt-7 flex flex-wrap gap-x-6 gap-y-3" role="list">
				<li class="flex items-center gap-2.5">
					<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full" style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary);">
						<Users size={17} />
					</span>
					<span class="w-40 shrink-0 text-xs font-medium leading-snug text-[var(--color-text)]">{t('japa_stage.feature_find', 'Find people on the same journey as you')}</span>
				</li>
				<li class="flex items-center gap-2.5">
					<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full" style="background-color: color-mix(in srgb, #16a34a 12%, transparent); color: #16a34a;">
						<Lightbulb size={17} />
					</span>
					<span class="w-40 shrink-0 text-xs font-medium leading-snug text-[var(--color-text)]">{t('japa_stage.feature_learn', 'Learn from real experiences')}</span>
				</li>
				<li class="flex items-center gap-2.5">
					<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full" style="background-color: color-mix(in srgb, #d97706 12%, transparent); color: #d97706;">
						<Send size={17} />
					</span>
					<span class="w-40 shrink-0 text-xs font-medium leading-snug text-[var(--color-text)]">{t('japa_stage.feature_move', 'Get inspired and move forward')}</span>
				</li>
			</ul>

			<div class="mt-7 flex flex-wrap items-center gap-4">
				<a
					href={data.authed ? '/japa-stage/me' : '/login?redirect=/japa-stage/me'}
					class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-3 text-sm font-medium text-[var(--color-primary-contrast)] shadow-sm transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					{data.authed ? t('japa_stage.update_my_stage', 'Update My Japa Stage') : t('japa_stage.set_my_stage', 'Set my stage')}
					<ArrowRight size={15} />
				</a>
				<button
					type="button"
					onclick={() => (guideOpen = true)}
					class="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)] transition-opacity hover:opacity-75"
				>
					<CirclePlay size={17} class="text-[var(--color-primary)]" />
					{t('common.how_it_works', 'How it works')}
				</button>
			</div>
		</div>
	</section>

	<!-- Stage strip — live counts from GET /japa-stage/stats; each tile
	     filters the directory. -->
	<div class="mt-3 overflow-x-auto rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
		<ul class="flex min-w-max items-stretch" role="list">
			{#each stageOpts as o (o.value)}
				{@const Icon = stageIcons[o.value] ?? Compass}
				{@const active = data.filters.stage === o.value}
				{@const color = japaStageColors[o.value as JapaStage]}
				{@const count = stageCount.get(o.value)}
				<li class="flex-1">
					<button
						type="button"
						onclick={() => selectStage(active ? 'all' : o.value)}
						aria-pressed={active}
						class="flex h-full w-full flex-col items-center gap-1.5 rounded-xl px-4 py-3 text-center transition-colors {active ? 'bg-[var(--color-surface-subtle)]' : 'hover:bg-[var(--color-surface-subtle)]'}"
						style={active ? `box-shadow: inset 0 -2px 0 ${color}` : ''}
					>
						<span class="grid h-11 w-11 place-items-center rounded-full" style="background-color: color-mix(in srgb, {color} 14%, transparent); color: {color};">
							<Icon size={18} />
						</span>
						<span class="text-sm font-semibold text-[var(--color-text)]">{o.label}</span>
						<span class="text-xs tabular-nums text-[var(--color-muted)]">{formatMetric(count ?? 0)}</span>
					</button>
				</li>
			{/each}
		</ul>
	</div>

	<!-- Filter bar — search + stage/destination/purpose/gender selects; the
	     "More filters" button opens a modal holding the same controls for
	     narrow viewports. -->
	<div class="mt-4 flex flex-wrap items-center gap-2 rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
		<form action="/japa-stage" method="get" role="search" class="min-w-48 flex-1">
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
			{#if genderFilter}
				<input type="hidden" name="gender" value={genderFilter} />
			{/if}
			{#if sort !== 'recent'}
				<input type="hidden" name="sort" value={sort} />
			{/if}
		</form>

		<div class="hidden w-40 shrink-0 lg:block">
			<AutocompleteSelect
				value={data.filters.stage}
				options={[
					{ value: '', label: t('japa_stage.filter_stages', 'Stages') },
					...stageOpts
				]}
				onValueChange={(v) => selectStage(v)}
				ariaLabel={t('japa_stage.filter_stage', 'Stage')}
				compact
			/>
		</div>
		<div class="hidden w-48 shrink-0 lg:block">
			<Autocomplete
				placeholder={t('japa_stage.all_destinations', 'All destinations')}
				ariaLabel={t('japa_stage.filter_dest', 'Destination')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchDestinationCountries}
				initialFetcher={fetchDestinationCountriesInitial}
				selectedId={destFilter || null}
				selectedLabel={selectedDestinationLabel}
				onSelect={(item) => { destFilter = item.id; applyFilters(); }}
				onClear={() => { destFilter = ''; applyFilters(); }}
				compact
			/>
		</div>
		<div class="hidden w-44 shrink-0 lg:block">
			<AutocompleteSelect
				value={intentFilter}
				options={[
					{ value: '', label: t('japa_stage.all_purposes', 'All purposes') },
					...intentOpts
				]}
				onValueChange={(v) => { intentFilter = v; applyFilters(); }}
				ariaLabel={t('japa_intent.filter', 'Journey type')}
				compact
			/>
		</div>
		<div class="hidden w-40 shrink-0 lg:block">
			<AutocompleteSelect
				value={genderFilter}
				options={genderOptions}
				onValueChange={(v) => { genderFilter = v; applyFilters(); }}
				ariaLabel={t('japa_stage.all_genders', 'All genders')}
				compact
			/>
		</div>

		<button
			type="button"
			onclick={() => (filtersModalOpen = true)}
			class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<SlidersHorizontal size={15} />
			{t('japa_stage.more_filters', 'More filters')}
			{#if hasFilters}
				<span class="h-2 w-2 rounded-full bg-[var(--color-primary)]" aria-hidden="true"></span>
			{/if}
		</button>
	</div>

	<!-- More-filters modal — the full control set for narrow screens, plus
	     sort and clear-all. -->
	<Modal bind:open={filtersModalOpen} title={t('japa_stage.more_filters', 'More filters')} icon={SlidersHorizontal} size="sm">
		{#snippet children(close)}
			<div class="flex flex-col gap-3">
				<div>
					<label for="modal-stage-filter" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('japa_stage.filter_stages', 'Stages')}</label>
					<AutocompleteSelect
						id="modal-stage-filter"
						value={data.filters.stage}
						options={[
							{ value: '', label: t('japa_stage.all_stages', 'All stages') },
							...stageOpts
						]}
						onValueChange={(v) => { selectStage(v); close(); }}
					/>
				</div>
				<div>
					<label for="modal-dest-filter" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('japa_stage.filter_dest', 'Destination')}</label>
					<Autocomplete
						id="modal-dest-filter"
						placeholder={t('japa_stage.all_destinations', 'All destinations')}
						ariaLabel={t('japa_stage.filter_dest', 'Destination')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchDestinationCountries}
						initialFetcher={fetchDestinationCountriesInitial}
						selectedId={destFilter || null}
						selectedLabel={selectedDestinationLabel}
						onSelect={(item) => { destFilter = item.id; applyFilters(); close(); }}
						onClear={() => { destFilter = ''; applyFilters(); }}
					/>
				</div>
				<div>
					<label for="modal-intent-filter" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('japa_stage.all_purposes', 'All purposes')}</label>
					<AutocompleteSelect
						id="modal-intent-filter"
						value={intentFilter}
						options={[
							{ value: '', label: t('japa_stage.all_purposes', 'All purposes') },
							...intentOpts
						]}
						onValueChange={(v) => { intentFilter = v; applyFilters(); close(); }}
					/>
				</div>
				<div>
					<label for="modal-gender-filter" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('japa_stage.all_genders', 'All genders')}</label>
					<AutocompleteSelect
						id="modal-gender-filter"
						value={genderFilter}
						options={genderOptions}
						onValueChange={(v) => { genderFilter = v; applyFilters(); close(); }}
					/>
				</div>
				<div>
					<label for="modal-sort" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('japa_stage.sort_by', 'Sort by')}</label>
					<AutocompleteSelect
						id="modal-sort"
						value={sort}
						options={sortOptions}
						onValueChange={(v) => { sort = v; applyFilters(); close(); }}
					/>
				</div>
				{#if hasFilters}
					<button
						type="button"
						onclick={() => { clearFilters(); close(); }}
						class="mt-1 self-start text-xs font-medium text-[var(--color-muted)] hover:text-[var(--color-text)]"
					>
						{t('common.clear', 'Clear')}
					</button>
				{/if}
			</div>
		{/snippet}
	</Modal>

	<!-- Members -->
	{#if data.loadError}
		<div class="mt-8">
			<EmptyState
				icon={Users}
				title={t('japa_stage.load_error', 'Could not load members')}
				body={t('japa_stage.load_error_body', 'Please try again later.')}
			/>
		</div>
	{:else if data.members.length === 0}
		<div class="mt-8">
			<EmptyState
				icon={Users}
				title={t('japa_stage.empty', 'No members found')}
				body={t('japa_stage.empty_body', 'Try adjusting your filters.')}
				action={data.authed
					? { label: t('japa_stage.set_my_stage', 'Set my stage'), href: '/japa-stage/me' }
					: undefined}
			/>
		</div>
	{:else}
		<div class="mt-8 flex flex-wrap items-end justify-between gap-3">
			<div>
				<h2 class="text-lg font-semibold text-[var(--color-text)]">
					{t('japa_stage.members_heading', 'Members on their Japa Journey')}
				</h2>
				{#if data.total > 0}
					<p class="mt-0.5 text-sm text-[var(--color-muted)]">
						{t('japa_stage.showing', 'Showing {shown} of {total}').replace('{shown}', String(data.members.length)).replace('{total}', formatMetric(data.total))} {t('japa_stage.members_word', 'members')}
					</p>
				{/if}
			</div>
			<div class="flex items-center gap-2">
				<span class="text-sm text-[var(--color-muted)]">{t('japa_stage.sort_by', 'Sort by')}</span>
				<div class="w-44">
					<AutocompleteSelect
						value={sort}
						options={sortOptions}
						onValueChange={(v) => { sort = v; applyFilters(); }}
						ariaLabel={t('japa_stage.sort_by', 'Sort by')}
						compact
					/>
				</div>
			</div>
		</div>

		<ul class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each data.members as m (m.user_id)}
				{@render memberCard(m)}
			{/each}
		</ul>

		{#if data.totalPages > 1}
			<div class="mt-6 flex justify-center">
				<PaginationControls
					page={data.currentPage}
					totalPages={data.totalPages}
					onPageChange={changePage}
				/>
			</div>
		{/if}

		<div class="mt-6 flex justify-center">
			<a href="/users" class="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-primary)] hover:underline">
				{t('japa_stage.view_all_members', 'View all members')}
				<ArrowRight size={14} class="rtl:-scale-x-100" />
			</a>
		</div>
	{/if}

	<!-- Trending destinations + popular discussions — rendered inline on
	     this wide route (was: right-sidebar widgets). -->
	{#if (data.stats?.destinations?.length ?? 0) > 0 || data.discussions.length > 0}
		<div class="mt-10 grid gap-4 lg:grid-cols-2">
			{#if (data.stats?.destinations?.length ?? 0) > 0}
				{@const maxCount = Math.max(...(data.stats?.destinations ?? []).map((x) => x.count), 1)}
				<section class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 {data.discussions.length === 0 ? 'lg:col-span-2' : ''}">
					<div class="mb-4 flex items-center justify-between">
						<h2 class="text-base font-semibold text-[var(--color-text)]">{t('japa_stage.trending_destinations', 'Trending destinations')}</h2>
						<a href="/radar?tab=countries" class="text-sm font-medium text-[var(--color-primary)] hover:underline">{t('common.view_all', 'View all')}</a>
					</div>
					<ul class="space-y-4">
						{#each data.stats?.destinations ?? [] as d (d.space_id)}
							<li>
								<a href="/japa-stage?dest={d.space_id}" class="group flex items-center gap-3">
									<span class="w-7 shrink-0 text-xl leading-none" aria-hidden="true">{flagEmoji(d.geo_code)}</span>
									<span class="w-32 shrink-0 truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{d.name}</span>
									<span class="h-2 min-w-0 flex-1 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
										<span
											class="block h-full rounded-full"
											style="width: {Math.max(6, Math.round((d.count / maxCount) * 100))}%; background: linear-gradient(90deg, color-mix(in srgb, var(--color-primary) 55%, transparent), var(--color-primary));"
										></span>
									</span>
									<span class="w-10 shrink-0 text-right text-sm tabular-nums text-[var(--color-muted)]">{formatMetric(d.count)}</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
			{#if data.discussions.length > 0}
				<section class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 {(data.stats?.destinations?.length ?? 0) === 0 ? 'lg:col-span-2' : ''}">
					<div class="mb-4 flex items-center justify-between">
						<h2 class="text-base font-semibold text-[var(--color-text)]">{t('japa_stage.popular_discussions', 'Popular discussions')}</h2>
						<a href="/forum" class="text-sm font-medium text-[var(--color-primary)] hover:underline">{t('common.view_all', 'View all')}</a>
					</div>
					<ul class="divide-y divide-[var(--color-border)]">
						{#each data.discussions.slice(0, 5) as item (item.id)}
							<li class="flex items-center gap-3 py-3 first:pt-0 last:pb-0">
								<a href={discussionUrl(item)} class="group flex min-w-0 flex-1 items-center gap-3">
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
										<MessagesSquare size={17} />
									</span>
									<span class="min-w-0">
										<span class="line-clamp-1 text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{item.title}</span>
										<span class="mt-1 flex items-center gap-2 text-xs text-[var(--color-muted)]">
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 font-medium capitalize text-[var(--color-text-subtle)]">{item.content_type.replaceAll('_', ' ')}</span>
											{#if item.view_count > 0}
												<span class="inline-flex items-center gap-1"><Eye size={12} /> {formatMetric(item.view_count)}</span>
											{/if}
										</span>
									</span>
								</a>
								<button
									type="button"
									onclick={() => toggleDiscussionBookmark(item)}
									aria-label={t('feed.card.bookmark', 'Bookmark')}
									aria-pressed={bookmarkedIds.has(item.id_2)}
									disabled={bookmarkPending.has(item.id_2)}
									class="grid h-8 w-8 shrink-0 place-items-center rounded-full text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)] disabled:opacity-50 {bookmarkedIds.has(item.id_2) ? 'text-[var(--color-primary)]' : ''}"
								>
									<Bookmark size={16} fill={bookmarkedIds.has(item.id_2) ? 'currentColor' : 'none'} />
								</button>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		</div>
	{/if}

	<!-- Bottom CTA -->
	<div class="japa-cta mt-10 grid items-center gap-6 overflow-hidden rounded-2xl border border-[var(--color-border)] p-6 sm:p-8 lg:grid-cols-[auto_minmax(0,1fr)_auto]">
		<div class="relative hidden h-28 w-32 sm:block" aria-hidden="true">
			<span class="grid h-24 w-24 place-items-center rounded-full text-[var(--color-primary-contrast)] shadow-md" style="background: linear-gradient(135deg, var(--color-primary), color-mix(in srgb, var(--color-primary) 60%, #9333ea));">
				<Earth size={44} strokeWidth={1.4} />
			</span>
			<span class="absolute bottom-0 right-2 grid h-11 w-11 place-items-center rounded-2xl bg-[var(--color-surface)] text-[var(--color-primary)] shadow-md">
				<Luggage size={20} />
			</span>
			<span class="absolute left-1 top-0 grid h-7 w-7 place-items-center rounded-full bg-[var(--color-surface)] text-[#e11d48] shadow-sm">
				<MapPin size={14} />
			</span>
		</div>
		<div class="min-w-0">
			<h2 class="text-xl font-semibold text-[var(--color-text)]">{t('japa_stage.cta_title', 'Where are you on your Japa Journey?')}</h2>
			<p class="mt-1.5 text-sm text-[var(--color-muted)]">{t('japa_stage.cta_body', 'Update your stage and help others find and connect with you.')}</p>
			<a
				href={data.authed ? '/japa-stage/me' : '/login?redirect=/japa-stage/me'}
				class="mt-4 inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				{t('japa_stage.update_my_stage', 'Update My Japa Stage')}
				<ArrowRight size={15} />
			</a>
		</div>
		<ul class="space-y-2.5 text-sm text-[var(--color-text-subtle)]">
			<li class="flex items-center gap-2.5">
				<span class="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/12 text-[var(--color-primary)]"><Check size={12} /></span>
				{t('japa_stage.cta_point_match', 'Get matched with the right people')}
			</li>
			<li class="flex items-center gap-2.5">
				<span class="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/12 text-[var(--color-primary)]"><Check size={12} /></span>
				{t('japa_stage.cta_point_opps', 'Receive relevant opportunities')}
			</li>
			<li class="flex items-center gap-2.5">
				<span class="grid h-5 w-5 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/12 text-[var(--color-primary)]"><Check size={12} /></span>
				{t('japa_stage.cta_point_progress', 'Track your progress')}
			</li>
		</ul>
	</div>
</div>

{#snippet memberCard(m: MemberStage)}
	{@const color = m.stage ? japaStageColors[m.stage as JapaStage] : undefined}
	<li class="overflow-hidden rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] transition-shadow hover:shadow-md">
		<!-- Cover -->
		<div class="relative h-24 w-full" style="background: {color ? `linear-gradient(120deg, color-mix(in srgb, ${color} 24%, transparent), color-mix(in srgb, ${color} 8%, transparent))` : 'var(--color-surface-subtle)'}">
			{#if m.cover_url}
				<img src={m.cover_url} alt="" class="h-full w-full object-cover" loading="lazy" />
			{/if}
			{#if m.stage}
				<span
					class="absolute right-2.5 top-2.5 inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium backdrop-blur-sm"
					style="color: {color}; background-color: color-mix(in srgb, {color} 14%, var(--color-surface));"
				>
					{japaStageLabel(t, m.stage as JapaStage)}
				</span>
			{/if}
		</div>

		<div class="px-4 pb-3.5">
			<!-- Avatar overlapping the cover; name + location stacked below it. -->
			<UserHoverCard
				id={m.user_id}
				slug={m.username}
				name={m.display_name ?? m.username ?? ''}
				avatar={m.avatar_url}
				class="-mt-8 block w-fit"
			>
				{#snippet trigger()}
					<span class="grid h-16 w-16 place-items-center overflow-hidden rounded-full border-[3px] border-[var(--color-surface)] bg-[var(--color-surface-subtle)] text-[var(--color-muted)] shadow-sm">
						{#if m.avatar_url}
							<img src={m.avatar_url} alt={m.username} class="h-full w-full object-cover" loading="lazy" />
						{:else}
							<Users size={26} />
						{/if}
					</span>
				{/snippet}
			</UserHoverCard>
			<a href="/u/{m.username}" class="mt-2 flex items-center gap-1.5 truncate text-base font-semibold text-[var(--color-text)] hover:underline">
				<span class="truncate">{m.display_name ?? `@${m.username}`}</span>
				{#if m.verified}
					<BadgeCheck size={16} class="shrink-0 text-[var(--color-primary)]" aria-label={t('japa_stage.verified', 'Verified')} />
				{/if}
			</a>
			{#if m.destination_space_name}
				<p class="mt-1 flex items-center gap-1.5 truncate text-sm text-[var(--color-muted)]">
					<span aria-hidden="true">{flagEmoji(m.destination_space_code)}</span>
					<span class="truncate">{m.destination_space_name}</span>
				</p>
			{:else if m.display_name && m.username}
				<p class="mt-1 truncate text-sm text-[var(--color-muted)]">@{m.username}</p>
			{/if}

			<!-- Bio -->
			{#if m.bio}
				<p class="mt-2.5 line-clamp-2 text-sm leading-relaxed text-[var(--color-text-subtle)]">{m.bio}</p>
			{/if}

			<!-- Profession + interest chips -->
			{#if m.profession || (m.interests && m.interests.length > 0)}
				<div class="mt-2.5 flex flex-wrap items-center gap-1.5">
					{#if m.profession}
						<span class="rounded-full px-2 py-0.5 text-xs font-medium" style={chipStyle(0)}>{m.profession}</span>
					{/if}
					{#each (m.interests ?? []).slice(0, 2) as interest, i (interest)}
						<span class="rounded-full px-2 py-0.5 text-xs font-medium" style={chipStyle(i + 1)}>{interest}</span>
					{/each}
					{#if (m.interests?.length ?? 0) > 2}
						<span class="text-xs text-[var(--color-muted)]">+{(m.interests?.length ?? 0) - 2}</span>
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
					<span class="flex items-center gap-1.5">
						<button
							type="button"
							onclick={() => openMessage(m)}
							aria-label={t('japa_stage.message', 'Message')}
							title={t('japa_stage.message', 'Message')}
							class="grid h-8 w-8 place-items-center rounded-full border border-[var(--color-border)] text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
						>
							<MessageCircle size={15} />
						</button>
						<FollowButton entityType="user" entityId={m.user_id} iconOnly />
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

<style>
	.japa-hero {
		background:
			radial-gradient(
				75% 100% at 88% 0%,
				color-mix(in srgb, var(--color-primary) 9%, transparent),
				transparent 65%
			),
			linear-gradient(
				115deg,
				var(--color-surface) 0%,
				color-mix(in srgb, var(--color-primary) 3%, var(--color-surface)) 55%,
				color-mix(in srgb, var(--color-primary) 6%, var(--color-surface)) 100%
			);
	}

	.japa-cta {
		background:
			radial-gradient(
				90% 160% at 100% 0%,
				color-mix(in srgb, var(--color-primary) 10%, transparent),
				transparent 60%
			),
			linear-gradient(
				115deg,
				color-mix(in srgb, var(--color-primary) 7%, var(--color-surface)) 0%,
				color-mix(in srgb, var(--color-primary) 4%, var(--color-surface)) 100%
			);
	}
</style>
