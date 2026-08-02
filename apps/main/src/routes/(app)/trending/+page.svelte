<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Select, Tabs } from 'bits-ui';
	import {
		Flame,
		Hash,
		Layers,
		Users,
		Eye,
		ArrowUp,
		Building2
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';
	import type { TrendingWindow } from '$lib/types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('trending.title', 'Trending — JapaConnect'));
	const description = $derived(
		t(
			'trending.description',
			'The hottest content, tags, spaces, and people on JapaConnect right now.'
		)
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Trending',
			description,
			url: page.url.href
		})
	);

	const tabs = $derived([
		{ id: 'content', label: t('trending.tab_content', 'Content'), icon: Flame },
		{ id: 'tags', label: t('trending.tab_tags', 'Tags'), icon: Hash },
		{ id: 'spaces', label: t('trending.tab_spaces', 'Spaces'), icon: Layers },
		{ id: 'users', label: t('trending.tab_users', 'People'), icon: Users }
	]);

	const windows: { value: TrendingWindow; label: string }[] = [
		{ value: '24h', label: t('trending.window_24h', '24 hours') },
		{ value: '7d', label: t('trending.window_7d', '7 days') },
		{ value: '30d', label: t('trending.window_30d', '30 days') }
	];

	// Round the trending score for display (Rule: round every displayed number).
	const score = (n: number) => Math.round(n);

	function tabHref(tabId: string): string {
		const params = new URLSearchParams(page.url.searchParams);
		params.set('tab', tabId);
		return `?${params.toString()}`;
	}

	function windowHref(w: TrendingWindow): string {
		const params = new URLSearchParams(page.url.searchParams);
		params.set('window', w);
		return `?${params.toString()}`;
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
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header: title + window selector -->
	<div class="flex items-center justify-between gap-3 border-b border-[var(--color-border)] px-4 py-4">
		<div class="min-w-0">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<Flame size={20} /> {t('trending.heading', 'Trending')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
				{t('trending.subtitle', 'What the community is engaging with.')}
			</p>
		</div>

		<!-- Window selector (24h / 7d / 30d) -->
		<Select.Root
			type="single"
			value={data.window}
			onValueChange={(v) => {
				const w = v as TrendingWindow;
				if (w) window.location.href = windowHref(w);
			}}
		>
			<Select.Trigger
				aria-label={t('trending.window_label', 'Time window')}
				class="flex shrink-0 items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
			>
				{windows.find((w) => w.value === data.window)?.label ?? data.window}
			</Select.Trigger>
			<Select.Portal>
				<Select.Content
					class="z-50 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
				>
					{#each windows as w (w.value)}
						<Select.Item
							value={w.value}
							class="flex cursor-pointer items-center rounded-[6px] px-3 py-2 text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)]"
						>
							{#snippet child({ props })}
								<div {...props}>{w.label}</div>
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Content>
			</Select.Portal>
		</Select.Root>
	</div>

	<!-- Tab bar — Bits UI Tabs (audit H6) -->
	<div class="border-b border-[var(--color-border)]">
		<Tabs.Root value={data.tab} onValueChange={(v) => { if (v && v !== data.tab) window.location.href = tabHref(v); }}>
			<Tabs.List class="flex gap-1 px-4">
				{#each tabs as tab (tab.id)}
					<Tabs.Trigger
						value={tab.id}
						class="flex items-center gap-1.5 border-b-2 px-4 py-3 text-sm font-medium transition-colors data-[state=active]:border-[var(--color-primary)] data-[state=active]:text-[var(--color-primary)] data-[state=inactive]:border-transparent data-[state=inactive]:text-[var(--color-text-subtle)] data-[state=inactive]:hover:text-[var(--color-text)]"
					>
						{#snippet child({ props })}
							<div {...props}>
								<tab.icon size={15} />
								{tab.label}
							</div>
						{/snippet}
					</Tabs.Trigger>
				{/each}
			</Tabs.List>
		</Tabs.Root>
	</div>

	<div class="p-4">
		{#if data.tab === 'content'}
			<!-- Content leaderboard -->
			{#if data.content.length === 0}
				<EmptyState
					icon={Flame}
					title={t('trending.empty_content', 'No trending content in this window yet.')}
				/>
			{:else}
				<ol class="space-y-2">
					{#each data.content as item, i (item.id)}
						<li>
							<a
								href="/blog/{item.slug}"
								class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<span
									class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
								>
									{i + 1}
								</span>
								<span class="min-w-0 flex-1">
									<span
										class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										{item.title}
									</span>
									<span
										class="mt-0.5 flex items-center gap-3 text-xs text-[var(--color-muted)]"
									>
										<span class="capitalize">{item.content_type.replace('_', ' ')}</span>
										<span class="flex items-center gap-1">
											<Eye size={12} /> {item.view_count}
										</span>
									</span>
								</span>
								<span
									class="flex shrink-0 items-center gap-0.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<ArrowUp size={12} /> {score(item.score)}
								</span>
							</a>
						</li>
					{/each}
				</ol>
			{/if}
		{:else if data.tab === 'tags'}
			<!-- Tags leaderboard -->
			{#if data.tags.length === 0}
				<EmptyState
					icon={Hash}
					title={t('trending.empty_tags', 'No trending tags in this window yet.')}
				/>
			{:else}
				<ol class="space-y-2">
					{#each data.tags as item, i (item.id)}
						<li>
							<a
								href="/tags/{item.slug}"
								class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<span
									class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
								>
									{i + 1}
								</span>
								<span class="min-w-0 flex-1">
									<span
										class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										#{item.name}
									</span>
									<span class="text-xs text-[var(--color-muted)]">
										{item.follower_count}
										{t('trending.followers', 'followers')}
									</span>
								</span>
								<span
									class="flex shrink-0 items-center gap-0.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<ArrowUp size={12} /> {score(item.score)}
								</span>
							</a>
						</li>
					{/each}
				</ol>
			{/if}
		{:else if data.tab === 'spaces'}
			<!-- Spaces leaderboard -->
			{#if data.spaces.length === 0}
				<EmptyState
					icon={Layers}
					title={t('trending.empty_spaces', 'No trending spaces in this window yet.')}
				/>
			{:else}
				<ol class="space-y-2">
					{#each data.spaces as item, i (item.id)}
						<li>
							<a
								href="/spaces/{item.slug}"
								class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<span
									class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
								>
									{i + 1}
								</span>
								<span class="min-w-0 flex-1">
									<span
										class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										{item.name}
									</span>
									<span
										class="mt-0.5 flex items-center gap-3 text-xs text-[var(--color-muted)]"
									>
										<span class="flex items-center gap-1">
											<Users size={12} /> {item.member_count}
										</span>
										<span>{item.post_count} {t('trending.posts', 'posts')}</span>
									</span>
								</span>
								<span
									class="flex shrink-0 items-center gap-0.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<ArrowUp size={12} /> {score(item.score)}
								</span>
							</a>
						</li>
					{/each}
				</ol>
			{/if}
		{:else if data.tab === 'users'}
			<!-- People leaderboard -->
			{#if data.users.length === 0}
				<EmptyState
					icon={Users}
					title={t('trending.empty_users', 'No trending people in this window yet.')}
				/>
			{:else}
				<ol class="space-y-2">
					{#each data.users as item, i (item.id)}
						<li>
							<a
								href="/u/{item.username}"
								class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<span
									class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
								>
									{i + 1}
								</span>
								<span class="grid h-9 w-9 shrink-0 place-items-center overflow-hidden rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
									{#if item.avatar_url}
										<img
											src={item.avatar_url}
											alt={item.username}
											class="h-full w-full object-cover"
											loading="lazy"
										/>
									{:else}
										<Building2 size={16} />
									{/if}
								</span>
								<span class="min-w-0 flex-1">
									<span
										class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										{item.display_name ?? item.username}
									</span>
									<span class="block truncate text-xs text-[var(--color-muted)]">
										@{item.username}
									</span>
								</span>
								<span
									class="flex shrink-0 items-center gap-0.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<ArrowUp size={12} /> {score(item.score)}
								</span>
							</a>
						</li>
					{/each}
				</ol>
			{/if}
		{/if}
	</div>
</div>
