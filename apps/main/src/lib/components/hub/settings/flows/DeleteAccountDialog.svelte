<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/i18n';
	import { deleteAccount } from '$lib/api/hub';
	import { goto } from '$app/navigation';
	import { matchesConfirmation } from '../danger';

	interface Props {
		open: boolean;
		username: string;
	}
	let { open = $bindable(), username }: Props = $props();

	let typed = $state('');
	let deleting = $state(false);
	let error = $state<string | null>(null);

	const canConfirm = $derived(matchesConfirmation(typed, username));

	async function doDelete() {
		if (!canConfirm) return;
		deleting = true;
		error = null;
		const res = await deleteAccount();
		deleting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		// Terminal action — hard redirect to the logged-out landing.
		open = false;
		await goto('/settings/logged-out');
	}

	function reset() {
		typed = '';
		error = null;
		deleting = false;
	}
</script>

<Dialog.Root bind:open onOpenChange={(v) => { if (!v) reset(); }}>
	<Dialog.Portal>
		<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
			<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
				{t('settings.danger.delete_title')}
			</Dialog.Title>
			<Dialog.Description class="mb-4 text-sm text-[var(--color-danger,#dc2626)]">
				{t('settings.danger.delete_warning')}
			</Dialog.Description>
			<p class="mb-2 text-sm text-[var(--color-text)]">
				{t('settings.danger.delete_confirm_prompt')} <strong>{username}</strong>
			</p>
			<input
				type="text"
				class="mb-4 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				placeholder={username}
				value={typed}
				oninput={(e) => (typed = e.currentTarget.value)}
			/>
			{#if error}<p class="mb-4 text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
			<div class="flex justify-end gap-2">
				<Dialog.Close class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
					{t('common.cancel')}
				</Dialog.Close>
				<button
					type="button"
					disabled={!canConfirm || deleting}
					class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					onclick={doDelete}
				>
					{deleting ? t('settings.danger.deleting') : t('settings.danger.delete_confirm')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
