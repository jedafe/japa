<script lang="ts">
	import { t } from '$lib/i18n';

	interface Props {
		mode: string;
	}

	let { mode = $bindable() }: Props = $props();

	const modes = ['none', 'eligibility', 'cost', 'pathway'] as const;
</script>

<div
	class="flex gap-1 rounded-[var(--radius-button)] border p-1"
	style="border-color: var(--color-border); background-color: var(--color-surface)"
>
	{#each modes as m (m)}
		<button
			type="button"
			class="rounded-[var(--radius-button)] px-2.5 py-1.5 text-xs font-medium transition-colors"
			class:font-bold={mode === m}
			style="background-color: {mode === m ? 'var(--color-bg)' : 'transparent'}; color: var(--color-text)"
			onclick={() => (mode = m)}
		>
			{t(`jce.smart_mode.${m}`)}
		</button>
	{/each}
</div>
