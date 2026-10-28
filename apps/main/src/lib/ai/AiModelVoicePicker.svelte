<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
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
		/** Optional tone list — renders a middle picker when non-empty. */
		toneOptions?: { value: string; label: string }[];
		model: string;
		voice: string;
		tone?: string;
		/** Optional id prefix when more than one picker lives on a page. */
		idPrefix?: string;
		/** Show visible labels above each select (default: sr-only labels). */
		showLabels?: boolean;
		/** Compact density — chip-height selects for the composer bottom bar. */
		compact?: boolean;
	}

	let {
		modelOptions,
		voiceOptions,
		toneOptions = [],
		model = $bindable(''),
		voice = $bindable(''),
		tone = $bindable(''),
		idPrefix = 'ai',
		showLabels = false,
		compact = false
	}: Props = $props();
</script>

{#if modelOptions.length > 0 || voiceOptions.length > 0 || toneOptions.length > 0}
	<div class="flex {compact ? 'items-center gap-1.5' : 'items-end gap-2'}">
		{#if modelOptions.length > 0}
			<div class="flex min-w-0 flex-1 flex-col gap-1">
				<label
					for="{idPrefix}-model"
					class={showLabels ? 'text-xs text-[var(--color-muted)]' : 'sr-only'}
					>{t('ai.picker.model_label', 'Model')}</label
				>
					<HintIcon text={t('ai.picker.model_hint', 'Which AI model to use.')} />
				<AutocompleteSelect id="{idPrefix}-model" bind:value={model} options={modelOptions} {compact} />
			</div>
		{/if}
		{#if toneOptions.length > 0}
			<div class="flex min-w-0 flex-1 flex-col gap-1">
				<label
					for="{idPrefix}-tone"
					class={showLabels ? 'text-xs text-[var(--color-muted)]' : 'sr-only'}
					>{t('ai.picker.tone_label', 'Tone')}</label
				>
					<HintIcon text={t('ai.picker.tone_hint', 'The writing tone for generated content.')} />
				<AutocompleteSelect id="{idPrefix}-tone" bind:value={tone} options={toneOptions} {compact} />
			</div>
		{/if}
		{#if voiceOptions.length > 0}
			<div class="flex min-w-0 flex-1 flex-col gap-1">
				<label
					for="{idPrefix}-voice"
					class={showLabels ? 'text-xs text-[var(--color-muted)]' : 'sr-only'}
					>{t('ai.picker.voice_label', 'Voice')}</label
				>
					<HintIcon text={t('ai.picker.voice_hint', 'The voice for audio output.')} />
				<AutocompleteSelect id="{idPrefix}-voice" bind:value={voice} options={voiceOptions} {compact} />
			</div>
		{/if}
	</div>
{/if}
