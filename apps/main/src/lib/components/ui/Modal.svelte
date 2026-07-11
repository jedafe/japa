<script lang="ts">
	import { Dialog } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		title = '',
		description = '',
		class: className = '',
		children,
		footer
	}: {
		open?: boolean;
		title?: string;
		description?: string;
		class?: string;
		children?: Snippet;
		footer?: Snippet;
	} = $props();
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50" />
		<Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border bg-[var(--color-surface)] p-6 shadow-lg {className}" style="border-color: var(--color-border)">
			{#if title}
				<Dialog.Title class="text-lg font-semibold text-[var(--color-foreground)]">
					{title}
				</Dialog.Title>
			{/if}
			{#if description}
				<Dialog.Description class="mt-1 text-sm text-[var(--color-muted)]">
					{description}
				</Dialog.Description>
			{/if}
			<div class="mt-4">
				{@render children?.()}
			</div>
			{#if footer}
				<div class="mt-6 flex justify-end gap-2">
					{@render footer?.()}
				</div>
			{/if}
			<Dialog.Close class="absolute right-4 top-4 text-[var(--color-muted)] hover:text-[var(--color-foreground)]">
				<svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
					<path d="M18 6L6 18M6 6l12 12" />
				</svg>
			</Dialog.Close>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
