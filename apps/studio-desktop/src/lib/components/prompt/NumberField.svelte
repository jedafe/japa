<script lang="ts">
	interface Props {
		label: string;
		value: number;
		placeholder?: string;
		min?: number;
		max?: number;
		oninput?: (value: number) => void;
	}

	let { label, value, placeholder, min, max, oninput }: Props = $props();

	function clamp(num: number): number {
		if (min !== undefined && num < min) return min;
		if (max !== undefined && num > max) return max;
		return num;
	}

	function handleInput(event: Event) {
		const raw = (event.currentTarget as HTMLInputElement).value;
		const parsed = raw === '' ? 0 : Number(raw);
		const num = Number.isNaN(parsed) ? 0 : clamp(parsed);
		oninput?.(num);
	}
</script>

<label class="block text-sm font-medium">
	{label}
	<input
		type="number"
		class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
		{value}
		{placeholder}
		min={min ?? undefined}
		max={max ?? undefined}
		oninput={handleInput}
	/>
</label>
