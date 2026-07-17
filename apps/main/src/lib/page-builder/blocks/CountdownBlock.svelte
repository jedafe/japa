<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	interface Props {
		target_date?: string;
		title?: string;
		show_days?: boolean;
		show_hours?: boolean;
		show_minutes?: boolean;
		show_seconds?: boolean;
		expired_text?: string;
		timezone?: string;
		readOnly?: boolean;
	}

	let {
		target_date = '',
		title = '',
		show_days = true,
		show_hours = true,
		show_minutes = true,
		show_seconds = true,
		expired_text = 'Event has started!',
		readOnly = false,
	}: Props = $props();

	let timeLeft = $state({ days: 0, hours: 0, minutes: 0, seconds: 0, expired: true });
	let intervalId = $state<number | null>(null);

	function calculateTimeLeft() {
		if (!target_date) return;
		const difference = new Date(target_date).getTime() - Date.now();
		if (difference <= 0) {
			timeLeft = { days: 0, hours: 0, minutes: 0, seconds: 0, expired: true };
			return;
		}
		timeLeft = {
			days: Math.floor(difference / (1000 * 60 * 60 * 24)),
			hours: Math.floor((difference / (1000 * 60 * 60)) % 24),
			minutes: Math.floor((difference / (1000 * 60)) % 60),
			seconds: Math.floor((difference / 1000) % 60),
			expired: false,
		};
	}

	$effect(() => {
		if (target_date) {
			calculateTimeLeft();
			intervalId = window.setInterval(calculateTimeLeft, 1000);
		}
		return () => {
			if (intervalId) clearInterval(intervalId);
		};
	});

	function pad(n: number): string {
		return String(n).padStart(2, '0');
	}

	let wrapperClass = $derived(
		`block-countdown${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if !target_date}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			Set a target date to start the countdown
		</div>
	{:else if timeLeft.expired}
		<div class="flex flex-col items-center justify-center rounded-lg bg-gray-50 p-8 text-center dark:bg-gray-800/50">
			{#if title}
				<h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{title}</h3>
			{/if}
			<p class="text-xl font-bold text-emerald-600 dark:text-emerald-400">{expired_text}</p>
		</div>
	{:else}
		<div class="flex flex-col items-center justify-center rounded-lg bg-gray-50 p-8 text-center dark:bg-gray-800/50">
			{#if title}
				<h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{title}</h3>
			{/if}
			<div class="flex gap-4">
				{#if show_days}
					<div class="flex flex-col items-center">
						<span class="text-4xl font-bold tabular-nums text-gray-900 dark:text-white">{pad(timeLeft.days)}</span>
						<span class="mt-1 text-xs text-gray-500 dark:text-gray-400">{t('lib.page-builder.blocks.CountdownBlock.svelte.days')}</span>
					</div>
				{/if}
				{#if show_hours}
					<div class="flex flex-col items-center">
						<span class="text-4xl font-bold tabular-nums text-gray-900 dark:text-white">{pad(timeLeft.hours)}</span>
						<span class="mt-1 text-xs text-gray-500 dark:text-gray-400">{t('lib.page-builder.blocks.CountdownBlock.svelte.hours')}</span>
					</div>
				{/if}
				{#if show_minutes}
					<div class="flex flex-col items-center">
						<span class="text-4xl font-bold tabular-nums text-gray-900 dark:text-white">{pad(timeLeft.minutes)}</span>
						<span class="mt-1 text-xs text-gray-500 dark:text-gray-400">{t('lib.page-builder.blocks.CountdownBlock.svelte.minutes')}</span>
					</div>
				{/if}
				{#if show_seconds}
					<div class="flex flex-col items-center">
						<span class="text-4xl font-bold tabular-nums text-gray-900 dark:text-white">{pad(timeLeft.seconds)}</span>
						<span class="mt-1 text-xs text-gray-500 dark:text-gray-400">{t('lib.page-builder.blocks.CountdownBlock.svelte.seconds')}</span>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</div>
