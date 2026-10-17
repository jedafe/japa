<script lang="ts">
	// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
	// Release-notes style changelog — shipped roadmap items grouped by the
	// month of shipped_at. All data from GET /roadmap/changelog (Rule 16).
	import { page } from '$app/state';
	import { t, i18n, siteName} from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { ArrowLeft, History, PackageOpen, TriangleAlert } from 'lucide-svelte';
	import type { RoadmapChangelogItem } from '$lib/types';

	let { data } = $props();
	const items = $derived(data.items ?? []);

	interface MonthGroup {
		key: string;
		label: string;
		items: RoadmapChangelogItem[];
	}

	function dayLabel(iso: string | null): string {
		if (!iso) return '';
		return new Date(iso).toLocaleDateString(i18n.locale, { day: 'numeric' });
	}

	const groups = $derived.by<MonthGroup[]>(() => {
		const map = new Map<string, RoadmapChangelogItem[]>();
		for (const item of items) {
			const d = new Date(item.shipped_at ?? item.created_at);
			const key = `${d.getFullYear()}-${String(d.getMonth()).padStart(2, '0')}`;
			const list = map.get(key) ?? [];
			list.push(item);
			map.set(key, list);
		}
		return [...map.entries()].map(([key, groupItems]) => {
			const [year, month] = key.split('-').map(Number);
			return {
				key,
				label: new Date(year, month).toLocaleDateString(i18n.locale, {
					year: 'numeric',
					month: 'long'
				}),
				items: groupItems
			};
		});
	});
</script>

<svelte:head>
	<title>{t('roadmap.changelog.title', 'Changelog')} — {siteName()}</title>
	<meta
		name="description"
		content={t('roadmap.changelog.subtitle', 'Everything that has shipped on the roadmap.')}
	/>
	<link rel="canonical" href="{page.url.origin}/roadmap/changelog" />
	<meta property="og:title" content={t('roadmap.changelog.title', 'Changelog')} />
	<meta
		property="og:description"
		content={t('roadmap.changelog.subtitle', 'Everything that has shipped on the roadmap.')}
	/>
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={t('roadmap.changelog.title', 'Changelog')} />
	<meta
		name="twitter:description"
		content={t('roadmap.changelog.subtitle', 'Everything that has shipped on the roadmap.')}
	/>
</svelte:head>

<section class="mx-auto max-w-3xl space-y-6 px-4 py-8">
	<a
		href="/roadmap"
		class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft class="h-4 w-4" />
		{t('roadmap.back_to_board', 'Back to roadmap')}
	</a>

	<header class="flex items-center gap-3">
		<div
			class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]"
		>
			<History class="h-5 w-5" />
		</div>
		<div>
			<h1 class="text-2xl font-semibold text-[var(--color-text)]">
				{t('roadmap.changelog.title', 'Changelog')}
			</h1>
			<p class="text-sm text-[var(--color-muted)]">
				{t('roadmap.changelog.subtitle', 'Everything that has shipped on the roadmap.')}
			</p>
		</div>
	</header>

	{#if data.loadError}
		<EmptyState
			icon={TriangleAlert}
			title={t('roadmap.unavailable', 'Roadmap unavailable')}
			body={t('roadmap.unavailable_body', 'The roadmap could not be loaded. Please try again later.')}
		/>
	{:else if groups.length === 0}
		<EmptyState
			icon={PackageOpen}
			title={t('roadmap.changelog.empty', 'Nothing shipped yet')}
			body={t('roadmap.changelog.empty_body', 'Shipped features will be listed here.')}
			action={{ label: t('roadmap.back_to_board', 'Back to roadmap'), href: '/roadmap' }}
		/>
	{:else}
		{#each groups as group (group.key)}
			<section aria-label={group.label}>
				<h2 class="text-lg font-semibold text-[var(--color-text)]">{group.label}</h2>
				<ul
					class="mt-3 divide-y divide-[var(--color-border)] rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
				>
					{#each group.items as item (item.id)}
						<li class="flex gap-4 p-4">
							<span
								class="w-8 shrink-0 pt-0.5 text-center text-sm font-semibold text-[var(--color-muted)] [font-variant-numeric:tabular-nums]"
							>
								{dayLabel(item.shipped_at ?? item.created_at)}
							</span>
							<div class="min-w-0 flex-1">
								<a
									href="/roadmap/{item.slug}"
									class="text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]"
								>
									{item.title}
								</a>
								{#if item.summary}
									<p class="mt-0.5 text-sm text-[var(--color-text-subtle)]">{item.summary}</p>
								{/if}
								{#if item.category_name}
									<span
										class="mt-1.5 inline-flex items-center gap-1 text-[11px] text-[var(--color-muted)]"
									>
										{#if item.category_color}
											<span
												class="inline-block h-2 w-2 rounded-full"
												style:background-color={item.category_color}
											></span>
										{/if}
										{item.category_name}
									</span>
								{/if}
							</div>
						</li>
					{/each}
				</ul>
			</section>
		{/each}
	{/if}
</section>
