<script lang="ts">
	interface Toggle {
		key: string;
		label: string;
	}

	interface Props {
		label: string;
		values: Record<string, boolean>;
		items: readonly Toggle[];
		onchange?: (key: string, value: boolean) => void;
	}

	let { label, values, items, onchange }: Props = $props();
</script>

<div class="block text-sm font-medium">
	{label}
	<div class="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-3">
		{#each items as item}
			<label
				class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-3 py-2 text-sm hover:bg-accent"
			>
				<input
					type="checkbox"
					class="h-4 w-4 rounded border-border"
					checked={values[item.key] ?? false}
					onchange={(event) =>
						onchange?.(item.key, (event.currentTarget as HTMLInputElement).checked)}
				/>
				<span>{item.label}</span>
			</label>
		{/each}
	</div>
</div>
