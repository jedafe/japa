<script lang="ts">
	// Ref: japaconnect-engineering/sections/05-spaces.md (user's joined spaces)
	import { page } from '$app/state';
	import { goto, invalidate } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { EmptyState, UrlTabs, type UrlTab, PaginationControls } from '$lib/components/ui';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import { UsersRound, Plus, Search, Volume2 } from 'lucide-svelte';
	import type { MeSpace } from '$lib/types';

	let { data } = $props();

	const spaces = $derived(data.spaces);
	const tab = $derived(data.tab);
	const pageNum = $derived(data.page);
	const pageSize = $derived(data.pageSize);
	const spaceTypes = $derived(data.spaceTypes ?? []);
	const levelFilter = $derived(data.level ?? '');
	const spaceTypeFilter = $derived(data.spaceType ?? '');
	// level + space_type filters only apply to the joined/muted endpoints.
	const showSpaceFilters = $derived(tab === 'joined' || tab === 'muted');

	const spaceLevelOptions = $derived([
		{ value: '', label: t('spaces.all_levels', 'All levels') },
		{ value: 'country', label: t('spaces.level_country', 'Country') },
		{ value: 'state', label: t('spaces.level_state', 'State') },
		{ value: 'city', label: t('spaces.level_city', 'City') },
		{ value: 'custom', label: t('spaces.level_custom', 'Community') },
		{ value: 'global', label: t('spaces.level_global', 'Global') }
	]);

	const pageSizeOptions = [
		{ value: '10', label: '10' },
		{ value: '25', label: '25' },
		{ value: '50', label: '50' }
	];

	const tabs = $derived<UrlTab[]>([
		{ value: 'joined', label: t('spaces.tab_joined', 'Joined') },
		{ value: 'moderated', label: t('spaces.tab_moderated', 'Moderated') },
		{ value: 'owned', label: t('spaces.tab_owned', 'Owned') },
		{ value: 'muted', label: t('spaces.tab_muted', 'Muted') }
	]);

	let query = $state('');
	$effect(() => {
		query = data.q ?? '';
	});

	let searchTimer: ReturnType<typeof setTimeout> | null = null;
	let unmutingSlug = $state<string | null>(null);

	const emptyState = $derived(() => {
		const copy = {
			joined: {
				title: t('spaces.empty_title_joined', 'No spaces joined'),
				body: t('spaces.empty_body_joined', 'Join a space to start participating in discussions.'),
				action: { label: t('spaces.browse', 'Browse spaces'), href: '/spaces' }
			},
			moderated: {
				title: t('spaces.empty_title_moderated', 'No spaces moderated'),
				body: t('spaces.empty_body_moderated', 'You are not moderating any spaces.'),
				action: { label: t('spaces.browse', 'Browse spaces'), href: '/spaces' }
			},
			owned: {
				title: t('spaces.empty_title_owned', 'No spaces created'),
				body: t('spaces.empty_body_owned', 'Create a space to build a community.'),
				action: { label: t('spaces.create_button', 'Create space'), href: '/spaces' }
			},
			muted: {
				title: t('spaces.empty_title_muted', 'No spaces muted'),
				body: t('spaces.empty_body_muted', 'Mute a space to stop seeing its posts in your feeds.'),
				action: { label: t('spaces.browse', 'Browse spaces'), href: '/spaces' }
			}
		}[tab];
		return copy;
	});

	function buildURL(overrides: Record<string, string | null | undefined>) {
		const url = new URL(page.url);
		for (const [key, value] of Object.entries(overrides)) {
			if (value === null || value === undefined || value === '') {
				url.searchParams.delete(key);
			} else {
				url.searchParams.set(key, value);
			}
		}
		return url.toString();
	}

	function changeTab(next: string) {
		goto(buildURL({ tab: next, page: '1', q: query.trim() || undefined }));
	}

	function changePage(next: number) {
		goto(buildURL({ page: String(next), q: query.trim() || undefined }));
	}

	function onSearchInput() {
		if (searchTimer) clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			goto(buildURL({ q: query.trim() || undefined, page: '1' }));
		}, 250);
	}

	async function unmuteSpace(slug: string) {
		unmutingSlug = slug;
		const res = await api.delete<{ muted: boolean }>(`/spaces/${slug}/mute`);
		unmutingSlug = null;
		if (res.error) return;
		await invalidate('/api/v1/me/muted-spaces');
	}

	const hasMore = $derived(data.hasMore ?? false);
	const showPagination = $derived(pageNum > 1 || hasMore);
</script>

<svelte:head>
	<title>{t('dashboard.nav.spaces')} — {t('app.name', '{site}')}</title>
</svelte:head>

<section class="space-y-4">
	<header class="space-y-3">
		<div class="flex items-center justify-between">
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t('dashboard.nav.spaces')}</h1>
			<a
				href="/spaces"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<Plus class="h-4 w-4" />
				{t('spaces.browse')}
			</a>
		</div>
		<UrlTabs {tabs} value={tab} onValueChange={(v) => changeTab(v)} />
		<div class="flex flex-wrap items-center gap-2">
			{#if showSpaceFilters}
				<AutocompleteSelect
					class="w-44"
					value={spaceTypeFilter}
					options={[
						{
							value: '',
							label: t('spaces.all_types', 'All types')
						},
						...spaceTypes.map((st) => ({ value: st.id, label: st.name }))
					]}
					ariaLabel={t('spaces.filter_type', 'Space type')}
					onValueChange={(v) => goto(buildURL({ space_type: v || null, page: null }))}
				/>
				<AutocompleteSelect
					class="w-36"
					value={levelFilter}
					options={spaceLevelOptions}
					ariaLabel={t('spaces.filter_level', 'Space level')}
					onValueChange={(v) => goto(buildURL({ level: v || null, page: null }))}
				/>
			{/if}
			<AutocompleteSelect
				class="w-20"
				value={String(pageSize)}
				options={pageSizeOptions}
				ariaLabel={t('spaces.page_size', 'Rows per page')}
				onValueChange={(v) => goto(buildURL({ page_size: v, page: null }))}
			/>
			<div class="relative min-w-40 flex-1 sm:ml-auto sm:max-w-xs">
				<Search
					size={16}
					class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--color-muted)]"
				/>
				<input
					type="search"
					name="q"
					bind:value={query}
					oninput={onSearchInput}
					placeholder={t('spaces.search_placeholder', 'Search spaces…')}
					aria-label={t('spaces.search_placeholder', 'Search spaces…')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-9 pr-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
			</div>
		</div>
	</header>

	{#if spaces.length === 0}
		<EmptyState
			icon={UsersRound}
			title={emptyState().title}
			body={emptyState().body}
			action={emptyState().action}
		/>
	{:else}
		<ul class="grid grid-cols-1 gap-2 md:grid-cols-2">
			{#each spaces as s (s.id)}
				<li class="h-full">
					{#if tab === 'muted'}
						<div
							class="flex h-full items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
						>
							<a
								href="/spaces/{s.slug}"
								class="flex h-full min-w-0 flex-1 flex-col justify-center transition hover:text-[var(--color-primary)]"
							>
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{s.name}</p>
								<p class="mt-0.5 text-xs text-[var(--color-muted)]">/{s.slug}</p>
							</a>
							<div class="flex items-center gap-3">
								{#if s.is_primary}
									<span class="shrink-0 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">
										{t('spaces.primary')}
									</span>
								{/if}
								<button
									type="button"
									disabled={unmutingSlug === s.slug}
									onclick={() => unmuteSpace(s.slug)}
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
								>
									<Volume2 class="h-3.5 w-3.5" />
									{t('spaces.unmute', 'Unmute')}
								</button>
							</div>
						</div>
					{:else if tab === 'owned' && s.status === 'suspended'}
						<div
							class="flex h-full items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
						>
							<a href="/spaces/{s.slug}" class="min-w-0 flex-1 transition hover:text-[var(--color-primary)]">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{s.name}</p>
								<p class="mt-0.5 text-xs text-[var(--color-muted)]">/{s.slug} · {t('spaces.status_suspended', 'Suspended')}</p>
							</a>
							<AppealButton entityType="space" entityId={s.id} />
						</div>
					{:else}
						<a
							href="/spaces/{s.slug}"
							class="flex h-full items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
						>
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{s.name}</p>
								<p class="mt-0.5 text-xs text-[var(--color-muted)]">/{s.slug}</p>
							</div>
							{#if s.is_primary}
								<span class="shrink-0 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">
									{t('spaces.primary')}
								</span>
							{/if}
						</a>
					{/if}
				</li>
			{/each}
		</ul>
		{#if showPagination}
			<div class="flex justify-center pt-2">
				<PaginationControls page={pageNum} {hasMore} onPageChange={changePage} />
			</div>
		{/if}
	{/if}
</section>
