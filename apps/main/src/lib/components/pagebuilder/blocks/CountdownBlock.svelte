<script lang="ts">
	import type { CountdownProps } from '../types';
	import { onMount, onDestroy } from 'svelte';

	let { props }: { props: CountdownProps } = $props();

	let remaining = $state({ days: 0, hours: 0, minutes: 0, seconds: 0 });
	let expired = $state(false);
	let interval: ReturnType<typeof setInterval> | null = null;

	function tick() {
		const target = new Date(props.target_date).getTime();
		const now = Date.now();
		const diff = target - now;
		if (diff <= 0) {
			expired = true;
			remaining = { days: 0, hours: 0, minutes: 0, seconds: 0 };
			if (interval) {
				clearInterval(interval);
				interval = null;
			}
			return;
		}
		remaining = {
			days: Math.floor(diff / 86400000),
			hours: Math.floor((diff % 86400000) / 3600000),
			minutes: Math.floor((diff % 3600000) / 60000),
			seconds: Math.floor((diff % 60000) / 1000)
		};
	}

	onMount(() => {
		tick();
		interval = setInterval(tick, 1000);
	});
	onDestroy(() => {
		if (interval) clearInterval(interval);
	});
</script>

<div class="mx-auto w-full max-w-2xl px-4 py-6 text-center">
	{#if props.label}
		<p class="mb-3 text-sm font-medium text-[var(--color-text)]">{props.label}</p>
	{/if}
	{#if expired}
		<p class="text-lg font-semibold text-[var(--color-text)]">⏰</p>
	{:else}
		<div class="flex justify-center gap-3">
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
				<p class="text-2xl font-bold text-[var(--color-text)]">{String(remaining.days).padStart(2, '0')}</p>
				<p class="text-xs text-[var(--color-muted)]">days</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
				<p class="text-2xl font-bold text-[var(--color-text)]">{String(remaining.hours).padStart(2, '0')}</p>
				<p class="text-xs text-[var(--color-muted)]">hrs</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
				<p class="text-2xl font-bold text-[var(--color-text)]">{String(remaining.minutes).padStart(2, '0')}</p>
				<p class="text-xs text-[var(--color-muted)]">min</p>
			</div>
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
				<p class="text-2xl font-bold text-[var(--color-text)]">{String(remaining.seconds).padStart(2, '0')}</p>
				<p class="text-xs text-[var(--color-muted)]">sec</p>
			</div>
		</div>
	{/if}
</div>
