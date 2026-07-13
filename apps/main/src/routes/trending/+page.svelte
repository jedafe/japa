<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Star, TrendingUp, Users, Tag } from 'lucide-svelte';

	type TrendingItem = { id: string; title: string; score: number };

	let content = $state<TrendingItem[]>([]);
	let spaces = $state<TrendingItem[]>([]);
	let tags = $state<TrendingItem[]>([]);
	let users = $state<TrendingItem[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const [contentData, spacesData, tagsData, usersData] = await Promise.all([
				request<TrendingItem[]>('/trending/content'),
				request<TrendingItem[]>('/trending/spaces'),
				request<TrendingItem[]>('/trending/tags'),
				request<TrendingItem[]>('/trending/users')
			]);
			content = contentData ?? [];
			spaces = spacesData ?? [];
			tags = tagsData ?? [];
			users = usersData ?? [];
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('trending.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('trending.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="grid gap-6 lg:grid-cols-2">
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><TrendingUp class="h-4 w-4" style="color: var(--color-muted)" /> {t('trending.content')}</h2>
			</div>
			{#if content.length === 0}
				<p class="px-4 py-4 text-sm" style="color: var(--color-muted)">{t('trending.empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each content as item (item.id)}
						<a href="/content/{item.id}" class="block px-4 py-3 text-sm hover:bg-black/5">{item.title}</a>
					{/each}
				</div>
			{/if}
		</div>
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><Users class="h-4 w-4" style="color: var(--color-muted)" /> {t('trending.users')}</h2>
			</div>
			{#if users.length === 0}
				<p class="px-4 py-4 text-sm" style="color: var(--color-muted)">{t('trending.empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each users as item (item.id)}
						<a href="/profile/{item.id}" class="block px-4 py-3 text-sm hover:bg-black/5">{item.title}</a>
					{/each}
				</div>
			{/if}
		</div>
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><Tag class="h-4 w-4" style="color: var(--color-muted)" /> {t('trending.tags')}</h2>
			</div>
			{#if tags.length === 0}
				<p class="px-4 py-4 text-sm" style="color: var(--color-muted)">{t('trending.empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each tags as item (item.id)}
						<a href="/tags/{item.title}" class="block px-4 py-3 text-sm hover:bg-black/5">#{item.title}</a>
					{/each}
				</div>
			{/if}
		</div>
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><Star class="h-4 w-4" style="color: var(--color-muted)" /> {t('trending.spaces')}</h2>
			</div>
			{#if spaces.length === 0}
				<p class="px-4 py-4 text-sm" style="color: var(--color-muted)">{t('trending.empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each spaces as item (item.id)}
						<a href="/spaces/{item.id}" class="block px-4 py-3 text-sm hover:bg-black/5">{item.title}</a>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/if}
