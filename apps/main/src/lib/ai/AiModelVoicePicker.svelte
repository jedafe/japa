<script lang="ts">
	import { AutocompleteSelect } from '$lib/components/ui';
	import { t } from '$lib/i18n';

	/**
	 * Shared model + voice pickers for the AI surfaces (JapaBot chat,
	 * content-assist panel). Renders nothing for a dimension whose option
	 * list is empty — the backend then applies its configured default.
	 */
	interface Props {
		modelOptions: { value: string; label: string }[];
		voiceOptions: { value: string; label: string }[];
		model: string;
		voice: string;
		/** Optional id prefix when more than one picker lives on a page. */
		idPrefix?: string;
		/** Show visible labels above each select (default: sr-only labels). */
		showLabels?: boolean;
	}

	let {
		modelOptions,
		voiceOptions,
		model = $bindable(''),
		voice = $bindable(''),
		idPrefix = 'ai',
		showLabels = false
	}: Props = $props();
</script>

{#if modelOptions.length > 0 || voiceOptions.length > 0}
	<div class="flex items-end gap-2">
		{#if modelOptions.length > 0}
			<div class="flex flex-1 flex-col gap-1">
				<label
					for="{idPrefix}-model"
					class={showLabels ? 'text-xs text-[var(--color-muted)]' : 'sr-only'}
					>{t('ai.picker.model_label', 'Model')}</label
				>
				<AutocompleteSelect id="{idPrefix}-model" bind:value={model} options={modelOptions} />
			</div>
		{/if}
		{#if voiceOptions.length > 0}
			<div class="flex flex-1 flex-col gap-1">
				<label
					for="{idPrefix}-voice"
					class={showLabels ? 'text-xs text-[var(--color-muted)]' : 'sr-only'}
					>{t('ai.picker.voice_label', 'Voice / Tone')}</label
				>
				<AutocompleteSelect id="{idPrefix}-voice" bind:value={voice} options={voiceOptions} />
			</div>
		{/if}
	</div>
{/if}
