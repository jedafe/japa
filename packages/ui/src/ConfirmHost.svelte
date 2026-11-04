<script lang="ts">
	// ConfirmHost — renders the pending confirmDialog() request as a small
	// centered dialog. Mount once in the root layout; anywhere in the app can
	// then `await confirmDialog(...)` instead of window.confirm().
	import { AlertTriangle } from 'lucide-svelte';
	import Modal from './Modal.svelte';
	import { getConfirmRequest, answerConfirm } from './confirm.svelte';
</script>

{#if getConfirmRequest()}
	{@const req = getConfirmRequest()!}
	<Modal open={true} title={req.title} size="sm" onOpenChange={(o) => { if (!o) answerConfirm(false); }}>
		<div class="flex items-start gap-3">
			{#if req.destructive}
				<AlertTriangle
					size={20}
					class="mt-0.5 shrink-0"
					style="color: var(--color-danger, var(--color-destructive, #ef4444))"
				/>
			{/if}
			<p class="text-sm" style="color: var(--color-text, var(--color-foreground))">{req.message}</p>
		</div>
		<div class="mt-5 flex justify-end gap-2">
			<button
				type="button"
				onclick={() => answerConfirm(false)}
				class="rounded-[var(--radius,var(--radius-md,8px))] border px-4 py-2 text-sm font-medium transition-colors"
				style="border-color: var(--color-border, var(--color-border-input, #374151)); color: var(--color-text, var(--color-foreground)); background: transparent"
			>
				{req.cancelLabel}
			</button>
			<button
				type="button"
				onclick={() => answerConfirm(true)}
				class="rounded-[var(--radius,var(--radius-md,8px))] px-4 py-2 text-sm font-medium text-white transition-colors"
				style="background: {req.destructive
					? 'var(--color-danger, var(--color-destructive, #ef4444))'
					: 'var(--color-primary, var(--color-accent, #3b82f6))'}"
			>
				{req.confirmLabel}
			</button>
		</div>
	</Modal>
{/if}
