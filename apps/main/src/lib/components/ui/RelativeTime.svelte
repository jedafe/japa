<script lang="ts">
	import { onMount } from 'svelte';

	interface Props {
		iso: string;
	}

	let { iso }: Props = $props();

	let label = $state('');

	function formatRelative(input: string): string {
		const then = new Date(input).getTime();
		if (Number.isNaN(then)) return '';
		const now = Date.now();
		const diffSeconds = Math.round((then - now) / 1000);

		const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });

		const divisions: { amount: number; unit: Intl.RelativeTimeFormatUnit }[] = [
			{ amount: 60, unit: 'second' },
			{ amount: 60, unit: 'minute' },
			{ amount: 24, unit: 'hour' },
			{ amount: 7, unit: 'day' },
			{ amount: 4.34524, unit: 'week' },
			{ amount: 12, unit: 'month' },
			{ amount: Number.POSITIVE_INFINITY, unit: 'year' }
		];

		let duration = diffSeconds;
		for (const division of divisions) {
			if (Math.abs(duration) < division.amount) {
				return rtf.format(Math.round(duration), division.unit);
			}
			duration /= division.amount;
		}
		return rtf.format(Math.round(duration), 'year');
	}

	$effect(() => {
		label = formatRelative(iso);
	});

	// Refresh once on mount so labels stay fresh after hydration.
	onMount(() => {
		label = formatRelative(iso);
	});
</script>

<time datetime={iso} title={iso}>{label}</time>
