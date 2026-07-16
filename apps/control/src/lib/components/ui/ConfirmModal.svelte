<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { t } from '$lib/stores/i18n';

	interface Props {
		open: boolean;
		title: string;
		consequence?: string;
		confirmText?: string;
		onConfirm?: () => void;
		onCancel?: () => void;
	}

	let { open = $bindable(false), title, consequence, confirmText = '', onConfirm, onCancel }: Props = $props();

	let typedText = $state('');
	let confirming = $state(false);

	function handleConfirm() {
		if (confirmText && typedText !== confirmText) return;
		confirming = true;
		onConfirm?.();
		open = false;
		typedText = '';
		confirming = false;
	}

	function handleCancel() {
		if (confirming) return;
		onCancel?.();
		open = false;
		typedText = '';
	}
</script>

<Dialog.Root bind:open={open} onOpenChange={(isOpen) => { if (!isOpen) handleCancel(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 focus:outline-none"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				{title}
			</Dialog.Title>
			{#if consequence}
				<div class="bg-destructive/10 mt-3 rounded-md p-3">
					<p class="text-destructive text-sm">{consequence}</p>
				</div>
			{/if}
			{#if confirmText}
				<div class="mt-4">
					<label for="confirm-input" class="text-foreground block text-sm font-medium">
						{t('common.type_to_confirm', 'Type')} <code class="bg-muted rounded px-1">{confirmText}</code> {t('common.to_confirm', 'to confirm')}
					</label>
					<input
						id="confirm-input"
						type="text"
						bind:value={typedText}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-md border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
				</div>
			{/if}
			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-md border px-4 py-2 text-sm font-medium transition-colors"
					onclick={handleCancel}
				>
					{t('common.cancel')}
				</Dialog.Close>
				<button
					type="button"
					class="bg-destructive text-white rounded-md px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					disabled={confirmText !== '' && typedText !== confirmText}
					onclick={handleConfirm}
				>
					{t('common.confirm')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
