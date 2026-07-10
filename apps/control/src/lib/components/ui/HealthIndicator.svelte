<script lang="ts">
	import clsx from 'clsx';

	let { variant = 'default', class: className = '' } = $props();

	const variants: Record<string, string> = {
		green: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
		yellow: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200',
		red: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
		default: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
	};
</script>

<div class={clsx('flex items-center gap-2', className)}>
	<span class={clsx('h-2.5 w-2.5 rounded-full', variants[variant]?.split(' ')[0])}></span>
	<span class="text-sm text-gray-600 dark:text-gray-300"><slot /></span>
</div>
