<script lang="ts">
	import { Sparkles } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		disabled?: boolean;
		onclick?: () => void;
		class?: string;
	}

	let { disabled = false, onclick, class: klass = '' }: Props = $props();
</script>

<button
	type="button"
	{disabled}
	onclick={() => onclick?.()}
	class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50 {klass}"
>
	<Sparkles size={12} class="text-[var(--color-primary)]" />
	{t('ai.body.button_label', 'japa AI')}
</button>
