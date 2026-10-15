<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
	// Read-only "Quick Facts" rail card — renders the space's structured field
	// values (capital, population, currency, …) from the community-editable
	// fields API. Edit affordances live in the body-level EntityFieldsPanel;
	// this widget is display-only and hides itself when no values exist.
	import {
		Banknote,
		Building2,
		Clock,
		Flag,
		Globe,
		Info,
		Languages,
		MapPin,
		Phone,
		Plug,
		Users
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { getEntityFields, type WikiEntityField, type WikiEntityFieldGroup } from '$lib/api/wiki';
	import { formatWikiFieldValue, isMediaFieldType } from '$lib/utils/wikiFieldValue';
	import type { CurrencyInfo } from '$lib/utils/currency';
	import type { IconComponent } from '$lib/components/ui/icons';

	let {
		entityType = 'space',
		entityId,
		maxItems = 8
	}: {
		entityType?: string;
		entityId: string;
		maxItems?: number;
	} = $props();

	let fields = $state<WikiEntityField[] | null>(null);
	// §58 — the entity's resolved country currency, display fallback for
	// currency/price_range fields that don't declare {currency_code}.
	let contextCurrency = $state('');
	let loading = $state(true);
	// §58 — shared active currency list from the root layout, used to
	// resolve symbols on currency/price_range field values.
	const currencies = $derived(page.data.currencies as CurrencyInfo[] | undefined);

	// §80 Super-Admin Direct Publish — the card renders the entity's
	// *overview* group's fields only (wiki_{class}_overview): the facts
	// rail is a space-level identity panel, not a per-topic summary. The
	// earlier bound-group `quick_facts`-section scoping is retired — an
	// entity whose overview group is empty renders no card.
	function pickFacts(groups: WikiEntityFieldGroup[]): WikiEntityField[] {
		const overview = groups.find((g) => g.slug.endsWith('_overview'));
		return overview?.fields ?? [];
	}

	$effect(() => {
		void entityType;
		void entityId;
		let cancelled = false;
		loading = true;
		getEntityFields(entityType, entityId)
			.then((data) => {
				if (cancelled) return;
				contextCurrency = data?.entity_currency_code ?? '';
				// §80 — media-typed fields (attraction gallery) are
				// {id,url,alt} objects, not scalar facts; the body panels
				// render them as thumbnails. Excluding them here keeps a
				// resolved media array from stringifying into the rail.
				fields = pickFacts(data?.groups ?? []).filter(
					(f) => f.visible && !isMediaFieldType(f.field_type) && hasValue(f)
				);
			})
			.catch(() => {
				if (!cancelled) fields = [];
			})
			.finally(() => {
				if (!cancelled) loading = false;
			});
		return () => {
			cancelled = true;
		};
	});

	function hasValue(f: WikiEntityField): boolean {
		const v = f.value;
		if (v === null || v === undefined || v === '') return false;
		if (Array.isArray(v) && v.length === 0) return false;
		return true;
	}

	const FIELD_ICONS: Array<[RegExp, IconComponent]> = [
		[/capital/i, Building2],
		[/city|cities/i, MapPin],
		[/population/i, Users],
		[/language/i, Languages],
		[/currency/i, Banknote],
		[/time_?zone|timezone/i, Clock],
		[/calling|phone|dialling|dialing/i, Phone],
		[/power|plug|voltage/i, Plug],
		[/country|nation/i, Flag],
		[/region|continent/i, Globe]
	];

	function fieldIcon(key: string): IconComponent {
		for (const [re, icon] of FIELD_ICONS) {
			if (re.test(key)) return icon;
		}
		return Info;
	}
</script>

{#if loading}
	<section
		class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		aria-busy="true"
	>
		<div class="mb-3 h-4 w-24 animate-pulse rounded bg-[var(--color-surface-subtle)]"></div>
		{#each Array(4) as _}
			<div class="mb-3 h-8 animate-pulse rounded bg-[var(--color-surface-subtle)]"></div>
		{/each}
	</section>
{:else if fields && fields.length > 0}
	<section class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
			{t('wiki.quick_facts', 'Quick Facts')}
		</h2>
		<ul class="space-y-3">
			{#each fields.slice(0, maxItems) as f (f.id)}
				{@const Icon = fieldIcon(f.key)}
				<li class="flex items-start gap-2.5">
					<Icon size={15} class="mt-0.5 shrink-0 text-[var(--color-muted)]" aria-hidden="true" />
					<span class="min-w-0">
						<span class="block text-xs text-[var(--color-muted)]">{f.label}</span>
						<span class="block break-words text-sm font-medium text-[var(--color-text)] line-clamp-3">
							{formatWikiFieldValue(f, currencies, contextCurrency)}
						</span>
					</span>
				</li>
			{/each}
		</ul>
	</section>
{/if}
