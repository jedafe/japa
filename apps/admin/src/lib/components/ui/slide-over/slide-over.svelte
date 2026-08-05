<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Dialog } from 'bits-ui';
	import { X } from 'lucide-svelte';
	import { clsx, type ClassValue } from 'clsx';
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		title,
		description,
		side = 'right',
		size = 'default',
		class: className,
		children,
		footer,
		onclose
	}: {
		open?: boolean;
		title?: string;
		description?: string;
		side?: 'right' | 'left';
		size?: 'sm' | 'default' | 'lg' | 'xl';
		class?: ClassValue;
		children?: Snippet;
		footer?: Snippet;
		onclose?: () => void;
	} = $props();

	const widths = {
		sm: 'w-[384px]',
		default: 'w-[480px]',
		lg: 'w-[640px]',
		xl: 'w-[832px]'
	};

	function handleClose() {
		open = false;
		onclose?.();
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay
			class="fixed inset-0 z-50 bg-black/60 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=closed]:animate-out data-[state=closed]:fade-out-0"
		/>
		<Dialog.Content
			data-slideover-content
			class={clsx(
				'fixed inset-y-0 z-50 flex h-full flex-col border shadow-xl duration-300 data-[state=open]:animate-in data-[state=closed]:animate-out',
				side === 'right'
					? clsx(
							'right-0 data-[state=open]:slide-in-from-right data-[state=closed]:slide-out-to-right'
						)
					: clsx(
							'left-0 data-[state=open]:slide-in-from-left data-[state=closed]:slide-out-to-left'
						),
				widths[size],
				className
			)}
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			<div
				class="flex items-start justify-between border-b px-5 py-4"
				style="border-color: var(--color-border)"
			>
				<div class="flex flex-col gap-1 pr-6">
					{#if title}
						<Dialog.Title
							class="text-base font-semibold leading-none"
							style="color: var(--color-foreground)">{title}</Dialog.Title
						>
					{/if}
					{#if description}
						<Dialog.Description class="text-sm" style="color: var(--color-muted)"
							>{description}</Dialog.Description
						>
					{/if}
				</div>
				<Dialog.Close
					class="absolute right-4 top-4 rounded-lg p-1 opacity-70 transition-opacity hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:pointer-events-none"
					style="color: var(--color-muted)"
					onclick={handleClose}
					aria-label={t('lib.components.ui.modal.modal.svelte.close', 'Close')}
				>
					<X class="h-4 w-4" />
					<span class="sr-only">
						{t('lib.components.ui.modal.modal.svelte.close', 'Close')}
					</span>
				</Dialog.Close>
			</div>
			<div class="flex-1 overflow-y-auto px-5 py-4">
				{@render children?.()}
			</div>
			{#if footer}
				<div
					class="flex items-center justify-end gap-2 border-t px-5 py-3"
					style="border-color: var(--color-border)"
				>
					{@render footer()}
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
