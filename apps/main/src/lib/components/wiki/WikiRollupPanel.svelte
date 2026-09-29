<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Hierarchical UX)
	// Read-time descendant rollup on a rollup-enabled category page — the
	// aggregate summary cards ("Average monthly expenses") plus the
	// per-descendant values table ("Cost of Living by City").
	import { t } from '$lib/i18n';
	import type { WikiRollup, WikiRollupField } from '$lib/types';
	import { formatPriceRange, priceRangeParts } from '$lib/utils/wikiFieldValue';

	let { rollup, spaceSlug }: { rollup: WikiRollup; spaceSlug: string } = $props();

	const fmtNum = (n: number | undefined) =>
		n === undefined ? '—' : n.toLocaleString(undefined, { maximumFractionDigits: 0 });

	// Summary cards — one per rollup field that produced a metric, capped so
	// a wide group doesn't flood the page; the full matrix is in the table.
	const summaryFields = $derived(rollup.fields.slice(0, 6));

	function metricHeadline(f: WikiRollupField): string {
		const m = f.metrics;
		if (f.field_type === 'price_range') {
			if (m.min !== undefined && m.max !== undefined) {
				return `${fmtNum(m.min)} – ${fmtNum(m.max)}`;
			}
			return fmtNum(m.avg);
		}
		return fmtNum(m.avg ?? m.min ?? m.max);
	}

	function cellValue(f: WikiRollupField, raw: unknown): string {
		if (f.field_type === 'price_range') {
			const p = priceRangeParts(raw);
			if (p?.avg !== undefined && p.min !== undefined && p.max !== undefined) {
				return `${fmtNum(p.avg)} (${fmtNum(p.min)}–${fmtNum(p.max)})`;
			}
			return formatPriceRange(raw);
		}
		return typeof raw === 'number' ? fmtNum(raw) : '—';
	}
</script>

{#if rollup.fields.length > 0 || rollup.rows.length > 0}
	<section class="mt-6 space-y-4">
		{#if summaryFields.length > 0}
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each summaryFields as f (f.key)}
					<div class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<p class="text-xs text-[var(--color-muted)]">{f.label}</p>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">
							{metricHeadline(f)}
						</p>
						{#if f.section}
							<p class="mt-0.5 text-[11px] uppercase tracking-wide text-[var(--color-muted)]">
								{t(`wiki.section_${f.section}`, f.section.replace(/_/g, ' '))}
							</p>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		{#if rollup.rows.length > 0}
			<div class="overflow-x-auto rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-[var(--color-border)] text-left text-xs uppercase tracking-wide text-[var(--color-muted)]">
							<th class="px-4 py-2 font-medium">
								{t('wiki.rollup_col_space', 'City')}
							</th>
							{#each rollup.fields.slice(0, 5) as f (f.key)}
								<th class="px-4 py-2 font-medium">{f.label}</th>
							{/each}
						</tr>
					</thead>
					<tbody class="divide-y divide-[var(--color-border)]">
						{#each rollup.rows as row (row.space.slug)}
							<tr>
								<td class="px-4 py-2">
									<a
										href="/wiki/{encodeURIComponent(row.space.slug)}"
										class="font-medium text-[var(--color-primary)] hover:underline"
									>
										{row.space.name}
									</a>
								</td>
								{#each rollup.fields.slice(0, 5) as f (f.key)}
									<td class="px-4 py-2 text-[var(--color-text)]">
										{cellValue(f, row.values[f.key])}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
			<p class="text-xs text-[var(--color-muted)]">
				{t('wiki.rollup_scope', 'Aggregated from {count} places under {name}', {
					count: rollup.spaces,
					name: spaceSlug
				})}
			</p>
		{/if}
	</section>
{/if}
