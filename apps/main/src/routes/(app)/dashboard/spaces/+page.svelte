<script lang="ts">
	// Ref: japaconnect-engineering/sections/05-spaces.md (user's joined spaces)
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { EmptyState, UrlTabs, type UrlTab, PaginationControls } from '$lib/components/ui';
	import { UsersRound, Plus, Search } from 'lucide-svelte';
	import type { MeSpace } from '$lib/types';

	let { data } = $props();

	const spaces = $derived(data.spaces);
	const tab = $derived(data.tab);
	const pageNum = $derived(data.page);
	const pageSize = $derived(data.pageSize);

	const tabs = $derived<UrlTab[]>([
		{ value: 'joined', label: t('spaces.tab_joined', 'Joined') },
		{ value: 'moderated', label: t('spaces.tab_moderated', 'Moderated') },
		{ value: 'owned', label: t('spaces.tab_owned', 'Owned') }
	]);

	let query = $state('');
	$effect(() => {
		query = data.q ?? '';
	});

	let searchTimer: ReturnType<typeof setTimeout> | null = null;

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

	const hasMore = $derived(spaces.length === pageSize);
	const showPagination = $derived(pageNum > 1 || hasMore);
</script>

<svelte:head>
	<title>{t('dashboard.nav.spaces')} — {t('app.name', 'JapaConnect')}</title>
</svelte:head>

<section class="space-y-4">
	<header class="space-y-4">
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
		<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
			<UrlTabs {tabs} value={tab} onValueChange={(v) => changeTab(v)} />
			<div class="relative max-w-xs flex-1">
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
		<ul class="space-y-2">
			{#each spaces as s (s.id)}
				<li>
					<a
						href="/spaces/{s.slug}"
						class="flex items-center justify-between gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition hover:border-[var(--color-primary)]/40"
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
