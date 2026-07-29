<script lang="ts">
	import { t } from '$lib/i18n';

	interface Props {
		/** Ad slot identifier, surfaced as the label. */
		slot: string;
		format?: 'banner' | 'square' | 'inline';
	}

	let { slot, format = 'banner' }: Props = $props();

	const dimensions = {
		banner: 'h-[90px] w-full',
		square: 'h-[250px] w-[300px]',
		inline: 'h-[60px] w-full'
	} as const;

	let sizeClass = $derived(dimensions[format]);
</script>

<div
	class="grid place-items-center rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-xs text-[var(--color-muted)] {sizeClass}"
	data-ad-slot={slot}
	aria-label={t('ads.label', 'Advertisement')}
>
	<span>{t('ads.label', 'Advertisement')} · {slot}</span>
</div>
