<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { X } from 'lucide-svelte';
	import { CheckCircle, AlertCircle, AlertTriangle, Info } from 'lucide-svelte';
	import { getToasts, dismissToast } from '$lib/stores/toast.svelte';
	import autoAnimate from '@formkit/auto-animate';

	let container: HTMLDivElement | undefined = $state();

	const icons: Record<string, typeof Info> = {
		success: CheckCircle,
		error: AlertCircle,
		warning: AlertTriangle,
		info: Info
	};

	const borderColors: Record<string, string> = {
		success: 'var(--color-success)',
		error: 'var(--color-danger)',
		warning: 'var(--color-warning)',
		info: 'var(--color-info)'
	};

	$effect(() => {
		if (container) {
			autoAnimate(container);
		}
	});
</script>

<div
	bind:this={container}
	class="fixed right-4 top-4 z-50 flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-2"
	role="region"
	aria-label="Notifications"
>
	{#each getToasts().current as toast (toast.id)}
		{@const Icon = icons[toast.type] ?? icons.info}
		<div
			class="flex items-start gap-3 rounded-lg border-l-4 p-4 shadow-lg"
			style="background-color: var(--color-surface); border-left-color: {borderColors[toast.type] ?? borderColors.info}"
			role="alert"
		>
			<Icon
				class="mt-0.5 h-5 w-5 shrink-0"
				style="color: {borderColors[toast.type] ?? borderColors.info}"
			/>
			<p class="flex-1 text-sm" style="color: var(--color-foreground)">{toast.message}</p>
			<button
				onclick={() => dismissToast(toast.id)}
				class="shrink-0 rounded p-0.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
				style="color: var(--color-muted)"
				aria-label="Dismiss"
			>
				<X class="h-4 w-4" />
			</button>
		</div>
	{/each}
</div>
