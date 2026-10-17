<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Itemized Pricing, Wiki Topic Pages)
	// Read-only itemized rendering of a bound field group: one table per
	// custom_fields.section token (transportation, accommodation…), with
	// price_range fields shown as Item | Average | Range. §80 Wiki Topic
	// Pages: sections synthesized from child page templates carry label +
	// page_slug — they render as a capped preview table with a "View more"
	// link to the real topic page; sections whose fields are all
	// price_parts render a named-parts table (columns = options.parts).
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import type { WikiEntityField, WikiEntityFieldGroup, WikiEntityFieldSection } from '$lib/api/wiki';
	import {
		fieldCurrencyCode,
		formatPricePart,
		formatPriceRange,
		formatWikiFieldValue,
		isMediaFieldType,
		mediaFieldItems,
		type PricePartDef,
		pricePartsSchema,
		pricePartsValue,
		priceRangeParts,
		type FxEquiv
	} from '$lib/utils/wikiFieldValue';
	import type { CurrencyInfo } from '$lib/utils/currency';

	let {
		group,
		/** Optional per-section "View more {label}" link builder —
		 *  composite sections link to their topic page, token sections to
		 *  the same-page ?section= filter. */
		moreHref = undefined,
		/** ?section={key} filtered view — only this section's table renders,
		 *  with a link back to the full page. */
		activeSection = undefined,
		allHref = undefined,
		/** §58 — host space's resolved country currency; fallback display code
		 *  for currency/price_range fields without an options.currency_code. */
		contextCurrency = '',
		/** §112 — local→base display rate for "(base)" equivalents. */
		contextFx = undefined
	}: {
		group: WikiEntityFieldGroup;
		moreHref?: (section: WikiEntityFieldSection) => string | undefined;
		activeSection?: string;
		allHref?: string;
		contextCurrency?: string;
		contextFx?: FxEquiv;
	} = $props();

	// §58 — active currency list from the root layout, for price_range
	// symbols.
	const currencies = $derived(page.data.currencies as CurrencyInfo[] | undefined);

	// Named sections first-seen order; section-less fields form the '' bucket
	// and render unlabeled — same contract as the backend fieldSections DTO.
	// §80 Wiki Topic Pages: the reserved quick_facts token renders in the
	// right rail, never a body table; composite (page_slug) sections render
	// even when empty so the drill-down link still shows.
	const sections = $derived.by(() => {
		const src =
			group.sections && group.sections.length > 0
				? group.sections
				: [{ key: '', fields: group.fields }];
		return src
			.filter((s) => s.key !== 'quick_facts')
			.map((s) => ({ ...s, fields: s.fields.filter((f) => f.visible) }))
			.filter((s) => s.fields.length > 0 || s.page_slug)
			.filter((s) => !activeSection || s.key === activeSection);
	});

	function sectionLabel(s: WikiEntityFieldSection): string {
		if (s.label) return s.label;
		return t(`wiki.section_${s.key}`, s.key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()));
	}

	function isPriceTable(fields: WikiEntityField[]): boolean {
		return fields.length > 0 && fields.every((f) => f.field_type === 'price_range');
	}

	function isPartsTable(fields: WikiEntityField[]): boolean {
		return fields.length > 0 && fields.every((f) => f.field_type === 'price_parts');
	}

	/** Column schema for a parts table — the union of every row's
	 *  options.parts in first-seen order, so mixed-schema sections
	 *  (transit vs taxi columns) still surface all declared parts.
	 *  Emphasis flags a total row (options.emphasize). */
	function partsColumns(fields: WikiEntityField[]) {
		const seen = new Map<string, PricePartDef>();
		for (const f of fields) {
			for (const p of pricePartsSchema(f.options)) {
				if (!seen.has(p.key)) seen.set(p.key, p);
			}
		}
		return [...seen.values()];
	}

	function isEmphasized(f: WikiEntityField): boolean {
		return (
			f.options !== null &&
			typeof f.options === 'object' &&
			!Array.isArray(f.options) &&
			(f.options as Record<string, unknown>).emphasize === true
		);
	}

	function partCell(f: WikiEntityField, key: string, kind: string | undefined): string {
		const code = fieldCurrencyCode(f.options) || contextCurrency;
		return formatPricePart(pricePartsValue(f.value)[key], kind, code, currencies, contextFx);
	}

	function avgOf(f: WikiEntityField): string {
		const code = fieldCurrencyCode(f.options) || contextCurrency;
		const p = priceRangeParts(f.value);
		return p?.avg !== undefined
			? formatPriceRange({ avg: p.avg }, code, currencies, contextFx)
			: formatPriceRange(f.value, code, currencies, contextFx);
	}

	function rangeOf(f: WikiEntityField): string {
		const code = fieldCurrencyCode(f.options) || contextCurrency;
		const p = priceRangeParts(f.value);
		if (!p || p.min === undefined || p.max === undefined) return '—';
		return formatPriceRange({ min: p.min, max: p.max }, code, currencies, contextFx);
	}
</script>

{#snippet computedBadge()}
	<!-- §80 page rollup — derived from options.computed sources; the
	     field is read-only by contract. -->
	<span
		class="ml-1.5 rounded-full border border-[var(--color-border)] px-1.5 py-px align-middle text-[10px] font-medium uppercase tracking-wide text-[var(--color-muted)]"
		title={t('wiki.field_computed_hint', 'Computed from this page’s sub-pages')}
	>
		{t('wiki.field_computed', 'computed')}
	</span>
{/snippet}

{#if activeSection && allHref}
	<div class="mb-3">
		<a
			href={allHref}
			class="text-xs font-medium text-[var(--color-primary)] hover:underline"
		>
			← {t('wiki.all_sections', 'All sections')}
		</a>
	</div>
{/if}
{#if sections.length > 0}
	<div class="space-y-5">
		{#each sections as section (section.key)}
			<!-- §80 Reader UX — "id" exposes a deep-link anchor per section. -->
			<section
				id={section.key ? `section-${section.key}` : undefined}
				class="scroll-mt-24 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]"
			>
				{#if section.key}
					<div class="flex items-center justify-between gap-2 border-b border-[var(--color-border)] px-4 py-2.5">
						<h3 class="text-sm font-semibold text-[var(--color-text)]">
							{sectionLabel(section)}
						</h3>
						{#if moreHref?.(section)}
							<a
								href={moreHref(section)}
								class="shrink-0 text-xs font-medium text-[var(--color-primary)] hover:underline"
							>
								{t('wiki.view_more_prices', 'View more {section} prices', { section: sectionLabel(section).toLowerCase() })} →
							</a>
						{/if}
					</div>
				{/if}
				{#if isPriceTable(section.fields)}
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[var(--color-border)] text-left text-xs uppercase tracking-wide text-[var(--color-muted)]">
								<th class="px-4 py-2 font-medium">{t('wiki.item_col_item', 'Item')}</th>
								<th class="px-4 py-2 font-medium">{t('wiki.item_col_avg', 'Avg. price')}</th>
								<th class="px-4 py-2 font-medium">{t('wiki.item_col_range', 'Range')}</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-[var(--color-border)]">
							{#each section.fields as f (f.id)}
								<tr>
									<td class="px-4 py-2 text-[var(--color-text)]">
										{f.label}{#if f.computed}{@render computedBadge()}{/if}
									</td>
									<td class="px-4 py-2 text-[var(--color-text)]">{avgOf(f)}</td>
									<td class="px-4 py-2 text-[var(--color-muted)]">{rangeOf(f)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{:else if isPartsTable(section.fields)}
					<!-- §80 Wiki Topic Pages — heterogeneous item table: one field
					     is one row, columns are the declared options.parts. -->
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-[var(--color-border)] text-left text-xs uppercase tracking-wide text-[var(--color-muted)]">
								<th class="px-4 py-2 font-medium">{t('wiki.item_col_item', 'Item')}</th>
								{#each partsColumns(section.fields) as col (col.key)}
									<th class="px-4 py-2 font-medium">{col.label || col.key}</th>
								{/each}
							</tr>
						</thead>
						<tbody class="divide-y divide-[var(--color-border)]">
							{#each section.fields as f (f.id)}
								<tr class={isEmphasized(f) ? 'bg-[var(--color-surface-subtle)] font-semibold' : ''}>
									<td class="px-4 py-2 text-[var(--color-text)]">
										{f.label}{#if f.computed}{@render computedBadge()}{/if}
									</td>
									{#each partsColumns(section.fields) as col (col.key)}
										<td class="px-4 py-2 text-[var(--color-text)]">{partCell(f, col.key, col.kind)}</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				{:else}
					<dl class="divide-y divide-[var(--color-border)]">
						{#each section.fields as f (f.id)}
							{#if isMediaFieldType(f.field_type)}
								<!-- §80 — image fields render as a linked thumbnail strip;
								     text formatting would stringify {id,url,alt}. -->
								{@const media = mediaFieldItems(f.value)}
								{#if media.length > 0}
									<div class="px-4 py-2 text-sm">
										<dt class="text-[var(--color-muted)]">
											{f.label}{#if f.computed}{@render computedBadge()}{/if}
										</dt>
										<dd class="mt-1.5 flex flex-wrap gap-2">
											{#each media as m (m.id || m.url)}
												<a
													href={m.url}
													target="_blank"
													rel="noopener noreferrer"
													class="block transition-opacity hover:opacity-90"
												>
													<img
														src={m.url}
														alt={m.alt ?? ''}
														loading="lazy"
														class="h-20 w-28 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
													/>
												</a>
											{/each}
										</dd>
									</div>
								{/if}
							{:else}
							<div class="flex items-baseline justify-between gap-3 px-4 py-2 text-sm">
								<dt class="shrink-0 text-[var(--color-muted)]">
									{f.label}{#if f.computed}{@render computedBadge()}{/if}
								</dt>
								<dd class="min-w-0 truncate text-right text-[var(--color-text)]">
									{formatWikiFieldValue(f, currencies, contextCurrency, contextFx)}
								</dd>
							</div>
							{/if}
						{/each}
					</dl>
				{/if}
			</section>
		{/each}
	</div>
{/if}
