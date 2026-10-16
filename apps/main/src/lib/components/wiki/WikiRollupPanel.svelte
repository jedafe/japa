<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// (Hierarchical UX + Reader UX)
	// Read-time descendant rollup on a rollup-enabled category page — the
	// aggregate summary cards ("Average monthly expenses"), an estimated
	// total + share-of-total donut for money fields, and the per-descendant
	// values table ("Cost of Living by City") with a compare-preselect rail.
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { PieChart } from 'layerchart';
	import type { WikiRollup, WikiRollupField } from '$lib/types';
	import {
		formatPricePart,
		formatPriceRange,
		pricePartsValue,
		priceRangeParts,
		type FxEquiv
	} from '$lib/utils/wikiFieldValue';
	import { currencySymbol, type CurrencyInfo } from '$lib/utils/currency';
	import { categoryIcon, categoryTint } from './icons';
	import { GitCompareArrows, Wallet } from 'lucide-svelte';

	let {
		rollup,
		spaceName,
		/** §58 — host space's resolved country currency; display fallback
		 *  for rollup fields that don't carry options.currency_code. */
		contextCurrency = '',
		/** §112 — local→base display rate for "(base)" equivalents. */
		contextFx = undefined
	}: { rollup: WikiRollup; spaceName: string; contextCurrency?: string; contextFx?: FxEquiv } = $props();

	// §58 — active currency list from the root layout; rollup fields carry
	// currency_code from their field definition's options, else the host
	// space's resolved country currency.
	const currencies = $derived(page.data.currencies as CurrencyInfo[] | undefined);

	function fieldCode(f: WikiRollupField): string {
		if (f.currency_code) return f.currency_code;
		// Context inheritance is for money-typed fields only — a plain
		// 'number' rollup (population, speed) must stay undenominated.
		return f.field_type === 'currency' ||
			f.field_type === 'price_range' ||
			f.field_type === 'price_parts'
			? contextCurrency
			: '';
	}

	const fmtNum = (n: number | undefined) =>
		n === undefined ? '—' : n.toLocaleString(undefined, { maximumFractionDigits: 0 });

	const moneyNum = (f: WikiRollupField, n: number | undefined) => {
		if (n === undefined) return '—';
		const code = fieldCode(f);
		if (!code) return fmtNum(n);
		const sym = currencySymbol(code, currencies);
		const primary = sym && sym !== code ? `${sym}${fmtNum(n)}` : `${code} ${fmtNum(n)}`;
		if (!contextFx || !code || code === contextFx.base || !(contextFx.rate > 0)) return primary;
		const baseSym = currencySymbol(contextFx.base, currencies);
		// Local fmtNum is 0-decimals (rolled-up sums are large); the base
		// equivalent needs 2 decimals for sub-unit amounts like $0.28.
		const amt = n / contextFx.rate;
		const eq = amt.toLocaleString(undefined, { maximumFractionDigits: amt < 100 ? 2 : 0 });
		const equiv = baseSym && baseSym !== contextFx.base ? `${baseSym}${eq}` : `${contextFx.base} ${eq}`;
		return `${primary} (${equiv})`;
	};

	// Summary cards — one per rollup field that produced a metric, capped so
	// a wide group doesn't flood the page; the full matrix is in the table.
	const summaryFields = $derived(rollup.fields.slice(0, 6));

	function metricHeadline(f: WikiRollupField): string {
		const m = f.metrics;
		if (f.field_type === 'price_range') {
			if (m.min !== undefined && m.max !== undefined) {
				return `${moneyNum(f, m.min)} – ${moneyNum(f, m.max)}`;
			}
			return moneyNum(f, m.avg);
		}
		return moneyNum(f, m.avg ?? m.min ?? m.max);
	}

	function cellValue(f: WikiRollupField, raw: unknown): string {
		if (f.field_type === 'price_parts') {
			// §80 Wiki Topic Pages — the row cell projects the declared
			// primary part of the stored {parts:{...}} object, formatted by
			// that part's kind (a number primary stays bare, money currency-formats).
			if (!f.primary_part) return '—';
			return formatPricePart(pricePartsValue(raw)[f.primary_part], f.primary_kind, fieldCode(f), currencies, contextFx);
		}
		if (f.field_type === 'price_range') {
			const p = priceRangeParts(raw);
			if (p?.avg !== undefined && p.min !== undefined && p.max !== undefined) {
				return `${moneyNum(f, p.avg)} (${moneyNum(f, p.min)}–${moneyNum(f, p.max)})`;
			}
			return formatPriceRange(raw, fieldCode(f), currencies, contextFx);
		}
		return typeof raw === 'number' ? moneyNum(f, raw) : '—';
	}

	// §80 Reader UX — icon chips on the summary cards: the field's section
	// drives the icon (same map as the category tabs); section-less fields
	// get the money glyph when they resolve a currency code.
	function cardIcon(f: WikiRollupField) {
		if (f.section) return categoryIcon(f.section);
		return Wallet;
	}

	function cardTint(f: WikiRollupField): { bg: string; fg: string } {
		return f.section ? categoryTint(f.section) : { bg: 'var(--color-surface-subtle)', fg: 'var(--color-muted)' };
	}

	// §80 Reader UX — estimated total + donut share. Money fields that
	// resolve to the same currency code are summed on their avg metric;
	// the dominant code (most fields) wins when codes disagree.
	const moneyFields = $derived(
		summaryFields.filter((f) => fieldCode(f) && f.metrics.avg !== undefined)
	);
	const totalCurrency = $derived.by(() => {
		const counts = new Map<string, number>();
		for (const f of moneyFields) {
			const c = fieldCode(f);
			counts.set(c, (counts.get(c) ?? 0) + 1);
		}
		let best = '';
		let bestN = 0;
		for (const [c, n] of counts) if (n > bestN) (best = c), (bestN = n);
		return best;
	});
	const totalFields = $derived(moneyFields.filter((f) => fieldCode(f) === totalCurrency));
	const estimatedTotal = $derived(
		totalFields.reduce((acc, f) => acc + (f.metrics.avg ?? 0), 0)
	);
	const totalSymbol = $derived.by(() => {
		const sym = currencySymbol(totalCurrency, currencies);
		return sym && sym !== totalCurrency ? sym : `${totalCurrency} `;
	});
	const donutSlices = $derived(
		totalFields.map((f) => ({ key: f.key, label: f.label, value: f.metrics.avg ?? 0 }))
	);
	// Pages scope sums derived aggregates (monthly_total/family_total fold
	// the same itemized sources the other fields carry), so a "total of
	// totals" is meaningless there — keep the cards, drop the donut.
	const showSummaryViz = $derived(
		rollup.scope !== 'pages' && totalFields.length >= 2 && estimatedTotal > 0
	);

	// §80 Reader UX — compare rail: pick up to 4 descendant spaces, deep-
	// linking to /compare with preselected items (JCE §30). The comparison
	// type + item_ref source derive from the descendants' space_type —
	// same convention as /compare/universities' redirect.
	const COMPARE_TYPE: Record<string, { type: string; source: string }> = {
		country: { type: 'countries', source: 'country' },
		state: { type: 'states', source: 'state' },
		city: { type: 'cities', source: 'city' },
		university: { type: 'universities', source: 'university' }
	};
	const compareTarget = $derived(
		COMPARE_TYPE[compareRows0Type(rollup.rows)] ?? null
	);
	function compareRows0Type(rows: WikiRollup['rows']): string {
		return rows.find((r) => r.space.id)?.space.type_slug ?? '';
	}
	// Compare is a cross-entity affordance — pages-scope rows all share the
	// host space, so the checkbox would compare a place against itself.
	const compareRows = $derived(
		rollup.scope === 'pages' || !compareTarget
			? []
			: rollup.rows.filter(
					(r) => r.space.id && r.space.type_slug === compareTarget.source
				)
	);
	let compareIds = $state<string[]>([]);

	// The component instance is reused across same-route navigations —
	// drop selections when a different rollup (another space's page)
	// arrives so stale foreign ids never leak into the preselect link.
	$effect(() => {
		rollup;
		compareIds = [];
	});

	// First-column label derives from the descendants' space_type slug
	// (singular wiki.*_label keys; 'Place' when the type is unknown).
	const spaceColLabel = $derived.by(() => {
		const slug = rollup.rows[0]?.space.type_slug ?? '';
		return slug ? t(`wiki.${slug}_label`, slug) : t('wiki.rollup_col_space', 'Place');
	});

	// §80 page rollup — "pages" rows aggregate this space's own sub-pages.
	const pageScope = $derived(rollup.scope === 'pages');

	// Pages-mode rows carry disjoint field sets (each topic page owns its
	// itemized keys), so the shared-column layout renders mostly '—'.
	// Render each row's own values instead — label + value pairs in rollup
	// field order, capped, with a "+N" overflow count.
	function rowEntries(row: WikiRollup['rows'][number]): {
		shown: { def: WikiRollupField; value: unknown }[];
		extra: number;
	} {
		const shown: { def: WikiRollupField; value: unknown }[] = [];
		let extra = 0;
		for (const f of rollup.fields) {
			const v = row.values?.[f.key];
			if (v == null) continue;
			if (shown.length < 8) shown.push({ def: f, value: v });
			else extra++;
		}
		return { shown, extra };
	}

	// Section tokens render Title-Cased — same fallback as WikiItemTables.
	function sectionLabel(key: string): string {
		return t(`wiki.section_${key}`, key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()));
	}

	function toggleCompare(id: string) {
		if (compareIds.includes(id)) {
			compareIds = compareIds.filter((x) => x !== id);
		} else if (compareIds.length < 4) {
			compareIds = [...compareIds, id];
		}
	}

	const compareHref = $derived(
		compareTarget
			? `/compare?type=${compareTarget.type}&source=${compareTarget.source}&preselect=${encodeURIComponent(compareIds.join(','))}`
			: ''
	);
</script>

{#if rollup.fields.length > 0 || rollup.rows.length > 0}
	<section id="wiki-rollup" class="mt-6 scroll-mt-24 space-y-4">
		{#if summaryFields.length > 0}
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each summaryFields as f (f.key)}
					{@const Icon = cardIcon(f)}
					{@const tint = cardTint(f)}
					<div class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2">
							<span
								class="flex h-7 w-7 shrink-0 items-center justify-center rounded-[calc(var(--radius)-2px)]"
								style:background={tint.bg}
								style:color={tint.fg}
							>
								<Icon size={15} />
							</span>
							<p class="min-w-0 truncate text-xs text-[var(--color-muted)]">{f.label}</p>
						</div>
						<p class="mt-2 text-lg font-semibold text-[var(--color-text)]">
							{metricHeadline(f)}
						</p>
						{#if f.section}
							<p class="mt-0.5 text-[11px] uppercase tracking-wide text-[var(--color-muted)]">
								{sectionLabel(f.section)}
							</p>
						{/if}
					</div>
				{/each}
			</div>
		{/if}

		{#if showSummaryViz}
			<div class="flex flex-col gap-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:flex-row sm:items-center">
				<div class="min-w-0 flex-1">
					<p class="text-xs uppercase tracking-wide text-[var(--color-muted)]">
						{t('wiki.rollup_estimated_total', 'Estimated total')}
					</p>
					<p class="mt-1 text-2xl font-semibold text-[var(--color-text)]">
						{totalSymbol}{fmtNum(estimatedTotal)}
					</p>
					<p class="mt-1 text-xs text-[var(--color-muted)]">
						{t('wiki.rollup_estimated_total_note', 'Sum of {count} averaged line items', {
							count: totalFields.length
						})}
					</p>
				</div>
				<div class="h-40 w-40 shrink-0 self-center">
					<PieChart
						data={donutSlices}
						key="key"
						label="label"
						value="value"
						innerRadius={-18}
						cornerRadius={4}
						padAngle={0.02}
						cRange={[
							'var(--chart-1)',
							'var(--chart-2)',
							'var(--chart-3)',
							'var(--chart-4)',
							'var(--chart-5)',
							'var(--color-primary)'
						]}
					/>
				</div>
				<ul class="min-w-0 flex-1 space-y-1">
					{#each donutSlices as s, i (s.key)}
						<li class="flex items-center gap-2 text-xs">
							<span
								class="h-2.5 w-2.5 shrink-0 rounded-sm"
								style:background={[
									'var(--chart-1)',
									'var(--chart-2)',
									'var(--chart-3)',
									'var(--chart-4)',
									'var(--chart-5)',
									'var(--color-primary)'
								][i % 6]}
							></span>
							<span class="min-w-0 truncate text-[var(--color-text)]">{s.label}</span>
							<span class="ml-auto shrink-0 tabular-nums text-[var(--color-muted)]">
								{Math.round((s.value / estimatedTotal) * 100)}%
							</span>
						</li>
					{/each}
				</ul>
			</div>
		{/if}

		{#if rollup.rows.length > 0}
			<div class="overflow-x-auto rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
				<table class="w-full text-sm">
					<thead>
						<tr class="border-b border-[var(--color-border)] text-left text-xs uppercase tracking-wide text-[var(--color-muted)]">
							<th class="px-4 py-2 font-medium">
								{pageScope ? t('wiki.rollup_col_page', 'Page') : spaceColLabel}
							</th>
							{#if pageScope}
								<th class="px-4 py-2 font-medium">{t('wiki.rollup_col_values', 'Values')}</th>
							{:else}
								{#each rollup.fields.slice(0, 5) as f (f.key)}
									<th class="px-4 py-2 font-medium">{f.label}</th>
								{/each}
							{/if}
							{#if compareRows.length >= 2}
								<th class="px-4 py-2 text-right font-medium" aria-label={t('wiki.rollup_compare_col', 'Compare')}></th>
							{/if}
						</tr>
					</thead>
					<tbody class="divide-y divide-[var(--color-border)]">
						{#each rollup.rows as row (row.page?.slug ?? row.space.slug)}
							{@const entries = pageScope ? rowEntries(row) : null}
							<tr>
								<td class="px-4 py-2">
									{#if row.page}
										<a
											href="/wiki/{encodeURIComponent(row.space.slug)}/{encodeURIComponent(row.page.slug)}"
											class="font-medium text-[var(--color-primary)] hover:underline"
										>
											{row.page.title}
										</a>
									{:else}
										<a
											href="/wiki/{encodeURIComponent(row.space.slug)}"
											class="font-medium text-[var(--color-primary)] hover:underline"
										>
											{row.space.name}
										</a>
									{/if}
								</td>
								{#if pageScope}
									<td class="px-4 py-2">
										{#if !entries || entries.shown.length === 0}
											<span class="text-[var(--color-muted)]">—</span>
										{:else}
											<div class="flex flex-wrap gap-x-4 gap-y-0.5">
												{#each entries.shown as e (e.def.key)}
													<span class="text-xs text-[var(--color-muted)]">
														{e.def.label} <span class="font-medium text-[var(--color-text)]">{cellValue(e.def, e.value)}</span>
													</span>
												{/each}
												{#if entries.extra > 0}
													<span class="text-xs text-[var(--color-muted)]">+{entries.extra}</span>
												{/if}
											</div>
										{/if}
									</td>
								{:else}
									{#each rollup.fields.slice(0, 5) as f (f.key)}
										<td class="px-4 py-2 text-[var(--color-text)]">
											{cellValue(f, row.values[f.key])}
										</td>
									{/each}
								{/if}
								{#if compareRows.length >= 2}
									<td class="px-4 py-2 text-right">
										{#if row.space.id && compareTarget && row.space.type_slug === compareTarget.source}
											<label class="inline-flex cursor-pointer items-center gap-1.5 text-xs text-[var(--color-muted)]">
												<input
													type="checkbox"
													checked={compareIds.includes(row.space.id)}
													onchange={() => toggleCompare(row.space.id ?? '')}
													disabled={!compareIds.includes(row.space.id) && compareIds.length >= 4}
													class="h-3.5 w-3.5 rounded border-[var(--color-border)]"
												/>
												{t('wiki.rollup_compare_row', 'Compare')}
											</label>
										{/if}
									</td>
								{/if}
							</tr>
						{/each}
					</tbody>
				</table>
				{#if compareRows.length >= 2}
					<div class="flex items-center justify-between gap-3 border-t border-[var(--color-border)] px-4 py-2.5">
						<p class="text-xs text-[var(--color-muted)]">
							{t('wiki.rollup_compare_hint', 'Pick 2–4 places to compare side by side.')}
						</p>
						<a
							href={compareHref || '/compare'}
							aria-disabled={compareIds.length < 2}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border px-3 py-1.5 text-xs font-medium transition-colors {compareIds.length >=
							2
								? 'border-[var(--color-primary)] text-[var(--color-primary)] hover:bg-[var(--color-primary)]/10'
								: 'pointer-events-none border-[var(--color-border)] text-[var(--color-muted)] opacity-60'}"
						>
							<GitCompareArrows size={14} />
							{t('wiki.rollup_compare_cta', 'Compare')}{compareIds.length > 0 ? ` (${compareIds.length})` : ''}
						</a>
					</div>
				{/if}
			</div>
			<p class="text-xs text-[var(--color-muted)]">
				{pageScope
					? t('wiki.rollup_scope_pages', 'Aggregated from {count} cost pages', {
							count: rollup.spaces
						})
					: t('wiki.rollup_scope', 'Aggregated from {count} places under {name}', {
							count: rollup.spaces,
							name: spaceName
						})}
			</p>
		{/if}
	</section>
{/if}
