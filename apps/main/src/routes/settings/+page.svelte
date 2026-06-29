<script lang="ts">
	import { User, Bell, Shield, Loader2, Save } from 'lucide-svelte';
	import { getToken } from '$lib/api';
	import { onMount } from 'svelte';

	let activeTab = $state<'profile' | 'notifications' | 'account'>('profile');
	let saving = $state(false);
	let saved = $state(false);
	let errorMessage = $state('');

	let profile = $state({
		display_name: '',
		bio: '',
		location: ''
	});

	let notificationPrefs = $state({
		email_notifications: true,
		push_notifications: true,
		follow_notifications: true,
		comment_notifications: true
	});

	function authHeaders(extra?: Record<string, string>): Record<string, string> {
		const h: Record<string, string> = { 'Content-Type': 'application/json', ...extra };
		const token = getToken();
		if (token) h['Authorization'] = `Bearer ${token}`;
		return h;
	}

	onMount(async () => {
		try {
			const res = await fetch('/api/v1/users/me', { headers: authHeaders() });
			const body = await res.json();
			if (body.data) {
				profile = { ...profile, ...body.data };
			}
		} catch {
			// use defaults
		}
	});

	async function saveProfile() {
		saving = true;
		saved = false;
		errorMessage = '';
		try {
			await fetch('/api/v1/users/me', {
				method: 'PATCH',
				headers: authHeaders(),
				body: JSON.stringify(profile)
			});
			saved = true;
			setTimeout(() => (saved = false), 2000);
		} catch {
			errorMessage = 'Failed to save profile. Please try again.';
		} finally {
			saving = false;
		}
	}

	const tabs = [
		{ id: 'profile' as const, icon: User, label: 'Profile' },
		{ id: 'notifications' as const, icon: Bell, label: 'Notifications' },
		{ id: 'account' as const, icon: Shield, label: 'Account' }
	];
</script>

<svelte:head>
	<title>Settings | JapaConnect</title>
	<meta name="description" content="Manage your JapaConnect account settings." />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<h1 class="mb-6 text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">Settings</h1>

	<div class="flex gap-6">
		<!-- Sidebar Tabs -->
		<aside class="hidden w-48 shrink-0 md:block">
			<nav class="space-y-1">
				{#each tabs as tab}
					<button
						onclick={() => (activeTab = tab.id)}
						class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
						style="color: {activeTab === tab.id ? 'var(--color-primary)' : 'var(--color-foreground)'}; background-color: {activeTab === tab.id ? 'var(--color-surface)' : 'transparent'}"
					>
						<tab.icon class="h-4 w-4" />
						{tab.label}
					</button>
				{/each}
			</nav>
		</aside>

		<!-- Mobile Tabs -->
		<div class="mb-4 flex gap-2 md:hidden">
			{#each tabs as tab}
				<button
					onclick={() => (activeTab = tab.id)}
					class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
					style="color: {activeTab === tab.id ? 'white' : 'var(--color-foreground)'}; background-color: {activeTab === tab.id ? 'var(--color-primary)' : 'var(--color-surface)'}"
				>
					<tab.icon class="h-3.5 w-3.5" />
					{tab.label}
				</button>
			{/each}
		</div>

		<!-- Content -->
		<div class="flex-1 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			{#if activeTab === 'profile'}
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">Profile Settings</h2>
				<div class="space-y-4">
					<div>
						<label for="display_name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							Display Name
						</label>
						<input
							id="display_name"
							type="text"
							bind:value={profile.display_name}
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						/>
					</div>
					<div>
						<label for="bio" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							Bio
						</label>
						<textarea
							id="bio"
							bind:value={profile.bio}
							rows="3"
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						></textarea>
					</div>
					<div>
						<label for="location" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							Location
						</label>
						<input
							id="location"
							type="text"
							bind:value={profile.location}
							placeholder="e.g. Lagos, Nigeria"
							class="w-full rounded-lg border px-3 py-2 text-sm outline-none focus:border-[var(--color-primary)]"
							style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
						/>
					</div>
					{#if errorMessage}
						<p class="text-sm" style="color: var(--color-danger)">{errorMessage}</p>
					{/if}
					<button
						onclick={saveProfile}
						disabled={saving}
						class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-primary)"
					>
						{#if saving}
							<Loader2 class="h-4 w-4 animate-spin" />
						{:else}
							<Save class="h-4 w-4" />
						{/if}
						{saved ? 'Saved!' : 'Save changes'}
					</button>
				</div>

			{:else if activeTab === 'notifications'}
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">Notification Preferences</h2>
				<div class="space-y-4">
					{#each Object.entries(notificationPrefs) as [key, value]}
						<label class="flex items-center justify-between rounded-lg border p-3" style="border-color: var(--color-border)">
							<span class="text-sm" style="color: var(--color-foreground)">
								{key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())}
							</span>
							<input
								type="checkbox"
								checked={value}
								onchange={(e) => {
									notificationPrefs = { ...notificationPrefs, [key]: (e.target as HTMLInputElement).checked };
								}}
								class="h-4 w-4 rounded"
								style="accent-color: var(--color-primary)"
							/>
						</label>
					{/each}
				</div>

			{:else if activeTab === 'account'}
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">Account Settings</h2>
				<div class="space-y-4">
					<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
						<h3 class="text-sm font-medium" style="color: var(--color-foreground)">Change Password</h3>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							Update your password to keep your account secure.
						</p>
						<button
							class="mt-3 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							Change password
						</button>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-danger)">
						<h3 class="text-sm font-medium" style="color: var(--color-danger)">Danger Zone</h3>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							Permanently delete your account and all associated data.
						</p>
						<button
							class="mt-3 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-danger); color: var(--color-danger)"
						>
							Delete account
						</button>
					</div>
				</div>
			{/if}
		</div>
	</div>
</main>
