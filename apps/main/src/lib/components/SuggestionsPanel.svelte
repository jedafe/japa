<script lang="ts">
	import { UserPlus, Layers, Hash, Loader2 } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';

	type SuggestedUser  = { id: string; username: string; display_name?: string; avatar_url?: string };
	type SuggestedSpace = { id: string; slug: string; name: string; member_count?: number };
	type SuggestedTag   = { id: string; slug: string; name: string; post_count?: number };

	let users  = $state<SuggestedUser[]>([]);
	let spaces = $state<SuggestedSpace[]>([]);
	let tags   = $state<SuggestedTag[]>([]);
	let loading = $state(true);

	onMount(async () => {
		try {
			const [u, s, t] = await Promise.allSettled([
				api.getSuggestedUsers(),
				api.getSuggestedSpaces(),
				api.getSuggestedTags()
			]);
			if (u.status === 'fulfilled') users  = (u.value.items ?? []) as SuggestedUser[];
			if (s.status === 'fulfilled') spaces = (s.value.items ?? []) as SuggestedSpace[];
			if (t.status === 'fulfilled') tags   = (t.value.items ?? []) as SuggestedTag[];
		} catch {
			// degrade gracefully
		} finally {
			loading = false;
		}
	});

	async function followUser(userId: string) {
		try {
			await api.followUser(userId);
			users = users.filter(u => u.id !== userId);
		} catch { /* ignore */ }
	}
</script>

<aside
	class="space-y-5 rounded-2xl border p-4"
	style="border-color: var(--color-border); background: var(--color-surface)"
>
	{#if loading}
		<div class="flex justify-center py-4">
			<Loader2 class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
		</div>
	{:else}
		<!-- Suggested people -->
		{#if users.length > 0}
			<section>
				<h2 class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide"
					style="color: var(--color-muted)">
					<UserPlus class="h-3.5 w-3.5" /> Who to follow
				</h2>
				<ul class="space-y-2">
					{#each users.slice(0, 4) as user (user.id)}
						<li class="flex items-center gap-2.5">
							<a href="/profile/{user.username}" class="flex items-center gap-2 min-w-0 flex-1">
								{#if user.avatar_url}
									<img src={user.avatar_url} alt={user.username}
										class="h-8 w-8 rounded-full object-cover shrink-0" />
								{:else}
									<div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white"
										style="background: var(--gradient-primary)">
										{(user.display_name ?? user.username).charAt(0).toUpperCase()}
									</div>
								{/if}
								<div class="min-w-0">
									<p class="truncate text-sm font-medium" style="color: var(--color-foreground)">
										{user.display_name ?? user.username}
									</p>
									<p class="truncate text-xs" style="color: var(--color-muted)">@{user.username}</p>
								</div>
							</a>
							<button
								onclick={() => followUser(user.id)}
								class="shrink-0 rounded-full px-3 py-1 text-xs font-semibold transition-colors"
								style="background: color-mix(in srgb, var(--color-primary) 12%, transparent); color: var(--color-primary)"
							>
								Follow
							</button>
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		<!-- Suggested spaces -->
		{#if spaces.length > 0}
			<section>
				<h2 class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide"
					style="color: var(--color-muted)">
					<Layers class="h-3.5 w-3.5" /> Spaces to join
				</h2>
				<ul class="space-y-1.5">
					{#each spaces.slice(0, 3) as space (space.id)}
						<li>
							<a href="/spaces/{space.slug}"
								class="flex items-center justify-between rounded-lg px-2 py-1.5 text-sm transition-colors hover:opacity-80"
								style="color: var(--color-foreground)">
								<span class="truncate font-medium">{space.name}</span>
								{#if space.member_count !== undefined}
									<span class="ml-2 shrink-0 text-xs" style="color: var(--color-muted)">
										{space.member_count.toLocaleString()} members
									</span>
								{/if}
							</a>
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		<!-- Suggested tags -->
		{#if tags.length > 0}
			<section>
				<h2 class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide"
					style="color: var(--color-muted)">
					<Hash class="h-3.5 w-3.5" /> Topics to explore
				</h2>
				<div class="flex flex-wrap gap-1.5">
					{#each tags.slice(0, 6) as tag (tag.id)}
						<a href="/tags/{tag.slug}"
							class="rounded-full px-3 py-1 text-xs font-medium transition-colors hover:opacity-80"
							style="background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">
							#{tag.name}
						</a>
					{/each}
				</div>
			</section>
		{/if}

		{#if users.length === 0 && spaces.length === 0 && tags.length === 0}
			<p class="text-center text-xs py-4" style="color: var(--color-muted)">
				No suggestions available yet
			</p>
		{/if}
	{/if}
</aside>
