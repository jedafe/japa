<script lang="ts">
	import { page } from '$app/state';
	import { UserPlus, UserCheck, Loader2, MapPin, Calendar } from 'lucide-svelte';
	import { getToken } from '$lib/api';

	let profile = $state<{
		id: string;
		username: string;
		display_name: string;
		avatar_url: string | null;
		bio: string | null;
		location: string | null;
		following: boolean;
		follower_count: number;
		content_count: number;
	} | null>(null);

	let loading = $state(true);
	let followLoading = $state(false);
	let error = $state('');
	let followError = $state('');

	const username = $derived(page.params.username);

	$effect(() => {
		loadProfile();
	});

	function authHeaders(extra?: Record<string, string>): Record<string, string> {
		const h: Record<string, string> = { 'Content-Type': 'application/json', ...extra };
		const token = getToken();
		if (token) h['Authorization'] = `Bearer ${token}`;
		return h;
	}

	async function loadProfile() {
		loading = true;
		error = '';
		try {
			const res = await fetch(`/api/v1/users/${username}`, { headers: authHeaders() });
			const body = await res.json();
			if (body.data) {
				profile = body.data;
			} else {
				error = 'User not found.';
			}
		} catch {
			error = 'Failed to load profile.';
		} finally {
			loading = false;
		}
	}

	async function toggleFollow() {
		if (!profile) return;
		followLoading = true;
		followError = '';
		try {
			const res = await fetch('/api/v1/follows', {
				method: 'POST',
				headers: authHeaders(),
				body: JSON.stringify({ entity_type: 'user', entity_id: profile.id })
			});
			const body = await res.json();
			if (body.data) {
				profile.following = body.data.following;
				profile.follower_count += body.data.following ? 1 : -1;
			}
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
						<span class="flex items-center gap-1">
							<UserPlus class="h-3.5 w-3.5" />
							{profile.follower_count} followers
						</span>
						<span>
							{profile.content_count} posts
						</span>
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
