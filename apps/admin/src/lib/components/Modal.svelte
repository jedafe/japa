<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let {
		open = $bindable(false),
		title,
		children,
		onConfirm,
		confirmLabel = t('common.confirm')
	}: {
		open: boolean;
		title: string;
		children?: Snippet;
		onConfirm?: () => void;
		confirmLabel?: string;
	} = $props();

	function handleBackdropClick(e: MouseEvent) {
		if (e.target === e.currentTarget) {
			open = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') open = false;
	}
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		role="dialog"
		tabindex="-1"
		onclick={handleBackdropClick}
		onkeydown={(e) => { if (e.key === 'Escape') open = false; }}
	>
		<div class="w-full max-w-md rounded-lg border p-6 shadow-lg" style="border-color: var(--color-border); background-color: var(--color-background)">
			<h2 class="text-lg font-semibold">{title}</h2>
			<div class="mt-4">
				{#if children}
					{@render children()}
				{/if}
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="border-color: var(--color-border)"
					onclick={() => (open = false)}
				>
					{t('common.cancel')}
				</button>
				{#if onConfirm}
					<button
						class="rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors"
						style="background-color: var(--color-primary)"
						onclick={() => { onConfirm(); open = false; }}
					>
						{confirmLabel}
					</button>
				{/if}
			</div>
		</div>
	</div>
{/if}
