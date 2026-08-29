<script lang="ts">
	import { Dialog } from 'bits-ui';
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
		deactivating = true;
		deactivateError = null;
		const res = await deactivateAccount();
		deactivating = false;
		deactivateOpen = false;
		if (res.error) {
			deactivateError = res.error.message;
			return;
		}
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
			{#if deactivateError}<p class="mb-2 text-xs text-[var(--color-danger,#dc2626)]">{deactivateError}</p>{/if}
			<Dialog.Root bind:open={deactivateOpen}>
				<Dialog.Trigger
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('settings.danger.deactivate_button')}
				</Dialog.Trigger>
				<Dialog.Portal>
					<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
						<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
							{t('settings.danger.deactivate_confirm_title')}
						</Dialog.Title>
						<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">
							{t('settings.danger.deactivate_confirm_body')}
						</Dialog.Description>
						<div class="flex justify-end gap-2">
							<Dialog.Close class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
								{t('common.cancel')}
							</Dialog.Close>
							<button
								type="button"
								disabled={deactivating}
								class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
								onclick={doDeactivate}
							>
								{t('settings.danger.deactivate_confirm')}
							</button>
						</div>
					</Dialog.Content>
				</Dialog.Portal>
			</Dialog.Root>
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
