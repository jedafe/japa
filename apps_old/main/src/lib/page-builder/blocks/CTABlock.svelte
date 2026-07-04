<script lang="ts">
	interface Props {
		text?: string;
		url?: string;
		style?: 'primary' | 'secondary' | 'outline';
		variant?: string;
		readOnly?: boolean;
	}

	let {
		text = '',
		url = '',
		style = 'primary',
		readOnly = false,
	}: Props = $props();

	const styleClasses: Record<string, string> = {
		primary: 'bg-blue-600 text-white hover:bg-blue-700 focus:ring-blue-500',
		secondary: 'bg-gray-600 text-white hover:bg-gray-700 focus:ring-gray-500',
		outline: 'border-2 border-blue-600 text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-950 focus:ring-blue-500',
	};

	let buttonClass = $derived(styleClasses[style] ?? styleClasses.primary);
	let wrapperClass = $derived(
		`block-cta flex items-center justify-center py-6${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if text && url}
		<a
			href={url}
			class="inline-flex items-center rounded-lg px-8 py-3 text-base font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 {buttonClass}"
		>
			{text}
		</a>
	{:else if text}
		<button
			type="button"
			class="inline-flex items-center rounded-lg px-8 py-3 text-base font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 {buttonClass}"
		>
			{text}
		</button>
	{:else}
		<div class="rounded-lg border-2 border-dashed border-gray-300 px-8 py-3 text-sm text-gray-400 dark:border-gray-600 dark:text-gray-500">
			CTA button — add text and URL
		</div>
	{/if}
</div>
