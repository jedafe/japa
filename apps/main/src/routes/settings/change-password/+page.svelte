<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowLeft, Loader2, CheckCircle2 } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let loading = $state(false);
	let error = $state('');
	let success = $state(false);

	async function changePassword(e: Event) {
		e.preventDefault();
		error = '';
		
		if (newPassword !== confirmPassword) {
			error = 'Passwords do not match.';
			return;
		}

		if (newPassword.length < 8) {
			error = 'Password must be at least 8 characters.';
			return;
		}

		loading = true;
		try {
			await fetch('/api/v1/me/change-password', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				credentials: 'include',
				body: JSON.stringify({ current_password: currentPassword, new_password: newPassword })
			}).then(async (res) => {
				if (!res.ok) {
					const data = await res.json();
					throw new Error(data.error?.message || 'Failed to change password');
				}
			});
			success = true;
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to change password.';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Change Password | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-lg px-4 py-8">
	<a
		href="/settings"
		class="mb-6 inline-flex items-center gap-1.5 text-sm font-medium hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to Settings
	</a>

	<h1 class="mb-6 text-2xl font-bold" style="color: var(--color-foreground)">{t('settings.change-password.change_password')}</h1>

	{#if success}
		<div class="mb-6 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-success, #16a34a); background-color: color-mix(in srgb, var(--color-success, #16a34a) 8%, transparent); color: var(--color-success, #16a34a)">
			<CheckCircle2 class="h-4 w-4" />
			Password changed successfully!
		</div>
	{/if}

	{#if error}
		<div class="mb-6 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	<form onsubmit={changePassword} class="space-y-4 rounded-xl border p-6" style="border-color: var(--color-border); background: var(--color-surface);">
		<div>
			<label for="current-password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				Current Password
			</label>
			<input
				id="current-password"
				type="password"
				required
				bind:value={currentPassword}
				class="w-full rounded-lg border py-2.5 px-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>

		<div>
			<label for="new-password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				New Password
			</label>
			<input
				id="new-password"
				type="password"
				required
				minlength="8"
				bind:value={newPassword}
				class="w-full rounded-lg border py-2.5 px-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>

		<div>
			<label for="confirm-password" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				Confirm New Password
			</label>
			<input
				id="confirm-password"
				type="password"
				required
				minlength="8"
				bind:value={confirmPassword}
				class="w-full rounded-lg border py-2.5 px-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)]"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>

		<button
			type="submit"
			disabled={loading || !currentPassword || !newPassword || !confirmPassword}
			class="flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
			style="background-color: var(--color-primary)"
		>
			{#if loading}
				<Loader2 class="h-4 w-4 animate-spin" />
			{/if}
			Change Password
		</button>
	</form>
</main>
