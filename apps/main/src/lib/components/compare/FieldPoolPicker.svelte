<script lang="ts">
	// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md (Sub-spec 6)
	import type { JCEFieldPoolGroup } from '#lib/types/jce.js';
	import { t } from '#lib/i18n/index.js';

	let {
		groups,
		selected = $bindable(new Set<string>()),
		maxHeight
	}: { groups: JCEFieldPoolGroup[]; selected: Set<string>; maxHeight?: string } = $props();

	// Group/field labels may carry a `{name}` template token intended for
	// per-item context ("{name} Attractions" → "Lagos Attractions"). At type
	// level there is no item — strip it so labels read plainly.
	function cleanLabel(label: string): string {
		return label.replace(/\{name\}\s*/g, '').trim();
	}

	function comparableIds(g: JCEFieldPoolGroup) {
		return g.fields.filter((f) => f.comparable).map((f) => f.id);
	}

	function toggleField(id: string) {
		const next = new Set(selected);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selected = next;
	}

	function toggleGroup(g: JCEFieldPoolGroup) {
		const ids = comparableIds(g);
		const next = new Set(selected);
		const allOn = ids.length > 0 && ids.every((id) => next.has(id));
		for (const id of ids) {
			if (allOn) next.delete(id);
			else next.add(id);
		}
		selected = next;
	}

	function toggleAll() {
		const all = groups.flatMap(comparableIds);
		const next = new Set(selected);
		const allOn = all.length > 0 && all.every((id) => next.has(id));
		selected = allOn ? new Set() : new Set(all);
	}

	function groupState(g: JCEFieldPoolGroup): 'all' | 'some' | 'none' {
		const ids = comparableIds(g);
		if (ids.length === 0) return 'none';
		const on = ids.filter((id) => selected.has(id)).length;
		return on === ids.length ? 'all' : on > 0 ? 'some' : 'none';
	}
</script>

<div class="pool-picker">
	<div class="pool-head">
		<span class="pool-title">{t('jce.compare.fields.title', 'Compare fields')}</span>
		<button type="button" class="pool-all" onclick={toggleAll}>
			{t('jce.compare.fields.all', 'All categories')}
		</button>
	</div>

	<div class="pool-groups" style:max-height={maxHeight} style:overflow-y={maxHeight ? 'auto' : undefined}>
	{#each groups as g (g.group_id)}
		{@const state = groupState(g)}
		{#if g.fields.length > 0}
		<details class="pool-group" open={g.is_default}>
			<summary class="pool-group-head">
				<input
					type="checkbox"
					checked={state === 'all'}
					indeterminate={state === 'some'}
					disabled={comparableIds(g).length === 0}
					onclick={(e) => {
						e.stopPropagation();
						toggleGroup(g);
					}}
				/>
				<span class="pool-group-label">{cleanLabel(g.label)}</span>
			</summary>
			<ul class="pool-fields">
				{#each g.fields as f (f.id)}
					<li>
						<label class:disabled={!f.comparable}>
							<input
								type="checkbox"
								checked={selected.has(f.id)}
								disabled={!f.comparable}
								onchange={() => toggleField(f.id)}
							/>
							{cleanLabel(f.label)}
							{#if !f.comparable}
								<span class="hint">{t('jce.compare.fields.non_comparable', 'not comparable')}</span>
							{/if}
						</label>
					</li>
				{/each}
			</ul>
		</details>
		{/if}
	{/each}
	</div>
</div>

<style>
	.pool-picker {
		border: 1px solid var(--color-border, #e2e8f0);
		border-radius: 0.75rem;
		padding: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.pool-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-weight: 600;
		font-size: 0.875rem;
	}
	.pool-all {
		font-size: 0.75rem;
		color: var(--color-primary, #2563eb);
		background: none;
		border: none;
		cursor: pointer;
	}
	.pool-group-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 500;
	}
	.pool-groups {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		/* Breathing room for the scrollbar + focus rings when capped. */
		padding-right: 0.25rem;
	}
	.pool-fields {
		list-style: none;
		margin: 0.25rem 0 0.25rem 1.5rem;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.pool-fields label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.8125rem;
	}
	.pool-fields label.disabled {
		opacity: 0.5;
	}
	.hint {
		font-size: 0.7rem;
		opacity: 0.7;
	}
</style>
