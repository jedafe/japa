<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Users, Star } from 'lucide-svelte';

	type Space = { id: string; name: string; member_count: number };
	type Following = { user_id: string; followed_at: string };
	type FollowedTag = { tag: string; followed_at: string };

	let spaces = $state<Space[]>([]);
	let following = $state<Following[]>([]);
	let followedTags = $state<FollowedTag[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const [spacesData, followingData, tagsData] = await Promise.all([
				request<Space[]>('/me/spaces'),
				request<Following[]>('/me/following'),
				request<FollowedTag[]>('/me/followed-tags')
			]);
			spaces = spacesData ?? [];
			following = followingData ?? [];
			followedTags = tagsData ?? [];
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.following.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.following.title')}</h1>

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
	<div class="grid gap-6 lg:grid-cols-3">
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><Users class="h-4 w-4" style="color: var(--color-muted)" /> {t('me.following.spaces')}</h2>
			</div>
			{#if spaces.length === 0}
				<p class="px-4 py-4 text-sm" style="color: var(--color-muted)">{t('me.following.spaces_empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each spaces as s (s.id)}
						<a href="/spaces/{s.id}" class="block px-4 py-3 text-sm hover:bg-black/5">{s.name}</a>
					{/each}
				</div>
			{/if}
		</div>
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><Users class="h-4 w-4" style="color: var(--color-muted)" /> {t('me.following.users')}</h2>
			</div>
			{#if following.length === 0}
				<p class="px-4 py-4 text-sm" style="color: var(--color-muted)">{t('me.following.users_empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each following as f (f.user_id)}
						<a href="/profile/{f.user_id}" class="block px-4 py-3 text-sm hover:bg-black/5">{f.user_id}</a>
					{/each}
				</div>
			{/if}
		</div>
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<div class="border-b px-4 py-3" style="border-color: var(--color-border); background: var(--color-surface)">
				<h2 class="font-medium flex items-center gap-2"><Star class="h-4 w-4" style="color: var(--color-muted)" /> {t('me.following.tags')}</h2>
			</div>
			{#if followedTags.length === 0}
				<p class="px-4 py-4 text-sm" style="color: var(--color-muted)">{t('me.following.tags_empty')}</p>
			{:else}
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each followedTags as ft (ft.tag)}
						<a href="/tags/{ft.tag}" class="block px-4 py-3 text-sm hover:bg-black/5">#{ft.tag}</a>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/if}
