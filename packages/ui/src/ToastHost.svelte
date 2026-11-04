<script lang="ts">
	// ToastHost — renders the shared toast store as a fixed stack in the
	// bottom-end corner. Mount once in the root layout alongside the app's
	// shell; toasts from anywhere in the app appear here.
	//
	// Colours use the same var-fallback chain as EmptyState so the component
	// renders sanely on every app's token scheme (main/academy/docs use
	// --color-*; admin/control use both --color-* and shadcn-style names).
	import { CheckCircle, Info, AlertTriangle, XCircle, X } from 'lucide-svelte';
	import { getToasts, dismissToast, type ToastVariant } from './toast.svelte';

	const icons: Record<ToastVariant, typeof Info> = {
		info: Info,
		success: CheckCircle,
		warning: AlertTriangle,
		error: XCircle
	};

	const accent: Record<ToastVariant, string> = {
		info: 'var(--color-primary, var(--color-accent, #3b82f6))',
		success: 'var(--color-success, #22c55e)',
		warning: 'var(--color-warning, var(--color-tertiary, #eab308))',
		error: 'var(--color-danger, var(--color-destructive, #ef4444))'
	};
</script>

<div
	class="pointer-events-none fixed bottom-4 end-4 z-[9999] flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2"
	aria-live="polite"
>
	{#each getToasts() as item (item.id)}
		{@const Icon = icons[item.variant]}
		<div
			role={item.variant === 'error' || item.variant === 'warning' ? 'alert' : 'status'}
			class="pointer-events-auto flex items-start gap-2.5 rounded-lg border px-3 py-2.5 text-sm shadow-lg"
			style="background: var(--color-surface, var(--color-card, var(--color-background-alt, #1f2937))); border-color: var(--color-border, var(--color-border-input, #374151)); color: var(--color-text, var(--color-foreground, #f9fafb))"
		>
			<Icon size={17} class="mt-0.5 shrink-0" style="color: {accent[item.variant]}" />
			<span class="min-w-0 flex-1 break-words">{item.message}</span>
			<button
				type="button"
				onclick={() => dismissToast(item.id)}
				aria-label="Dismiss"
				class="shrink-0 opacity-60 transition-opacity hover:opacity-100"
			>
				<X size={15} />
			</button>
		</div>
	{/each}
</div>
