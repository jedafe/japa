<script lang="ts">
	import { t } from '$lib/i18n';
	import { statusLabel } from './gate';
	import type { ComposerStatus } from '$lib/types';

	interface Props {
		status: ComposerStatus;
		savedAt: number | null;
		canPublish: boolean;
		publishReason: string | null;
		onSaveDraft: () => void;
		onPublish: () => void;
	}

	let { status, canPublish, publishReason, onSaveDraft, onPublish }: Props = $props();
</script>

<div
	class="sticky bottom-0 flex items-center justify-between gap-3 border-t border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3"
>
	<span class="text-sm text-[var(--color-muted)]">{t(statusLabel(status))}</span>
	<div class="flex items-center gap-2">
		{#if !canPublish && publishReason}
			<span class="text-xs text-[var(--color-muted)]">{t(publishReason)}</span>
		{/if}
		<button
			type="button"
			class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			onclick={onSaveDraft}
		>
			{t('create.action.save_draft')}
		</button>
		<button
			type="button"
			disabled={!canPublish}
			onclick={onPublish}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
		>
			{t('create.action.publish')}
		</button>
	</div>
</div>
