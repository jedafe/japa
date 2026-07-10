<script lang="ts">
	import clsx from 'clsx';

	let { label, value, delta, icon, class: className = '' } = $props();
</script>

<div class={clsx('rounded-lg border p-4', 'border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-800', className)}>
	<div class="flex items-center justify-between">
		<span class="text-sm font-medium text-gray-500 dark:text-gray-400">{label}</span>
		{#if icon}
			<span class="text-gray-400">{@render icon()}</span>
		{/if}
	</div>
	<div class="mt-2">
		<span class="text-2xl font-bold text-gray-900 dark:text-gray-100">{value}</span>
		{#if delta !== undefined}
			<span class={clsx('ml-2 text-sm font-medium', delta >= 0 ? 'text-green-600' : 'text-red-600')}>
				{delta >= 0 ? '+' : ''}{delta}%
			</span>
		{/if}
	</div>
</div>
