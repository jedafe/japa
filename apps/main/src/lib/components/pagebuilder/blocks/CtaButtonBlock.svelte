<script lang="ts">
	import type { CtaButtonProps } from '../types';

	interface Props {
		props: CtaButtonProps;
	}
	let { props }: Props = $props();

	const align = $derived(props.align ?? 'center');
	const style = $derived(props.style ?? 'primary');

	const styleClass = $derived(
		style === 'primary'
			? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'
			: style === 'secondary'
				? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)] hover:bg-[var(--color-surface)]'
				: 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
	);
</script>

<section class="mx-auto max-w-3xl px-6 py-8" class:text-left={align === 'left'} class:text-center={align === 'center'} class:text-right={align === 'right'}>
	<a
		href={props.url}
		target={props.open_new_tab ? '_blank' : undefined}
		rel={props.open_new_tab ? 'noopener noreferrer' : undefined}
		class="inline-flex items-center justify-center rounded-[var(--radius)] px-6 py-3 text-sm font-semibold transition-colors {styleClass}"
	>
		{props.text}
	</a>
</section>
