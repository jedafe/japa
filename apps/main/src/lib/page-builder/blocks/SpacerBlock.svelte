<script lang="ts">
	interface Props {
		height?: number;
		style?: 'space' | 'line' | 'dots';
		readOnly?: boolean;
	}

	let {
		height = 32,
		style = 'space',
		readOnly = false,
	}: Props = $props();

	let heightStyle = $derived(`height: ${height}px`);
	let wrapperClass = $derived(
		`block-spacer${!readOnly ? ' ring-2 ring-blue-400/20 hover:ring-blue-400/40' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if style === 'space'}
		<div style={heightStyle}></div>
	{:else if style === 'line'}
		<div class="flex items-center" style={heightStyle}>
			<hr class="w-full border-gray-200 dark:border-gray-700" />
		</div>
	{:else if style === 'dots'}
		<div class="flex items-center justify-center gap-2" style={heightStyle}>
			<span class="h-1.5 w-1.5 rounded-full bg-gray-300 dark:bg-gray-600"></span>
			<span class="h-1.5 w-1.5 rounded-full bg-gray-300 dark:bg-gray-600"></span>
			<span class="h-1.5 w-1.5 rounded-full bg-gray-300 dark:bg-gray-600"></span>
		</div>
	{/if}
</div>
