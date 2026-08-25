<script lang="ts">
	interface Props {
		label: string;
		value: number;
		placeholder?: string;
		oninput?: (value: number) => void;
	}

	let { label, value, placeholder, oninput }: Props = $props();

	function handleInput(event: Event) {
		const raw = (event.currentTarget as HTMLInputElement).value;
		const parsed = raw === '' ? 0 : Number(raw);
		const num = Number.isNaN(parsed) ? 0 : parsed;
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
		oninput={handleInput}
	/>
</label>
