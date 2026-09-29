<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Itemized Pricing)
	// Read-only itemized rendering of a bound field group: one table per
	// custom_fields.section token (transportation, accommodation…), with
	// price_range fields shown as Item | Average | Range.
	import { t } from '$lib/i18n';
	import type { WikiEntityField, WikiEntityFieldGroup } from '$lib/api/wiki';
	import { formatPriceRange, formatWikiFieldValue, priceRangeParts } from '$lib/utils/wikiFieldValue';

	let {
		group,
		/** Optional per-section "View more {label}" link builder — sibling
		 *  template pages each render their own section's item table. */
		moreHref = undefined,
		/** ?section={key} filtered view — only this section's table renders,
		 *  with a link back to the full page. */
		activeSection = undefined,
		allHref = undefined
	}: {
		group: WikiEntityFieldGroup;
		moreHref?: (sectionKey: string) => string | undefined;
		activeSection?: string;
		allHref?: string;
	} = $props();

	// Named sections first-seen order; section-less fields form the '' bucket
	// and render unlabeled — same contract as the backend fieldSections DTO.
	const sections = $derived.by(() => {
		const src =
			group.sections && group.sections.length > 0
				? group.sections
				: [{ key: '', fields: group.fields }];
		return src
			.map((s) => ({ key: s.key, fields: s.fields.filter((f) => f.visible) }))
			.filter((s) => s.fields.length > 0)
			.filter((s) => !activeSection || s.key === activeSection);
	});

	function sectionLabel(key: string): string {
		return t(`wiki.section_${key}`, key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()));
	}

	function isPriceTable(fields: WikiEntityField[]): boolean {
		return fields.length > 0 && fields.every((f) => f.field_type === 'price_range');
	}

	function avgOf(f: WikiEntityField): string {
		const p = priceRangeParts(f.value);
		return p?.avg !== undefined
			? p.avg.toLocaleString()
			: formatPriceRange(f.value);
	}

	function rangeOf(f: WikiEntityField): string {
		const p = priceRangeParts(f.value);
		if (!p || p.min === undefined || p.max === undefined) return '—';
		return `${p.min.toLocaleString()} – ${p.max.toLocaleString()}`;
	}
</script>

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
			<section class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)]">
				{#if section.key}
					<div class="flex items-center justify-between gap-2 border-b border-[var(--color-border)] px-4 py-2.5">
						<h3 class="text-sm font-semibold text-[var(--color-text)]">
							{sectionLabel(section.key)}
						</h3>
						{#if moreHref?.(section.key)}
							<a
								href={moreHref(section.key)}
								class="shrink-0 text-xs font-medium text-[var(--color-primary)] hover:underline"
							>
								{t('wiki.view_more_prices', 'View more {section} prices', { section: sectionLabel(section.key).toLowerCase() })} →
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
									<td class="px-4 py-2 text-[var(--color-text)]">{f.label}</td>
									<td class="px-4 py-2 text-[var(--color-text)]">{avgOf(f)}</td>
									<td class="px-4 py-2 text-[var(--color-muted)]">{rangeOf(f)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				{:else}
					<dl class="divide-y divide-[var(--color-border)]">
						{#each section.fields as f (f.id)}
							<div class="flex items-baseline justify-between gap-3 px-4 py-2 text-sm">
								<dt class="shrink-0 text-[var(--color-muted)]">{f.label}</dt>
								<dd class="min-w-0 truncate text-right text-[var(--color-text)]">
									{formatWikiFieldValue(f)}
								</dd>
							</div>
						{/each}
					</dl>
				{/if}
			</section>
		{/each}
	</div>
{/if}
