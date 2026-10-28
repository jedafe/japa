<script lang="ts">
	import { Modal } from '$lib/components/ui';
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

<Modal bind:open onOpenChange={(v) => { if (!v) reset(); }} title={t('settings.danger.delete_title')} size="md">
	{#snippet children(close)}
		<p class="mb-4 text-sm text-[var(--color-danger,#dc2626)]">
			{t('settings.danger.delete_warning')}
		</p>
		<p class="mb-2 text-sm text-[var(--color-text)]">
				{t('settings.danger.delete_confirm_prompt')} <strong>{username}</strong>
			</p>
			<input
				type="text"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				placeholder={username}
				aria-label={t('settings.danger.delete_confirm_prompt')}
				aria-describedby="delete-confirm-hint"
				value={typed}
				oninput={(e) => (typed = e.currentTarget.value)}
			/>
			<p id="delete-confirm-hint" class="mb-4 mt-1 text-xs text-[var(--color-muted)]">
				{t('settings.danger.delete_confirm_hint', 'Type your username exactly as shown to enable deletion.')}
			</p>
		{#if error}<p class="mb-4 text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
		<div class="flex justify-end gap-2">
			<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
				{t('common.cancel')}
			</button>
				<button
					type="button"
					disabled={!canConfirm || deleting}
					class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					onclick={doDelete}
				>
					{deleting ? t('settings.danger.deleting') : t('settings.danger.delete_confirm')}
			</button>
		</div>
	{/snippet}
</Modal>
