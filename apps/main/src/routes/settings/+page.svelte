<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { goto } from '$app/navigation';
	import { User, Bell, Shield, Loader2, Save, Trash2, KeyRound, Lock, Monitor, Link2 } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';
	import { Button, Input, Textarea } from '$lib/components/ui';

	type TabId = 'profile' | 'notifications' | 'account';

	let activeTab = $state<TabId>('profile');
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

	// Delete account state
	let showDeleteConfirm = $state(false);
	let deleteConfirmText = $state('');
	let deleteLoading = $state(false);
	let deleteError = $state('');

	onMount(async () => {
		try {
			const me = await api.getMe();
			profile = { ...profile, display_name: me.display_name ?? '', bio: me.bio ?? '', location: me.location ?? '' };
		} catch {
			// use defaults
		}
	});

	async function saveProfile() {
		saving = true;
		saved = false;
		errorMessage = '';
		try {
			await api.updateMe(profile);
			saved = true;
			setTimeout(() => (saved = false), 2000);
		} catch {
			errorMessage = 'Failed to save profile. Please try again.';
		} finally {
			saving = false;
		}
	}

	async function deleteAccount() {
		if (deleteConfirmText !== 'DELETE') return;
		deleteLoading = true;
		deleteError = '';

		try {
			await api.requestAccountDeletion();
			// Redirect to server-side logout to properly clear the httpOnly cookie.
			goto('/logout');
		} catch {
			deleteError = 'Network error. Please try again.';
		} finally {
			deleteLoading = false;
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
	<h1 class="mb-6 text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{t('settings.settings')}</h1>

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
				<a
					href="/settings/security"
					class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
					style="color: var(--color-foreground)"
				>
					<KeyRound class="h-4 w-4" />
					Security
				</a>
				<a
					href="/settings/change-password"
					class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
					style="color: var(--color-foreground)"
				>
					<KeyRound class="h-4 w-4" />
					Change Password
				</a>
				<a
					href="/settings/privacy"
					class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
					style="color: var(--color-foreground)"
				>
					<Lock class="h-4 w-4" />
					Privacy
				</a>
				<a
					href="/settings/sessions"
					class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
					style="color: var(--color-foreground)"
				>
					<Monitor class="h-4 w-4" />
					Sessions
				</a>
				<a
					href="/settings/connected-accounts"
					class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition-colors"
					style="color: var(--color-foreground)"
				>
					<Link2 class="h-4 w-4" />
					Connected Accounts
				</a>
			</nav>
		</aside>

		<!-- Mobile Tabs -->
		<div class="mb-4 flex flex-wrap gap-2 md:hidden">
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
			<a
				href="/settings/security"
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="color: var(--color-foreground); background-color: var(--color-surface)"
			>
				<KeyRound class="h-3.5 w-3.5" />
				Security
			</a>
			<a
				href="/settings/change-password"
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="color: var(--color-foreground); background-color: var(--color-surface)"
			>
				<KeyRound class="h-3.5 w-3.5" />
				Password
			</a>
			<a
				href="/settings/privacy"
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="color: var(--color-foreground); background-color: var(--color-surface)"
			>
				<Lock class="h-3.5 w-3.5" />
				Privacy
			</a>
			<a
				href="/settings/sessions"
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="color: var(--color-foreground); background-color: var(--color-surface)"
			>
				<Monitor class="h-3.5 w-3.5" />
				Sessions
			</a>
			<a
				href="/settings/connected-accounts"
				class="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="color: var(--color-foreground); background-color: var(--color-surface)"
			>
				<Link2 class="h-3.5 w-3.5" />
				Accounts
			</a>
		</div>

		<!-- Content -->
		<div class="flex-1 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
			{#if activeTab === 'profile'}
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">{t('settings.profile_settings')}</h2>
				<div class="space-y-4">
					<div>
						<label for="display_name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							Display Name
						</label>
						<Input
							id="display_name"
							type="text"
							bind:value={profile.display_name}
						/>
					</div>
					<div>
						<label for="bio" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							Bio
						</label>
						<Textarea
							id="bio"
							bind:value={profile.bio}
							rows={3}
						/>
					</div>
					<div>
						<label for="location" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							Location
						</label>
						<Input
							id="location"
							type="text"
							bind:value={profile.location}
							placeholder="e.g. Lagos, Nigeria"
						/>
					</div>
					{#if errorMessage}
						<p class="text-sm" style="color: var(--color-danger)">{errorMessage}</p>
					{/if}
					<Button
						onclick={saveProfile}
						disabled={saving}
						loading={saving}
					>
						{#if !saving}
							<Save class="h-4 w-4" />
						{/if}
						{saved ? 'Saved!' : 'Save changes'}
					</Button>
				</div>

			{:else if activeTab === 'notifications'}
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">{t('settings.notification_preferences')}</h2>
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
				<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">{t('settings.account_settings')}</h2>
				<div class="space-y-4">
					<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
						<h3 class="text-sm font-medium" style="color: var(--color-foreground)">{t('settings.change_password')}</h3>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							Update your password to keep your account secure.
						</p>
						<a
							href="/settings/change-password"
							class="mt-3 inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							Change password
						</a>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
						<h3 class="flex items-center gap-2 text-sm font-medium" style="color: var(--color-foreground)">
							<KeyRound class="h-4 w-4" />
							Two-Factor Authentication
						</h3>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							Add an extra layer of security with 2FA.
						</p>
						<a
							href="/settings/security"
							class="mt-3 inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							Manage 2FA
						</a>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
						<h3 class="flex items-center gap-2 text-sm font-medium" style="color: var(--color-foreground)">
							<Link2 class="h-4 w-4" />
							Connected Accounts
						</h3>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							Manage your connected social accounts.
						</p>
						<a
							href="/settings/connected-accounts"
							class="mt-3 inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							Manage accounts
						</a>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
						<h3 class="flex items-center gap-2 text-sm font-medium" style="color: var(--color-foreground)">
							<Monitor class="h-4 w-4" />
							Active Sessions
						</h3>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							View and revoke active sessions on your account.
						</p>
						<a
							href="/settings/sessions"
							class="mt-3 inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border); color: var(--color-foreground)"
						>
							Manage sessions
						</a>
					</div>
					<div class="rounded-lg border p-4" style="border-color: var(--color-danger)">
						<h3 class="text-sm font-medium" style="color: var(--color-danger)">{t('settings.danger_zone')}</h3>
						<p class="mt-1 text-sm" style="color: var(--color-muted)">
							Permanently delete your account and all associated data. This action cannot be undone.
						</p>

						{#if !showDeleteConfirm}
							<button
								onclick={() => (showDeleteConfirm = true)}
								class="mt-3 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
								style="border-color: var(--color-danger); color: var(--color-danger)"
							>
								Delete account
							</button>
						{:else}
							<div class="mt-3 space-y-3">
								{#if deleteError}
									<div
										class="rounded-lg border px-4 py-3 text-sm"
										style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
									>
										{deleteError}
									</div>
								{/if}
								<p class="text-sm" style="color: var(--color-danger)">
									Type <strong>{t('settings.delete')}</strong> to confirm:
								</p>
								<Input
									type="text"
									bind:value={deleteConfirmText}
									placeholder="DELETE"
								/>
								<div class="flex gap-2">
									<Button
										variant="danger"
										onclick={deleteAccount}
										disabled={deleteLoading || deleteConfirmText !== 'DELETE'}
										loading={deleteLoading}
									>
										{#if !deleteLoading}
											<Trash2 class="h-4 w-4" />
										{/if}
										Permanently delete
									</Button>
									<Button
										variant="secondary"
										onclick={() => { showDeleteConfirm = false; deleteConfirmText = ''; deleteError = ''; }}
									>
										Cancel
									</Button>
								</div>
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	</div>
</main>
