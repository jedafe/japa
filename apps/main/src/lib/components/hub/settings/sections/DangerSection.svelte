<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { Modal } from '$lib/components/ui';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { exportData, deactivateAccount } from '$lib/api/hub';
	import DeleteAccountDialog from '../flows/DeleteAccountDialog.svelte';

	let { data }: { data: unknown } = $props();
	// The danger loader returns the username from /me so the deletion
	// confirmation prompt can require typing the user's exact username.
	const sectionData = $derived((data as { username: string } | null) ?? null);
	const username = $derived(sectionData?.username ?? '');

	let exportRequested = $state(false);
	let exportError = $state<string | null>(null);
	let deactivateOpen = $state(false);
	let deleteOpen = $state(false);
	let deactivating = $state(false);
	let deactivateError = $state<string | null>(null);
	let deactivatePassword = $state('');

	async function doExport() {
		exportError = null;
		const res = await exportData();
		if (res.error) {
			exportError = res.error.message;
			return;
		}
		exportRequested = true;
	}

	async function doDeactivate() {
		if (deactivating || !deactivatePassword) return;
		deactivating = true;
		deactivateError = null;
		const res = await deactivateAccount(deactivatePassword);
		deactivating = false;
		if (res.error) {
			// Keep the dialog open so the user can correct the password.
			deactivateError = res.error.message;
			return;
		}
		// Deactivation revokes every session server-side — send the user to
		// the login screen; signing back in reactivates the account (§50).
		deactivateOpen = false;
		await goto('/login');
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.danger')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.danger.desc')}</p>
	</header>

	<div class="space-y-4">
		<!-- Export data -->
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-1 text-sm font-medium text-[var(--color-text)]">{t('settings.danger.export_title')}</h2>
			<p class="mb-3 text-sm text-[var(--color-muted)]">{t('settings.danger.export_desc')}</p>
			{#if exportRequested}
				<p class="text-sm text-green-600">{t('settings.danger.export_requested')}</p>
			{:else}
				{#if exportError}<p class="mb-2 text-xs text-[var(--color-danger,#dc2626)]">{exportError}</p>{/if}
				<button
					type="button"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					onclick={doExport}
				>
					{t('settings.danger.export_button')}
				</button>
			{/if}
		</div>

		<!-- Deactivate -->
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-1 text-sm font-medium text-[var(--color-text)]">{t('settings.danger.deactivate_title')}</h2>
			<p class="mb-3 text-sm text-[var(--color-muted)]">{t('settings.danger.deactivate_desc')}</p>
			<Modal
				bind:open={deactivateOpen}
				onOpenChange={(v) => { if (!v) { deactivatePassword = ''; deactivateError = null; } }}
				title={t('settings.danger.deactivate_confirm_title')}
				description={t('settings.danger.deactivate_confirm_body')}
				triggerClass="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				size="md"
			>
				{#snippet trigger()}
					{t('settings.danger.deactivate_button')}
				{/snippet}
				{#snippet children(close)}
						<!-- The backend requires the account password — it verifies
						     ownership before deactivating (missing_password → 400). -->
						<label class="mb-4 block">
							<span class="mb-1 block text-sm text-[var(--color-muted)]">
								{t('settings.danger.deactivate_password', 'Confirm with your password')}
							</span>
							<input
								type="password"
								autocomplete="current-password"
								aria-describedby="deactivate-password-hint"
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
								value={deactivatePassword}
								oninput={(e) => (deactivatePassword = e.currentTarget.value)}
							/>
							<HintIcon id="deactivate-password-hint" text={t('settings.danger.deactivate_password_hint', 'Enter your current password to confirm deactivation.')} />
						</label>
						{#if deactivateError}<p class="mb-4 text-xs text-[var(--color-danger,#dc2626)]">{deactivateError}</p>{/if}
						<div class="flex justify-end gap-2">
							<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
								{t('common.cancel')}
							</button>
							<button
								type="button"
								disabled={deactivating || !deactivatePassword}
								class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
								onclick={doDeactivate}
							>
								{deactivating ? t('common.submitting', 'Submitting…') : t('settings.danger.deactivate_confirm')}
							</button>
						</div>
				{/snippet}
			</Modal>
		</div>

		<!-- Delete account -->
		<div class="rounded-[var(--radius)] border border-[var(--color-danger,#dc2626)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-1 text-sm font-medium text-[var(--color-danger,#dc2626)]">{t('settings.danger.delete_title')}</h2>
			<p class="mb-3 text-sm text-[var(--color-muted)]">{t('settings.danger.delete_desc')}</p>
			<button
				type="button"
				class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-2 text-sm font-medium text-white"
				onclick={() => (deleteOpen = true)}
			>
				{t('settings.danger.delete_button')}
			</button>
		</div>
	</div>

	<DeleteAccountDialog bind:open={deleteOpen} {username} />
</section>
