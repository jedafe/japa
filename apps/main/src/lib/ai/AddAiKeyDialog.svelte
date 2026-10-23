<script lang="ts">
	// AddAiKeyDialog — attach a personal AI provider key (BYOK). The
	// "Get an API key →" link opens the provider's admin-configured
	// affiliate signup_url and fires the referral-click counter.
	//
	// Spec: docs/superpowers/specs/2026-10-19-ai-byok-design.md
	import { t } from '$lib/i18n';
	import { Modal } from '$lib/components/ui';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { createAIKey, verifyAIKey, trackByokReferral } from '$lib/api/byok';
	import type { ByokProviderOption } from '$lib/ai/agentTypes';
	import { ExternalLink } from 'lucide-svelte';

	interface Props {
		open: boolean;
		providers: ByokProviderOption[];
		onclose?: () => void;
		onsaved?: () => void;
	}

	let { open = $bindable(false), providers, onclose, onsaved }: Props = $props();

	let providerId = $state('');
	let apiKey = $state('');
	let label = $state('');
	let saving = $state(false);
	let verifying = $state(false);
	let errorMessage = $state('');

	const providerOptions = $derived(
		providers.map((p) => ({ value: p.id, label: p.display_name }))
	);
	const selectedProvider = $derived(providers.find((p) => p.id === providerId));

	function reset() {
		providerId = '';
		apiKey = '';
		label = '';
		errorMessage = '';
		saving = false;
		verifying = false;
	}

	function close() {
		open = false;
		onclose?.();
	}

	// Reset the form whenever the dialog closes (backdrop, X, or close()).
	let wasOpen = $state(false);
	$effect(() => {
		if (open && !wasOpen) reset();
		wasOpen = open;
	});

	async function save() {
		if (!providerId || !apiKey.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const res = await createAIKey({
				byok_provider_id: providerId,
				api_key: apiKey.trim(),
				label: label.trim() || undefined
			});
			if (res.error || !res.data) {
				errorMessage = res.error?.message ?? t('ai.byok.save_failed', 'Could not save the key');
				return;
			}
			// Best-effort verify — an unreachable provider doesn't block save.
			const keyId = res.data.id;
			verifying = true;
			try {
				await verifyAIKey(keyId);
			} catch {
				/* status stays whatever the backend recorded */
			}
			verifying = false;
			saving = false;
			onsaved?.();
			close();
		} finally {
			saving = false;
			verifying = false;
		}
	}
</script>

<Modal bind:open title={t('ai.byok.add_key', 'Add API key')}>
	<div class="space-y-4">
		<div>
			<label for="byok-provider" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('ai.byok.provider', 'Provider')}</label
			>
			<AutocompleteSelect
				id="byok-provider"
				bind:value={providerId}
				options={providerOptions}
			/>
			{#if providers.length === 0}
				<p class="mt-1.5 text-sm text-[var(--color-muted)]">{t('ai.byok.no_providers', 'No AI providers are available right now.')}</p>
			{/if}
			{#if selectedProvider?.signup_url}
				<a
					href={selectedProvider.signup_url}
					target="_blank"
					rel="noopener"
					onclick={() => trackByokReferral(selectedProvider.id)}
					class="mt-1.5 inline-flex items-center gap-1 text-sm text-[var(--color-primary)] hover:underline"
				>
					{selectedProvider.signup_label || t('ai.byok.get_key', 'Get an API key')}
					<ExternalLink class="h-3.5 w-3.5 rtl:-scale-x-100" />
				</a>
			{/if}
		</div>

		<div>
			<label for="byok-key" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('ai.byok.api_key', 'API key')}</label
			>
			<input
				id="byok-key"
				type="password"
				bind:value={apiKey}
				autocomplete="off"
				maxlength="4096"
				placeholder="sk-…"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			/>
		</div>

		<div>
			<label for="byok-label" class="mb-1 block text-sm font-medium text-[var(--color-text)]"
				>{t('ai.byok.label', 'Label (optional)')}</label
			>
			<input
				id="byok-label"
				type="text"
				bind:value={label}
				maxlength="120"
				placeholder={t('ai.byok.label_hint', 'e.g. My OpenAI')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
			/>
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
				disabled={saving || !providerId || !apiKey.trim()}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-foreground)] disabled:opacity-50"
			>
				{verifying
					? t('ai.byok.verifying', 'Verifying…')
					: saving
						? t('common.saving', 'Saving…')
						: t('ai.byok.save', 'Save key')}
			</button>
		</div>
	</div>
</Modal>
