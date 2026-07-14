<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { Dialog } from 'bits-ui';

	let {
		open = $bindable(false),
		title,
		children,
		onConfirm,
		confirmLabel = t('common.confirm'),
		onclose
	}: {
		open?: boolean;
		title: string;
		children?: Snippet;
		onConfirm?: () => void;
		confirmLabel?: string;
		onclose?: () => void;
	} = $props();

	function handleClose() {
		open = false;
		onclose?.();
	}
</script>

<Dialog.Root bind:open={open}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-lg data-[state=open]:animate-in data-[state=open]:fade-in data-[state=open]:zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out data-[state=closed]:zoom-out-95" style="border-color: var(--color-border); background-color: var(--color-background)">
			<Dialog.Title class="text-lg font-semibold">{title}</Dialog.Title>
			<div class="mt-4">
				{#if children}
					{@render children()}
				{/if}
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<Dialog.Close>
					{#snippet child({ props })}
						<button
							{...props}
							class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
							style="border-color: var(--color-border)"
							onclick={handleClose}
						>
							{t('common.cancel')}
						</button>
					{/snippet}
				</Dialog.Close>
				{#if onConfirm}
					<Dialog.Close>
						{#snippet child({ props })}
							<button
								{...props}
								class="rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors"
								style="background-color: var(--color-primary)"
								onclick={() => onConfirm()}
							>
								{confirmLabel}
							</button>
						{/snippet}
					</Dialog.Close>
				{/if}
			</div>
			<Dialog.Close>
				{#snippet child({ props })}
					<button
						{...props}
						class="absolute right-4 top-4 rounded-md p-1 hover:bg-black/5 dark:hover:bg-white/10"
						style="color: var(--color-muted)"
						aria-label={t('common.close')}
					>
						×
					</button>
				{/snippet}
			</Dialog.Close>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
