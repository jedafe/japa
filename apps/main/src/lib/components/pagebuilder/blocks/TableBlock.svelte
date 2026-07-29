<script lang="ts">
	import type { TableProps } from '../types';

	let { props }: { props: TableProps } = $props();
	const headers = $derived(props.headers ?? []);
	const rows = $derived(props.rows ?? []);
</script>

{#if headers.length > 0}
	<div class="mx-auto w-full max-w-4xl px-4 py-6">
		<div class="overflow-x-auto rounded-[var(--radius-card)] border border-[var(--color-border)]">
			<table class="w-full text-sm">
				<thead class="bg-[var(--color-surface-subtle)]">
					<tr>
						{#each headers as h (h)}
							<th class="px-4 py-3 text-left font-medium text-[var(--color-text)]">{h}</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each rows as row, i (i)}
						<tr class={props.striped && i % 2 === 1 ? 'bg-[var(--color-surface-subtle)]' : ''}>
							{#each row as cell (cell)}
								<td class="border-t border-[var(--color-border)] px-4 py-3 text-[var(--color-text-subtle)]">{cell}</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
{/if}
