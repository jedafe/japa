<script lang="ts">
	import { t } from '$lib/i18n';
	import { Plus, X } from 'lucide-svelte';
	import {
		createComparison,
		addComparisonItem,
		addComparisonDimension,
		deleteComparisonItem,
		deleteComparisonDimension,
		patchComparisonItem,
		patchComparisonDimension,
		setDimensionValues
	} from '$lib/api/create';

	interface Props {
		title: string;
		spaceId: string | null;
		oncounts: (items: number, dims: number) => void;
		onedit: () => void;
	}

	let { title, spaceId, oncounts, onedit }: Props = $props();

	let comparisonId = $state<string | null>(null);
	let items = $state<{ id: string | null; title: string }[]>([]);
	let dimensions = $state<{ id: string | null; label: string; dimension_type: string }[]>([]);
	let cells = $state<Record<string, string>>({}); // `${dimId}:${itemId}` → value

	// Debounce timers for cell-value writes (avoid N API calls per keystroke).
	let valueTimers = $state<Record<string, ReturnType<typeof setTimeout>>>({});

	$effect(() => oncounts(items.length, dimensions.length));

	function cellKey(dimIdx: number, itemIdx: number) {
		return `${dimIdx}:${itemIdx}`;
	}

	async function ensureComparison() {
		if (comparisonId) return comparisonId;
		const res = await createComparison({ title, space_id: spaceId ?? undefined });
		if (res.data) comparisonId = res.data.id;
		return comparisonId;
	}

	async function addItem() {
		const id = await ensureComparison();
		if (!id) return;
		const res = await addComparisonItem(id, { title: 'New item', position: items.length });
		if (res.data) items = [...items, { id: res.data.id, title: 'New item' }];
		onedit();
	}

	async function addDimension() {
		const id = await ensureComparison();
		if (!id) return;
		const res = await addComparisonDimension(id, {
			label: t('compare.new_dimension', 'New dimension'),
			dimension_type: 'text',
			position: dimensions.length
		});
		if (res.data) dimensions = [...dimensions, { id: res.data.id, label: t('compare.new_dimension', 'New dimension'), dimension_type: 'text' }];
		onedit();
	}

	// Server-first removal: call DELETE, then drop the row from local state only
	// after the server confirms (Rule 16 — never mutate server data locally).
	async function removeItem(idx: number) {
		const item = items[idx];
		if (item?.id) {
			const res = await deleteComparisonItem(item.id);
			if (res.error) return;
		}
		items = items.filter((_, i) => i !== idx);
		onedit();
	}

	async function removeDimension(idx: number) {
		const dim = dimensions[idx];
		if (dim?.id) {
			const res = await deleteComparisonDimension(dim.id);
			if (res.error) return;
		}
		dimensions = dimensions.filter((_, i) => i !== idx);
		onedit();
	}

	// Persist item/dimension title edits on blur (Rule 16 — server is source of
	// truth). Local state updates immediately for responsiveness; the PATCH
	// confirms on blur.
	async function commitItemTitle(idx: number) {
		const item = items[idx];
		if (!item?.id) return;
		await patchComparisonItem(item.id, { title: item.title });
		onedit();
	}

	async function commitDimensionLabel(idx: number) {
		const dim = dimensions[idx];
		if (!dim?.id) return;
		await patchComparisonDimension(dim.id, { label: dim.label });
		onedit();
	}

	// Debounced cell-value write: update local state immediately, but only fire
	// the PUT after 400ms of inactivity (one call per cell edit, not per
	// keystroke).
	function editCell(dimIdx: number, itemIdx: number, value: string) {
		const key = cellKey(dimIdx, itemIdx);
		cells = { ...cells, [key]: value };
		onedit();
		const existing = valueTimers[key];
		if (existing) clearTimeout(existing);
		valueTimers = { ...valueTimers, [key]: setTimeout(() => void flushCell(dimIdx, itemIdx), 400) };
	}

	async function flushCell(dimIdx: number, itemIdx: number) {
		const dim = dimensions[dimIdx];
		const item = items[itemIdx];
		const key = cellKey(dimIdx, itemIdx);
		if (dim?.id && item?.id) {
			await setDimensionValues(dim.id, [{ item_id: item.id, value: cells[key] ?? '' }]);
		}
		valueTimers = { ...valueTimers, [key]: undefined as unknown as ReturnType<typeof setTimeout> };
	}
</script>

<div class="flex flex-col gap-3">
	<div class="flex items-center gap-2">
		<button type="button" class="btn-secondary" onclick={addItem}>
			<Plus size={14} /> {t('create.comparison.add_item')}
		</button>
		<button type="button" class="btn-secondary" onclick={addDimension}>
			<Plus size={14} /> {t('create.comparison.add_dimension')}
		</button>
	</div>

	{#if items.length > 0 && dimensions.length > 0}
		<div class="overflow-x-auto">
			<table class="w-full border-collapse text-sm">
				<thead>
					<tr>
						<th class="border border-[var(--color-border)] p-2 text-left"></th>
						{#each dimensions as dim, dIdx (dIdx)}
							<th class="border border-[var(--color-border)] p-2 text-left">
								<input
									type="text"
									class="w-full bg-transparent font-medium outline-none"
									value={dim.label}
									oninput={(e) => {
										dimensions[dIdx].label = e.currentTarget.value;
										onedit();
									}}
								onblur={() => commitDimensionLabel(dIdx)}
								/>
								<button type="button" onclick={() => removeDimension(dIdx)} class="text-[var(--color-muted)] hover:text-red-500">
									<X size={12} />
								</button>
							</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each items as item, iIdx (iIdx)}
						<tr>
							<td class="border border-[var(--color-border)] p-2">
								<input
									type="text"
									class="w-full bg-transparent font-medium outline-none"
									value={item.title}
									oninput={(e) => {
										items[iIdx].title = e.currentTarget.value;
										onedit();
									}}
								onblur={() => commitItemTitle(iIdx)}
								/>
								<button type="button" onclick={() => removeItem(iIdx)} class="text-[var(--color-muted)] hover:text-red-500">
									<X size={12} />
								</button>
							</td>
							{#each dimensions as _, dIdx (dIdx)}
								<td class="border border-[var(--color-border)] p-1">
									<input
										type="text"
										class="w-full bg-transparent outline-none"
										value={cells[cellKey(dIdx, iIdx)] ?? ''}
										oninput={(e) => editCell(dIdx, iIdx, e.currentTarget.value)}
									/>
								</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{:else}
		<p class="text-sm text-[var(--color-muted)]">{t('create.comparison.empty')}</p>
	{/if}
</div>

<style>
	.btn-secondary {
		display: flex;
		align-items: center;
		gap: 4px;
		padding: 4px 10px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		font-size: 0.875rem;
		color: var(--color-text);
		background: var(--color-surface);
		cursor: pointer;
	}
	.btn-secondary:hover {
		background: var(--color-surface-subtle);
	}
</style>
