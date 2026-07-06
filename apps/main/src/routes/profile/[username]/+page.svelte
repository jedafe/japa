<script lang="ts">
	import { page } from '$app/state';
	import { UserPlus, UserCheck, Loader2, MapPin, Calendar, Rss } from 'lucide-svelte';
	import { api, type UserProfile } from '$lib/api';
	import { trackEvent, AnalyticsEvents } from '$lib/analytics';

	let profile = $state<UserProfile | null>(null);

	let loading = $state(true);
	let followLoading = $state(false);
	let error = $state('');
	let followError = $state('');

	const username = $derived(page.params.username ?? '');

	$effect(() => {
		loadProfile();
	});

	async function loadProfile() {
		loading = true;
		error = '';
		try {
			const user = await api.getUser(username);
			profile = { following: false, follower_count: 0, following_count: 0, content_count: 0, ...user };
		} catch {
			error = 'User not found.';
		} finally {
			loading = false;
		}
	}

	async function toggleFollow() {
		if (!profile) return;
		followLoading = true;
		followError = '';
		try {
			const result = await api.toggleFollow({ entity_type: 'user', entity_id: profile.id });
			profile.following = result.following;
			profile.follower_count = (profile.follower_count ?? 0) + (result.following ? 1 : -1);
			trackEvent(result.following ? AnalyticsEvents.FOLLOW : AnalyticsEvents.UNFOLLOW, { user_id: profile.id });
		} catch {
			followError = 'Failed to update follow status. Please try again.';
		} finally {
			followLoading = false;
		}
	}
</script>

<svelte:head>
	<title>{profile?.display_name || username} | JapaConnect</title>
	<meta name="description" content={profile?.bio || `View ${username}'s profile on JapaConnect.`} />
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	{#if loading}
		<div class="space-y-4">
			<div class="h-32 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
			<div class="h-48 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
		</div>
	{:else if error}
		<div class="rounded-xl border p-8 text-center" style="border-color: var(--color-border)">
			<p style="color: var(--color-muted)">{error}</p>
			<a href="/" class="mt-4 inline-block text-sm font-medium hover:underline" style="color: var(--color-primary)">
				Go home
			</a>
		</div>
	{:else if profile}
		<!-- Profile Header -->
		<div class="rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			<div class="flex items-start gap-4">
				{#if profile.avatar_url}
					<img
						src={profile.avatar_url}
						alt={profile.display_name}
						class="h-16 w-16 rounded-full object-cover"
					/>
				{:else}
					<div
						class="flex h-16 w-16 items-center justify-center rounded-full text-xl font-bold text-white"
						style="background-color: var(--color-primary)"
					>
						{(profile.display_name || profile.username).charAt(0).toUpperCase()}
					</div>
				{/if}

				<div class="flex-1 min-w-0">
					<div class="flex items-center gap-3">
						<h1 class="text-xl font-bold" style="color: var(--color-foreground)">
							{profile.display_name}
						</h1>
						{#if followError}
						<p class="text-xs" style="color: var(--color-danger)">{followError}</p>
					{/if}
					<button
							onclick={toggleFollow}
							disabled={followLoading}
							class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
							style="background-color: {profile.following ? 'var(--color-surface)' : 'var(--color-primary)'}; color: {profile.following ? 'var(--color-foreground)' : 'white'}; border: 1px solid {profile.following ? 'var(--color-border)' : 'transparent'}"
						>
							{#if followLoading}
								<Loader2 class="h-3.5 w-3.5 animate-spin" />
							{:else if profile.following}
								<UserCheck class="h-3.5 w-3.5" />
								Following
							{:else}
								<UserPlus class="h-3.5 w-3.5" />
								Follow
							{/if}
						</button>
					</div>
					<p class="text-sm" style="color: var(--color-muted)">@{profile.username}</p>

					{#if profile.bio}
						<p class="mt-3 text-sm" style="color: var(--color-foreground)">{profile.bio}</p>
					{/if}

					<div class="mt-3 flex flex-wrap items-center gap-4 text-sm" style="color: var(--color-muted)">
						{#if profile.location}
							<span class="flex items-center gap-1">
								<MapPin class="h-3.5 w-3.5" />
								{profile.location}
							</span>
						{/if}
						<a
							href="/profile/{profile.username}/followers"
							class="flex items-center gap-1 hover:underline"
							style="color: var(--color-muted)"
						>
							<UserPlus class="h-3.5 w-3.5" />
							{profile.follower_count} followers
						</a>
						<a
							href="/profile/{profile.username}/following"
							class="hover:underline"
							style="color: var(--color-muted)"
						>
							{profile.following_count ?? 0} following
						</a>
						<span>
							{profile.content_count} posts
						</span>
						<a
							href="/api/v1/users/{profile.username}/feed.xml"
							class="flex items-center gap-1 rounded-lg border px-2 py-1 text-xs font-medium transition-colors hover:opacity-80"
							style="border-color: var(--color-border); color: var(--color-muted)"
							target="_blank"
							rel="noopener noreferrer"
							title="Subscribe via RSS"
						>
							<Rss class="h-3 w-3" />
							RSS
						</a>
					</div>
				</div>
			</div>
		</div>

		<!-- User's Content Feed -->
		<div class="mt-6">
			<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">Posts</h2>
			<div
				class="rounded-xl border p-8 text-center"
				style="border-color: var(--color-border)"
			>
				<p class="text-sm" style="color: var(--color-muted)">
					Content feed coming soon.
				</p>
			</div>
		</div>
	{/if}
</main>
