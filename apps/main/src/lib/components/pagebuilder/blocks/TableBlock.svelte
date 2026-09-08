<script lang="ts">
	import type { TableProps } from '../types';

	let { props }: { props: TableProps } = $props();
	const headers = $derived(props.headers ?? []);
	const rows = $derived(props.rows ?? []);
</script>

{#if headers.length > 0}
	<div class="mx-auto w-full max-w-4xl px-4 py-6">
		<div class="prose-table-wrapper">
			<table class="text-sm">
				<thead>
					<tr>
						{#each headers as h (h)}
							<th class="text-[var(--color-text)]">{h}</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each rows as row, i (i)}
						<tr>
							{#each row as cell (cell)}
								<td class="text-[var(--color-text-subtle)]">{cell}</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	</div>
{/if}
