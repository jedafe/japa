<script lang="ts">
	import clsx from 'clsx';

	let { variant = 'default', size = 'default', class: className = '', children } = $props();

	const variants: Record<string, string> = {
		default: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
		primary: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
		success: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
		warning: 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200',
		danger: 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200',
		info: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900 dark:text-cyan-200',
	};

	const sizes: Record<string, string> = {
		sm: 'px-2 py-0.5 text-xs',
		default: 'px-2.5 py-0.5 text-xs',
		lg: 'px-3 py-1 text-sm',
	};
</script>

<span class={clsx('inline-flex items-center rounded-full font-medium', variants[variant], sizes[size], className)}>
	{@render children()}
</span>
