<script lang="ts">
	import { page } from '$app/state';
	import { ArrowLeft, UserPlus, UserCheck, Users, Loader2, ChevronLeft, ChevronRight, Sparkles } from 'lucide-svelte';
	import { api, type FollowUser } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';

	let following = $state<FollowUser[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state('');
	let currentPage = $state(0);
	const pageSize = 20;
	let followLoadingId = $state<string | null>(null);

	const username = $derived(page.params.username);
	const totalPages = $derived(Math.ceil(total / pageSize));

	let userId = $state<string | null>(null);

	onMount(async () => {
		await resolveUser();
	});

	async function resolveUser() {
		loading = true;
		error = '';
		try {
			const user = await api.getUser(username ?? '');
			userId = user.id;
			await loadFollowing();
		} catch {
			error = 'Failed to load user.';
		} finally {
			loading = false;
		}
	}

	async function loadFollowing() {
		if (!userId) return;
		loading = true;
		try {
			const result = await api.getFollowing(userId, { limit: pageSize, offset: currentPage * pageSize });
			following = result.following;
			total = result.total;
		} catch {
			error = 'Failed to load following.';
		} finally {
			loading = false;
		}
	}

	async function toggleFollow(user: FollowUser) {
		followLoadingId = user.id;
		try {
			const result = await api.toggleFollow({ entity_type: 'user', entity_id: user.id });
			following = following.map((f) =>
				f.id === user.id ? { ...f, following: result.following } : f
			);
		} catch {
			// ignore
		} finally {
			followLoadingId = null;
		}
	}

	function prevPage() {
		if (currentPage > 0) {
			currentPage--;
			loadFollowing();
		}
	}

	function nextPage() {
		if ((currentPage + 1) * pageSize < total) {
			currentPage++;
			loadFollowing();
		}
	}
</script>

<svelte:head>
	<title>Following — @{username} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<a
		href="/profile/{username}"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		{t('profile.back_to')} @{username}
	</a>

	<h1 class="mb-6 flex items-center gap-2 text-2xl font-bold" style="color: var(--color-foreground)">
		<Users class="h-6 w-6" style="color: var(--color-primary)" />
		{t('profile.following')}
		{#if total > 0}
			<span class="text-base font-normal" style="color: var(--color-muted)">({total})</span>
		{/if}
	</h1>

	{#if loading}
		<div class="space-y-3">
			{#each [1, 2, 3, 4, 5] as _}
				<div class="h-16 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
			{/each}
		</div>
	{:else if error}
		<div class="rounded-xl border p-8 text-center" style="border-color: var(--color-border)">
			<p style="color: var(--color-muted)">{error}</p>
		</div>
	{:else if following.length === 0}
		<div class="rounded-xl border p-8 text-center" style="border-color: var(--color-border)">
			<Users class="mx-auto mb-3 h-10 w-10" style="color: var(--color-muted)" />
			<p class="text-sm" style="color: var(--color-muted)">{t('profile.no_following')}</p>
		</div>
	{:else}
		<div class="space-y-2">
			{#each following as user (user.id)}
				<div
					class="flex items-center gap-3 rounded-xl border p-4 transition-colors"
					style="border-color: var(--color-border); background: var(--color-surface)"
				>
					<a href="/profile/{user.username}" class="flex items-center gap-3 min-w-0 flex-1">
						{#if user.avatar_url}
							<img src={user.avatar_url} alt="" class="h-10 w-10 shrink-0 rounded-full object-cover" />
						{:else}
							<div
								class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white"
								style="background-color: var(--color-primary)"
							>
								{(user.display_name || user.username).charAt(0).toUpperCase()}
							</div>
						{/if}
						<div class="min-w-0">
							<div class="flex items-center gap-2">
								<span class="truncate text-sm font-medium" style="color: var(--color-foreground)">
									{user.display_name}
								</span>
								{#if user.mutual}
									<span
										class="inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[10px] font-medium"
										style="background-color: color-mix(in srgb, var(--color-success, #16a34a) 12%, transparent); color: var(--color-success, #16a34a)"
									>
										<Sparkles class="h-2.5 w-2.5" />
										{t('profile.mutual')}
									</span>
								{/if}
							</div>
							<span class="text-xs" style="color: var(--color-muted)">@{user.username}</span>
						</div>
					</a>
					<button
						onclick={() => toggleFollow(user)}
						disabled={followLoadingId === user.id}
						class="flex shrink-0 items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors"
						style="background-color: {user.following ? 'var(--color-surface)' : 'var(--color-primary)'}; color: {user.following ? 'var(--color-foreground)' : 'white'}; border: 1px solid {user.following ? 'var(--color-border)' : 'transparent'}"
					>
						{#if followLoadingId === user.id}
							<Loader2 class="h-3 w-3 animate-spin" />
						{:else if user.following}
							<UserCheck class="h-3 w-3" />
							{t('profile.following')}
						{:else}
							<UserPlus class="h-3 w-3" />
							{t('profile.follow')}
						{/if}
					</button>
				</div>
			{/each}
		</div>

		{#if totalPages > 1}
			<div class="mt-6 flex items-center justify-center gap-2">
				<button
					onclick={prevPage}
					disabled={currentPage === 0}
					class="flex items-center gap-1 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-50"
					style="border-color: var(--color-border); color: var(--color-foreground)"
				>
					<ChevronLeft class="h-4 w-4" />
					{t('reviews.prev')}
				</button>
				<span class="text-sm" style="color: var(--color-muted)">
					{currentPage + 1} of {totalPages}
				</span>
				<button
					onclick={nextPage}
					disabled={(currentPage + 1) >= totalPages}
					class="flex items-center gap-1 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-50"
					style="border-color: var(--color-border); color: var(--color-foreground)"
				>
					{t('common.next')}
					<ChevronRight class="h-4 w-4" />
				</button>
			</div>
		{/if}
	{/if}
</main>
