<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// EditAiKeyDialog — rename a key's label and/or rotate its credential.
	// Rotating resets status/last_error server-side after a re-verify
	// (PATCH /me/ai-keys/{id}).
	//
	// Spec: docs/superpowers/specs/2026-10-19-ai-byok-design.md
	import { t } from '$lib/i18n';
	import { Modal } from '$lib/components/ui';
	import { updateAIKey } from '$lib/api/byok';
	import type { UserAIKeyView } from '$lib/ai/agentTypes';

	interface Props {
		open: boolean;
		keyEntry: UserAIKeyView | null;
		onclose?: () => void;
		onsaved?: () => void;
	}

	// `open` is driven by the parent (editTarget !== null); every dismiss
	// path funnels through close() → onclose so the parent clears its target
	// (same pattern as ApiKeysSection's confirm modals).
	let { open, keyEntry, onclose, onsaved }: Props = $props();

	let label = $state('');
	let apiKey = $state('');
	let saving = $state(false);
	let errorMessage = $state('');

	function reset() {
		label = keyEntry?.label ?? '';
		apiKey = '';
		errorMessage = '';
		saving = false;
	}

	function close() {
		onclose?.();
	}

	// Seed the label from the target key each time the dialog opens.
	let wasOpen = $state(false);
	let seededFor = $state('');
	$effect(() => {
		if (open && (!wasOpen || seededFor !== keyEntry?.id)) {
			seededFor = keyEntry?.id ?? '';
			reset();
		}
		wasOpen = open;
	});

	const dirty = $derived(label.trim() !== (keyEntry?.label ?? '') || apiKey.trim() !== '');

	async function save() {
		if (!keyEntry || saving || !dirty) return;
		saving = true;
		errorMessage = '';
		const res = await updateAIKey(keyEntry.id, {
			...(label.trim() !== (keyEntry.label ?? '') ? { label: label.trim() } : {}),
			...(apiKey.trim() ? { api_key: apiKey.trim() } : {})
		});
		saving = false;
		if (res.error || !res.data) {
			errorMessage = res.error?.message ?? t('ai.byok.save_failed', 'Could not save the key');
			return;
		}
		onsaved?.();
		close();
	}
</script>

<Modal {open} onOpenChange={(v) => { if (!v) close(); }} title={t('ai.byok.edit_key', 'Edit API key')} description={keyEntry?.provider?.display_name}>
	<div class="space-y-4">
		<div>
			<label for="byok-edit-label" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('ai.byok.label', 'Label (optional)')}</label
			>
			<input
				id="byok-edit-label"
				type="text"
				bind:value={label}
				maxlength="120"
				placeholder={t('ai.byok.label_hint', 'e.g. My OpenAI')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			/>
				<p class="mt-1 text-xs text-[var(--color-muted)]">{t('ai.byok.label_desc', 'A name so you remember what this key is for.')}</p>
		</div>

		<div>
			<label for="byok-edit-key" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('ai.byok.new_api_key', 'New API key (optional — rotates the credential)')}</label
			>
			<input
				id="byok-edit-key"
				type="password"
				bind:value={apiKey}
				autocomplete="off"
				maxlength="4096"
				placeholder={keyEntry?.api_key_masked ?? 'sk-…'}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			/>
				<HintIcon text={t('ai.byok.key_hint', 'Stored encrypted — used only for your AI features.')} />
		</div>

		{#if errorMessage}
			<p class="text-sm text-[var(--color-destructive)]">{errorMessage}</p>
		{/if}

		<div class="flex justify-end gap-2">
			<button
				type="button"
				onclick={close}
				class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)]"
				>{t('common.cancel', 'Cancel')}</button
			>
			<button
				type="button"
				onclick={save}
				disabled={saving || !dirty}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-foreground)] disabled:opacity-50"
			>
				{saving ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
			</button>
		</div>
	</div>
</Modal>
