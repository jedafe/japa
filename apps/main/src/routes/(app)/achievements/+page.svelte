<script lang="ts">
	import { untrack } from 'svelte';
	// Ref: japaconnect-engineering/sections/89-achievement-feed.md
	// Achievement Feed list page. Renders milestone cards from the SSR-loaded
	// list (GET /achievements) with a destination/type/sort filter bar. The
	// milestone_type select options mirror the schema-defined enum — no
	// hardcoded data arrays (Rule 16).
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import {
		Plus,
		Trophy,
		Plane,
		Briefcase,
		GraduationCap,
		MapPin,
		Cake,
		BadgeCheck,
		Sparkles
	} from 'lucide-svelte';
	import type { Achievement, AchievementMilestoneType } from '$lib/types';
	import type { PageData } from './$types';
	import type { IconComponent } from '$lib/components/ui/icons';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('achievements.list_title', 'Achievements — JapaConnect'));
	const description = $derived(
		t(
			'achievements.list_description',
			'Relocation milestones and wins from the Nigerian diaspora community.'
		)
	);

	// Local filter state seeded from SSR props; navigating updates the URL so
	// the load re-runs server-side.
	let dest = $state(untrack(() => data.filters.dest));
	let milestone = $state(untrack(() => data.filters.milestone));
	let sort = $state(untrack(() => data.filters.sort));

	// Keep local filter state in sync when navigating between filter states.
	$effect(() => {
		dest = data.filters.dest;
		milestone = data.filters.milestone;
		sort = data.filters.sort;
	});

	const milestoneOptions: { value: AchievementMilestoneType; label: string }[] = [
		{ value: 'visa_approved', label: t('achievements.type_visa_approved', 'Visa approved') },
		{ value: 'job_offer', label: t('achievements.type_job_offer', 'Job offer') },
		{ value: 'university_accepted', label: t('achievements.type_university_accepted', 'University accepted') },
		{ value: 'arrived', label: t('achievements.type_arrived', 'Arrived') },
		{ value: 'one_year', label: t('achievements.type_one_year', 'One year') },
		{ value: 'citizenship', label: t('achievements.type_citizenship', 'Citizenship') },
		{ value: 'other', label: t('achievements.type_other', 'Other') }
	];

	const milestoneIcons: Record<AchievementMilestoneType, IconComponent> = {
		visa_approved: BadgeCheck,
		job_offer: Briefcase,
		university_accepted: GraduationCap,
		arrived: Plane,
		one_year: Cake,
		citizenship: Trophy,
		other: Sparkles
	};

	function milestoneIcon(type: AchievementMilestoneType): IconComponent {
		return milestoneIcons[type] ?? Sparkles;
	}

	function milestoneLabel(type: AchievementMilestoneType): string {
		return milestoneOptions.find((o) => o.value === type)?.label ?? type;
	}

	function applyFilters() {
		const params = new URLSearchParams();
		if (dest) params.set('dest', dest);
		if (milestone) params.set('milestone', milestone);
		params.set('sort', sort || 'latest');
		goto(`/achievements?${params.toString()}`, { keepFocus: true, noScroll: true });
	}

	function clearFilters() {
		dest = '';
		milestone = '';
		sort = 'latest';
		goto('/achievements', { keepFocus: true, noScroll: true });
	}

	const hasFilters = $derived(!!data.filters.dest || !!data.filters.milestone || data.filters.sort !== 'latest');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function buildPageUrl(filters: { dest: string; milestone: string; sort: string }, nextPage: number): string {
		const params = new URLSearchParams();
		if (filters.dest) params.set('dest', filters.dest);
		if (filters.milestone) params.set('milestone', filters.milestone);
		params.set('sort', filters.sort || 'latest');
		params.set('page', String(nextPage));
		return params.toString();
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Trophy size={20} /> {t('achievements.heading', 'Achievements')}
		</h1>
		{#if data.authed}
			<a
				href="/achievements/new"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('achievements.share', 'Share Achievement')}
			</a>
		{:else}
			<a
				href="/login?redirect=/achievements"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('achievements.share', 'Share Achievement')}
			</a>
		{/if}
	</div>

	<!-- Filter bar -->
	<div class="border-b border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3">
		<form
			onsubmit={(e) => {
				e.preventDefault();
				applyFilters();
			}}
			class="flex flex-wrap items-end gap-3"
		>
			<div class="flex flex-col gap-1">
				<label for="filter-dest" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('achievements.filter_destination', 'Destination')}
				</label>
				<select
					id="filter-dest"
					value={dest}
					onchange={(e) => (dest = e.currentTarget.value)}
					class={inputClass}
				>
					<option value="">{t('achievements.filter_all_countries', 'All countries')}</option>
					{#each data.countries as c (c.id)}
						<option value={c.id}>{c.name}</option>
					{/each}
				</select>
			</div>
			<div class="flex flex-col gap-1">
				<label for="filter-milestone" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('achievements.filter_type', 'Milestone')}
				</label>
				<select id="filter-milestone" value={milestone} onchange={(e) => (milestone = e.currentTarget.value)} class={inputClass}>
					<option value="">{t('achievements.filter_all_types', 'All milestones')}</option>
					{#each milestoneOptions as opt (opt.value)}
						<option value={opt.value}>{opt.label}</option>
					{/each}
				</select>
			</div>
			<div class="flex flex-col gap-1">
				<label for="filter-sort" class="text-xs font-medium text-[var(--color-text-subtle)]">
					{t('achievements.filter_sort', 'Sort')}
				</label>
				<select id="filter-sort" value={sort} onchange={(e) => (sort = e.currentTarget.value)} class={inputClass}>
					<option value="latest">{t('achievements.sort_latest', 'Latest')}</option>
					<option value="top">{t('achievements.sort_top', 'Top')}</option>
				</select>
			</div>
			<div class="flex items-center gap-2">
				<button
					type="submit"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-2 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					{t('common.apply', 'Apply')}
				</button>
				{#if hasFilters}
					<button
						type="button"
						onclick={clearFilters}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface)]"
					>
						{t('common.clear', 'Clear')}
					</button>
				{/if}
			</div>
		</form>
	</div>

	<!-- List -->
	<div class="p-3">
		{#if data.loadError}
			<EmptyState
				icon={Trophy}
				title={t('achievements.load_error', 'Could not load achievements')}
				body={t('achievements.load_error_body', 'Please try again later.')}
			/>
		{:else if data.achievements.length === 0}
			<EmptyState
				icon={Trophy}
				title={t('achievements.empty', 'No achievements yet')}
				body={t('achievements.empty_body', 'Be the first to share a relocation milestone with the community.')}
				action={data.authed
					? { label: t('achievements.share', 'Share Achievement'), href: '/achievements/new' }
					: { label: t('achievements.login_to_share', 'Log in to share'), href: '/login?redirect=/achievements' }}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each data.achievements as item (item.id)}
					{@const Icon = milestoneIcon(item.milestone_type as AchievementMilestoneType)}
					{@const isAnon = item.is_anonymous}
					<a
						href={`/achievements/${item.slug}`}
						class="flex flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40"
					>
						<div class="flex items-center gap-2">
							<span class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-primary)]/10 px-2.5 py-1 text-xs font-semibold text-[var(--color-primary)]">
								<Icon size={13} />
								{milestoneLabel(item.milestone_type as AchievementMilestoneType)}
							</span>

						</div>

						<h2 class="line-clamp-2 text-sm font-semibold text-[var(--color-text)]">{item.title}</h2>

						<div class="mt-auto flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--color-text-subtle)]">
							{#if isAnon}
								<span class="inline-flex items-center gap-1">
									<MapPin size={12} />
									{t('achievements.anonymous_from', 'Anonymous member from')}
									{item.origin_space_name ?? '?'} → {item.destination_space_name}
								</span>
							{:else}
								<span class="inline-flex items-center gap-1.5">
									{#if item.author_avatar}
										<img src={item.author_avatar} alt="" class="h-5 w-5 rounded-full object-cover" />
									{:else}
										<span class="grid h-5 w-5 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[10px] font-bold text-[var(--color-primary)]">
											{(item.author_name ?? 'M').charAt(0)}
										</span>
									{/if}
									<span class="font-medium text-[var(--color-text)]">{item.author_name ?? t('achievements.member', 'Member')}</span>
								</span>
								<span>·</span>
								<span class="inline-flex items-center gap-1">
									<MapPin size={12} /> {item.destination_space_name}
								</span>
							{/if}
							<span>·</span>
							<span>{relativeTime(item.created_at, new Date(), i18n.locale)}</span>
						</div>
					</a>
				{/each}
			</div>

			<!-- Pagination (simple page-based; the backend supports page/page_size). -->
			{#if data.achievements.length >= data.pageSize}
				<div class="mt-4 flex justify-center">
					<a
						href={`/achievements?${buildPageUrl(data.filters, (Number(page.url.searchParams.get('page') ?? '1')) + 1)}`}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						{t('common.load_more', 'Load more')}
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>
