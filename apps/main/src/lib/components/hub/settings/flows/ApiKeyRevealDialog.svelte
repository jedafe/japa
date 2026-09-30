<script lang="ts">
	import { Modal } from '$lib/components/ui';
	import { t } from '$lib/i18n';

	interface Props {
		open: boolean;
		apiKey: string;
		onClose: () => void;
	}
	let { open = $bindable(), apiKey, onClose }: Props = $props();

	let acknowledged = $state(false);

	function close() {
		if (!acknowledged) return;
		open = false;
		onClose();
		acknowledged = false;
	}

	async function copyKey() {
		try {
			await navigator.clipboard.writeText(apiKey);
		} catch {
			// clipboard may be unavailable; the key is still visible for manual copy
		}
	}
</script>

<Modal bind:open title={t('settings.apikeys.reveal_title')} description={t('settings.apikeys.reveal_warning')} size="md">
		<div class="mb-4 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-3">
				<code class="block break-all text-sm text-[var(--color-text)]">{apiKey}</code>
			</div>
			<div class="mb-4 flex items-center gap-2">
				<button
					type="button"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					onclick={copyKey}
				>
					{t('settings.apikeys.copy')}
				</button>
			</div>
			<label class="mb-4 flex items-center gap-2">
				<input
					type="checkbox"
					class="h-4 w-4 rounded border-[var(--color-border)]"
					checked={acknowledged}
					onchange={(e) => (acknowledged = e.currentTarget.checked)}
				/>
				<span class="text-sm text-[var(--color-muted)]">{t('settings.apikeys.ack')}</span>
			</label>
			<div class="flex justify-end">
				<button
					type="button"
					disabled={!acknowledged}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
					onclick={close}
				>
					{t('settings.apikeys.done')}
				</button>
		</div>
</Modal>
