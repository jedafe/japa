<script lang="ts">
	import { Dialog } from 'bits-ui';

	let { open = $bindable(false), title, consequence, confirmText = '', onConfirm, onCancel } = $props();

	let typedText = $state('');

	function handleConfirm() {
		if (confirmText && typedText !== confirmText) return;
		onConfirm?.();
		open = false;
		typedText = '';
	}

	function handleCancel() {
		onCancel?.();
		open = false;
		typedText = '';
	}
</script>

<Dialog.Root bind:open={open} onOpenChange={(isOpen) => { if (!isOpen) handleCancel(); }}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
		<Dialog.Content
			class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border border-gray-200 bg-white p-6 shadow-xl focus:outline-none dark:border-gray-700 dark:bg-gray-800"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-lg font-semibold text-gray-900 dark:text-gray-100">
				{title}
			</Dialog.Title>
			{#if consequence}
				<div class="mt-3 rounded-md bg-red-50 p-3 dark:bg-red-900/20">
					<p class="text-sm text-red-700 dark:text-red-300">{consequence}</p>
				</div>
			{/if}
			{#if confirmText}
				<div class="mt-4">
					<label for="confirm-input" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
						Type <code class="rounded bg-gray-100 px-1 dark:bg-gray-700">{confirmText}</code> to confirm
					</label>
					<input
						id="confirm-input"
						type="text"
						bind:value={typedText}
						class="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm dark:border-gray-600 dark:bg-gray-700 dark:text-gray-100"
					/>
				</div>
			{/if}
			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close
					class="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
					onclick={handleCancel}
				>
					Cancel
				</Dialog.Close>
				<button
					type="button"
					class="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
					disabled={confirmText !== '' && typedText !== confirmText}
					onclick={handleConfirm}
				>
					Confirm
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
