<script lang="ts">
	// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	// Public product roadmap board — Canny-style status columns fed entirely
	// by GET /roadmap/board (Rule 16: categories, columns, and items all come
	// from the API). The flagship item (is_flagship) is pinned in a banner
	// above the grid. Voting is logged-in only; guests get a login CTA.
	import { page } from '$app/state';
	import { t, siteName} from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import VoteButton from '$lib/components/roadmap/VoteButton.svelte';
	import RoadmapCard from '$lib/components/roadmap/RoadmapCard.svelte';
	import StatusSelect from '$lib/components/roadmap/StatusSelect.svelte';
	import { Map as MapIcon, History, Send, Rocket, TriangleAlert, Inbox, Package } from 'lucide-svelte';
	import type { RoadmapBoardItem } from '$lib/types';

	let { data } = $props();
	const board = $derived(data.board);
	const authed = $derived(!!data.user);
	// Inline moderation affordance — gates the status <select> on cards.
	// The backend enforces roadmap.moderate on PATCH /admin/roadmap/items/{id}/status.
	const canModerate = $derived(!!data.user?.permissions?.includes('roadmap.moderate'));

	let sort = $state<'votes' | 'recent'>('votes');

	// The first flagship item across columns renders as the pinned banner and
	// is excluded from the column lists so it isn't duplicated.
	const flagship = $derived(
		board ? (board.columns.flatMap((c) => c.items).find((i) => i.is_flagship) ?? null) : null
	);

	function columnItems(items: RoadmapBoardItem[]): RoadmapBoardItem[] {
		const rest = items.filter((i) => !i.is_flagship);
		const sorted = [...rest];
		if (sort === 'recent') {
			sorted.sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));
		} else {
			sorted.sort((a, b) => b.vote_count - a.vote_count);
		}
		return sorted;
	}

	const totalVisible = $derived(
		board ? board.columns.reduce((n, c) => n + columnItems(c.items).length, 0) : 0
	);
</script>

<svelte:head>
	<title>{t('roadmap.title', 'Product Roadmap')} — {siteName()}</title>
	<meta name="description" content={t('roadmap.subtitle', 'See what we are building and vote on what matters to you.')} />
	<link rel="canonical" href="{page.url.origin}/roadmap" />
	<meta property="og:title" content={t('roadmap.title', 'Product Roadmap')} />
	<meta property="og:description" content={t('roadmap.subtitle', 'See what we are building and vote on what matters to you.')} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={t('roadmap.title', 'Product Roadmap')} />
	<meta name="twitter:description" content={t('roadmap.subtitle', 'See what we are building and vote on what matters to you.')} />
</svelte:head>

<section class="mx-auto max-w-7xl space-y-6 px-4 py-8 sm:px-6 lg:px-10">
	<header class="flex flex-wrap items-start justify-between gap-4">
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]"
			>
				<MapIcon class="h-5 w-5" />
			</div>
			<div>
				<h1 class="text-2xl font-semibold text-[var(--color-text)]">
					{t('roadmap.title', 'Product Roadmap')}
				</h1>
				<p class="text-sm text-[var(--color-muted)]">
					{t('roadmap.subtitle', 'See what we are building and vote on what matters to you.')}
				</p>
			</div>
		</div>
		<div class="flex items-center gap-2">
			<a
				href="/roadmap/changelog"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				<History class="h-4 w-4" />
				{t('roadmap.view_changelog', 'Changelog')}
			</a>
			<a
				href="/roadmap/releases"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				<Package class="h-4 w-4" />
				{t('roadmap.view_releases', 'Releases')}
			</a>
			<a
				href="/roadmap/submit"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90"
			>
				<Send class="h-4 w-4 rtl:-scale-x-100" />
				{t('roadmap.submit_cta', 'Submit a request')}
			</a>
		</div>
	</header>

	{#if data.loadError || !board}
		<EmptyState
			icon={TriangleAlert}
			title={t('roadmap.unavailable', 'Roadmap unavailable')}
			body={t('roadmap.unavailable_body', 'The roadmap could not be loaded. Please try again later.')}
		/>
	{:else}
		{#if flagship}
			<!-- Pinned flagship banner — the headline item of the current wave. -->
			<div
				class="rounded-[var(--radius-card)] border border-[var(--color-primary)]/40 bg-[var(--color-primary)]/5 p-4"
			>
				<div class="flex gap-3">
					<VoteButton item={flagship} {authed} />
					<div class="min-w-0 flex-1">
						<div class="flex flex-wrap items-center gap-2">
							<span
								class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)] px-2 py-0.5 text-[11px] font-semibold text-[var(--color-primary-contrast)]"
							>
								<Rocket size={12} />
								{t('roadmap.flagship', 'Now building')}
							</span>
							{#if flagship.category_name}
								<span class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)]">
									{#if flagship.category_color}
										<span
											class="inline-block h-2 w-2 rounded-full"
											style:background-color={flagship.category_color}
										></span>
									{/if}
									{flagship.category_name}
								</span>
							{/if}
							{#if flagship.target_window}
								<span class="text-xs text-[var(--color-muted)]">{flagship.target_window}</span>
							{/if}
						</div>
						<a
							href="/roadmap/{flagship.slug}"
							class="mt-1 block text-lg font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]"
						>
							{flagship.title}
						</a>
						{#if flagship.summary}
							<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{flagship.summary}</p>
						{/if}
						{#if canModerate}
							<div class="mt-3 flex justify-end">
								<StatusSelect itemId={flagship.id} status={flagship.status} featureFlagKey={flagship.feature_flag_key} />
							</div>
						{/if}
					</div>
				</div>
			</div>
		{/if}

		{#if board.categories.length > 0}
			<!-- Category filter — ?category= links re-run the server load. -->
			<nav class="flex flex-wrap items-center gap-2" aria-label={t('roadmap.filter_label', 'Filter by category')}>
				<a
					href="/roadmap"
					class="rounded-full border px-3 py-1 text-xs font-medium transition-colors {data.category === ''
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				>
					{t('roadmap.filter_all', 'All')}
				</a>
				{#each board.categories as cat (cat.id)}
					<a
						href="/roadmap?category={encodeURIComponent(cat.slug)}"
						class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors {data.category === cat.slug
							? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>
						{#if cat.color}
							<span class="inline-block h-2 w-2 rounded-full" style:background-color={cat.color}></span>
						{/if}
						{cat.name}
					</a>
				{/each}

				<div class="ms-auto">
					<AutocompleteSelect
						value={sort}
						options={[
							{ value: 'votes', label: t('roadmap.sort_votes', 'Top votes') },
							{ value: 'recent', label: t('roadmap.sort_recent', 'Most recent') }
						]}
						onValueChange={(v) => (sort = v as 'votes' | 'recent')}
						compact
						class="w-36"
						ariaLabel={t('roadmap.sort_label', 'Sort items')}
					/>
				</div>
			</nav>
		{/if}

		{#if totalVisible === 0 && !flagship}
			<EmptyState
				icon={Inbox}
				title={t('roadmap.empty', 'No roadmap items yet')}
				body={t('roadmap.empty_body', 'Check back soon — or submit the first request.')}
				action={{ label: t('roadmap.submit_cta', 'Submit a request'), href: '/roadmap/submit' }}
			/>
		{:else}
			<!-- Status columns; horizontal scroll until the container can fit
			     five readable columns. The /roadmap group hides the shell's right
			     sidebar, so the board owns roughly viewport-minus-nav width — xl
			     guarantees ~190px+ per column. Column order + labels come from
			     board.columns (the backend ships the Testing column between In
			     Progress and Recently Shipped); the aria-label mirrors the heading. -->
			<div class="flex gap-4 overflow-x-auto pb-2 xl:grid xl:grid-cols-5 xl:overflow-visible">
				{#each board.columns as column (column.status)}
					{@const items = columnItems(column.items)}
					{@const label = t(`roadmap.col_${column.status}`, column.label)}
					<section class="min-w-[16rem] flex-1 xl:min-w-0" aria-label={label}>
						<header class="mb-3 flex items-center justify-between px-1">
							<h2 class="text-sm font-semibold text-[var(--color-text)]">{label}</h2>
							<span
								class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[11px] font-medium text-[var(--color-muted)] [font-variant-numeric:tabular-nums]"
							>
								{items.length}
							</span>
						</header>
						<div class="space-y-3">
							{#each items as item (item.id)}
								<RoadmapCard {item} {authed} {canModerate} />
							{:else}
								<p
									class="rounded-[var(--radius-card)] border border-dashed border-[var(--color-border)] px-3 py-6 text-center text-xs text-[var(--color-muted)]"
								>
									{t('roadmap.column_empty', 'Nothing here yet')}
								</p>
							{/each}
						</div>
					</section>
				{/each}
			</div>
		{/if}
	{/if}
</section>
