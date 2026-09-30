<script lang="ts">
	import type { CustomFieldDef, GeoCountry } from '$lib/types';
	import Autocomplete from '../Autocomplete.svelte';
	import type { AutocompleteItem } from '../Autocomplete.svelte';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import { listCountries } from '$lib/api/geo';
	import { listCurrencies } from '$lib/api/hub';
	import { isoToLocalDateTime } from '$lib/utils/datetime';
	import { page } from '$app/state';
	import { currencySymbol, type CurrencyInfo } from '$lib/utils/currency';
	import { t } from '$lib/i18n';

	interface Props {
		fields: CustomFieldDef[];
		values: Record<string, unknown>;
		onvalues: (v: Record<string, unknown>) => void;
		onedit: () => void;
		/** §58 — the host entity's resolved country currency (e.g. the wiki
		 *  page's space currency); denomination fallback for
		 *  currency/price_range fields with no options.currency_code. */
		contextCurrencyCode?: string;
	}

	let { fields, values, onvalues, onedit, contextCurrencyCode = '' }: Props = $props();

	function setField(key: string, val: unknown) {
		onvalues({ ...values, [key]: val });
		onedit();
	}

	// Repeater helpers — add/remove/move items. Plain repeaters store
	// string[]; when options.item_fields is set (§06) each row is an object
	// with one entry per declared sub-key (e.g. {name, link}).
	interface RepeaterItemField {
		key: string;
		label: string;
		type?: string;
	}
	function repeaterItemFields(field: CustomFieldDef): RepeaterItemField[] {
		const opts = field.options;
		if (opts && !Array.isArray(opts) && Array.isArray((opts as { item_fields?: unknown }).item_fields)) {
			return (opts as { item_fields: RepeaterItemField[] }).item_fields;
		}
		return [];
	}
	function repeaterAdd(key: string, itemFields: RepeaterItemField[] = []) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as unknown[])] : [];
		arr.push(itemFields.length ? Object.fromEntries(itemFields.map((f) => [f.key, ''])) : '');
		setField(key, arr);
	}
	function repeaterSetItem(key: string, idx: number, subKey: string, val: string) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as unknown[])] : [];
		const cur = typeof arr[idx] === 'object' && arr[idx] !== null ? (arr[idx] as Record<string, unknown>) : {};
		arr[idx] = { ...cur, [subKey]: val };
		setField(key, arr);
	}
	function repeaterRemove(key: string, idx: number) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as string[])] : [];
		arr.splice(idx, 1);
		setField(key, arr);
	}
	function repeaterSet(key: string, idx: number, val: string) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as string[])] : [];
		arr[idx] = val;
		setField(key, arr);
	}

	// The three bounds a price_range value carries — typed tuple so part is
	// 'min'|'avg'|'max' in the template.
	const PRICE_PARTS = ['min', 'avg', 'max'] as const;

	// §06/§80 price_range — three bound inputs (min / avg / max) emitting one
	// structured {min?, avg?, max?} value. Raw-string drafts ride alongside
	// `values` so partial keystrokes ("2.") aren't clobbered by the
	// round-trip through the parent; seeds re-derive whenever the stored
	// object changes (post-submit refetch, dialog reseed).
	const priceSeeds = $derived.by(() => {
		const out: Record<string, Record<'min' | 'avg' | 'max', string>> = {};
		for (const f of fields) {
			if (f.field_type !== 'price_range') continue;
			const v = values[f.key];
			const o =
				v && typeof v === 'object' && !Array.isArray(v)
					? (v as Record<string, unknown>)
					: {};
			out[f.key] = {
				min: typeof o.min === 'number' && Number.isFinite(o.min) ? String(o.min) : '',
				avg: typeof o.avg === 'number' && Number.isFinite(o.avg) ? String(o.avg) : '',
				max: typeof o.max === 'number' && Number.isFinite(o.max) ? String(o.max) : ''
			};
		}
		return out;
	});
	let priceEdits = $state<Record<string, Record<'min' | 'avg' | 'max', string>>>({});

	function pricePart(field: CustomFieldDef, part: 'min' | 'avg' | 'max'): string {
		return priceEdits[field.key]?.[part] ?? priceSeeds[field.key]?.[part] ?? '';
	}

	// Emits the structured object the backend normalizePriceRange expects;
	// all three cleared collapses to '' (the shared "no value" convention).
	function setPricePart(field: CustomFieldDef, part: 'min' | 'avg' | 'max', raw: string) {
		const cur: Record<'min' | 'avg' | 'max', string> = {
			...(priceSeeds[field.key] ?? { min: '', avg: '', max: '' }),
			...(priceEdits[field.key] ?? {})
		};
		cur[part] = raw;
		priceEdits = { ...priceEdits, [field.key]: cur };
		const out: Record<string, number> = {};
		for (const p of ['min', 'avg', 'max'] as const) {
			const n = Number(cur[p]);
			if (cur[p] !== '' && Number.isFinite(n)) out[p] = n;
		}
		setField(field.key, Object.keys(out).length > 0 ? out : '');
	}

	// Multiselect helpers — toggle a value in/out of the array.
	function multiselectToggle(key: string, opt: string) {
		const arr = Array.isArray(values[key]) ? [...(values[key] as string[])] : [];
		const i = arr.indexOf(opt);
		if (i >= 0) arr.splice(i, 1);
		else arr.push(opt);
		setField(key, arr);
	}
	function multiselectChecked(key: string, opt: string): boolean {
		return Array.isArray(values[key]) && (values[key] as string[]).includes(opt);
	}

	const label = (f: CustomFieldDef) => f.label + (f.is_required ? ' *' : '');

	// §06 'suggest' fields: type-ahead over the field's options list; the
	// Autocomplete's allowFreeText mode also accepts a typed value that is
	// not in the list.
	// options may be an array (select/suggest/multiselect) or an object
	// config (repeater item_fields, number min/max) — optionList normalizes.
	function optionList(field: CustomFieldDef): (string | { value: string; label: string })[] {
		return Array.isArray(field.options) ? field.options : [];
	}

	// §06 'suggest'/'select' fields can declare a named option source instead
	// of a static list — {"source":"geo_countries"} pulls the platform's
	// countries from GET /geo/countries, {"source":"wallet_currencies"} pulls
	// the managed currency list from GET /currencies (Rule 16, §58).
	function optionSource(field: CustomFieldDef): string | null {
		const o = field.options;
		if (o && !Array.isArray(o) && typeof (o as { source?: unknown }).source === 'string') {
			return (o as { source: string }).source;
		}
		return null;
	}

	// §06/§58 'currency'/'price_range' fields are denominated by the field's
	// {currency_code} option, else the host entity's resolved country
	// currency — show the managed symbol/code as an adornment so the
	// amount's denomination is visible in the input.
	const layoutCurrencies = $derived(page.data.currencies as CurrencyInfo[] | undefined);
	function fieldCurrencySymbol(field: CustomFieldDef): string {
		if (field.field_type !== 'currency' && field.field_type !== 'price_range') return '';
		const o = field.options;
		const code =
			o && !Array.isArray(o) && typeof (o as { currency_code?: unknown }).currency_code === 'string'
				? ((o as { currency_code: string }).currency_code)
				: contextCurrencyCode;
		return code ? currencySymbol(code, layoutCurrencies) || code : '';
	}

	let geoCountries = $state<GeoCountry[] | null>(null);
	$effect(() => {
		if (
			geoCountries === null &&
			fields.some((f) => f.field_type === 'suggest' && optionSource(f) === 'geo_countries')
		) {
			geoCountries = [];
			listCountries()
				.then((c) => (geoCountries = c))
				.catch(() => (geoCountries = []));
		}
	});

	// Managed currency list (Control → Currencies). Fetched lazily when a
	// field declares options {"source":"wallet_currencies"} — select fields
	// store the currency code (e.g. "USD") as the value.
	let walletCurrencies = $state<{ code: string; name: string; symbol: string }[] | null>(null);
	$effect(() => {
		if (
			walletCurrencies === null &&
			fields.some((f) => optionSource(f) === 'wallet_currencies')
		) {
			walletCurrencies = [];
			listCurrencies()
				.then((r) => (walletCurrencies = r.data ?? []))
				.catch(() => (walletCurrencies = []));
		}
	});

	function currencyItems(q: string): AutocompleteItem[] {
		const items = (walletCurrencies ?? []).map((c): AutocompleteItem => ({
			id: c.code,
			label: c.name ? `${c.code} — ${c.name}` : c.code,
			sublabel: c.symbol
		}));
		const query = q.toLowerCase().trim();
		return query
			? items.filter((i) => i.label.toLowerCase().includes(query) || i.id.toLowerCase().includes(query))
			: items;
	}

	function suggestItems(field: CustomFieldDef, q: string): Promise<AutocompleteItem[]> {
		if (optionSource(field) === 'geo_countries') {
			const items = (geoCountries ?? []).map((c): AutocompleteItem => ({
				id: c.geo_code,
				label: c.name,
				sublabel: c.geo_code
			}));
			const query = q.toLowerCase().trim();
			return Promise.resolve(
				query ? items.filter((i) => i.label.toLowerCase().includes(query) || i.id.toLowerCase() === query) : items
			);
		}
		if (optionSource(field) === 'wallet_currencies') {
			return Promise.resolve(currencyItems(q));
		}
		const items = optionList(field).map((o): AutocompleteItem => {
			const opt = typeof o === 'object' ? o : { value: o, label: o };
			return { id: opt.value || opt.label, label: opt.label || opt.value };
		});
		const query = q.toLowerCase().trim();
		return Promise.resolve(query ? items.filter((i) => i.label.toLowerCase().includes(query)) : items);
	}

	// 'select' fields render via the shared Autocomplete too — a closed
	// list, so free text stays off. Stored values remain the option `value`
	// (item.id), matching what the native <select> used to save.
	function selectItems(field: CustomFieldDef): AutocompleteItem[] {
		return optionList(field).map((o): AutocompleteItem => {
			const opt = typeof o === 'object' ? o : { value: o, label: o };
			return { id: opt.value || opt.label, label: opt.label || opt.value };
		});
	}
	function selectFetcher(field: CustomFieldDef) {
		if (optionSource(field) === 'wallet_currencies') {
			return (q = '') => Promise.resolve(currencyItems(q));
		}
		return staticAutocompleteFetcher(selectItems(field));
	}
	// Resolve a stored option value back to its label for the chip; falls
	// back to the raw value for legacy data saved before {value,label}.
	function selectLabel(field: CustomFieldDef, val: unknown): string | null {
		if (typeof val !== 'string' || !val) return null;
		if (optionSource(field) === 'wallet_currencies') {
			const c = (walletCurrencies ?? []).find((c) => c.code === val);
			return c ? (c.name ? `${c.code} — ${c.name}` : c.code) : val;
		}
		const opt = optionList(field).find((o) => (typeof o === 'object' ? o.value : o) === val);
		return opt ? (typeof opt === 'object' ? opt.label : opt) : val;
	}

	// Field types that should always span the full width of the grid,
	// regardless of how many columns are available. These are complex or
	// tall inputs that don't work well side-by-side.
	const FULL_WIDTH_TYPES = new Set([
		'textarea',
		'rich_text',
		'repeater',
		'multiselect',
		'price_range',
		'group'
	]);

	function isFullWidth(field: CustomFieldDef): boolean {
		return FULL_WIDTH_TYPES.has(field.field_type);
	}
</script>

<div class="cf-grid">
{#each fields as field (field.id)}
	<div class="cf-field" class:cf-field-full={isFullWidth(field)}>
		{#if field.field_type === 'boolean'}
			<label class="cf-checkbox">
				<input
					type="checkbox"
					checked={values[field.key] === true}
					onchange={(e) => setField(field.key, e.currentTarget.checked)}
				/>
				<span>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</span>
			</label>
		{:else if field.field_type === 'select'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<Autocomplete
				id={field.key}
				placeholder={field.placeholder ?? field.label}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={selectFetcher(field)}
				initialFetcher={selectFetcher(field)}
				selectedId={typeof values[field.key] === 'string' && values[field.key] ? (values[field.key] as string) : null}
				selectedLabel={selectLabel(field, values[field.key])}
				onSelect={(item) => setField(field.key, item.id)}
				onClear={() => setField(field.key, '')}
			/>
		{:else if field.field_type === 'suggest'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<Autocomplete
				id={field.key}
				placeholder={field.placeholder ?? field.label}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={(q) => suggestItems(field, q)}
				initialFetcher={() => suggestItems(field, '')}
				selectedId={typeof values[field.key] === 'string' && values[field.key] ? (values[field.key] as string) : null}
				selectedLabel={typeof values[field.key] === 'string' ? (values[field.key] as string) : null}
				onSelect={(item) => setField(field.key, item.label)}
				onClear={() => setField(field.key, '')}
				allowFreeText={optionSource(field) === null}
			/>
		{:else if field.field_type === 'multiselect'}
			<span class="cf-label">{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</span>
			<div class="cf-multiselect">
			{#each optionList(field) as opt (typeof opt === 'object' ? opt.value : opt)}
				{@const optValue = typeof opt === 'object' ? opt.value : opt}
				{@const optLabel = typeof opt === 'object' ? opt.label : opt}
				<label class="cf-checkbox">
					<input
						type="checkbox"
						checked={multiselectChecked(field.key, optValue)}
						onchange={() => multiselectToggle(field.key, optValue)}
					/>
					<span>{optLabel}</span>
				</label>
			{/each}
			</div>
		{:else if field.field_type === 'price_range'}
			<!-- §80 Itemized Pricing — one field, three bounds; emits the
			     {min?,avg?,max?} object normalizePriceRange expects. -->
			{@const curSym = fieldCurrencySymbol(field)}
			<span class="cf-label">{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}{#if curSym} <span class="cf-price-currency">({curSym})</span>{/if}</span>
			<div class="cf-price-range">
				{#each PRICE_PARTS as part}
					<label class="cf-price-bound">
						<span class="cf-price-bound-label">{t(`wiki.price_${part}`, part.toUpperCase())}</span>
						<input
							type="number"
							class="cf-input"
							step="any"
							value={pricePart(field, part)}
							oninput={(e) => setPricePart(field, part, e.currentTarget.value)}
						/>
					</label>
				{/each}
			</div>
		{:else if field.field_type === 'number' || field.field_type === 'currency' || field.field_type === 'percentage' || field.field_type === 'rating'}
			{@const curSym = fieldCurrencySymbol(field)}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<div class="cf-money-wrap" class:cf-money-active={curSym}>
				<input
					id={field.key}
					type="number"
					class="cf-input"
					placeholder={field.placeholder ?? ''}
					value={values[field.key] ?? ''}
					step={field.field_type === 'rating' ? '1' : 'any'}
					min={field.field_type === 'rating' ? '1' : undefined}
					max={field.field_type === 'rating' ? '5' : undefined}
					oninput={(e) => setField(field.key, e.currentTarget.valueAsNumber)}
				/>
				{#if curSym}<span class="cf-money-symbol">{curSym}</span>{/if}
			</div>
		{:else if field.field_type === 'date'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="date"
				class="cf-input"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'datetime'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="datetime-local"
				class="cf-input"
				value={isoToLocalDateTime(typeof values[field.key] === 'string' ? (values[field.key] as string) : null) ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'time'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="time"
				class="cf-input"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'duration'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="text"
				class="cf-input"
				placeholder="e.g. 2 weeks, 3 months, 30 days"
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'textarea'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<textarea
				id={field.key}
				class="cf-input cf-textarea"
				rows="3"
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			>{values[field.key] ?? ''}</textarea>
		{:else if field.field_type === 'url'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="url"
				class="cf-input"
				placeholder={field.placeholder ?? ''}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'email'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="email"
				class="cf-input"
				placeholder={field.placeholder ?? ''}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'phone'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="tel"
				class="cf-input"
				placeholder={field.placeholder ?? ''}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'color'}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="color"
				class="cf-color"
				value={values[field.key] ?? '#000000'}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{:else if field.field_type === 'repeater'}
			{@const itemFields = repeaterItemFields(field)}
			<span class="cf-label">{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</span>
			<div class="cf-repeater">
				{#each (Array.isArray(values[field.key]) ? (values[field.key] as unknown[]) : []) as item, idx (idx)}
					<div class="cf-repeater-row">
						{#if itemFields.length}
							{#each itemFields as sf (sf.key)}
								<input
									type={sf.type === 'url' ? 'url' : 'text'}
									class="cf-input"
									placeholder={sf.label}
									value={typeof item === 'object' && item !== null ? ((item as Record<string, unknown>)[sf.key] as string ?? '') : ''}
									oninput={(e) => repeaterSetItem(field.key, idx, sf.key, e.currentTarget.value)}
								/>
							{/each}
						{:else}
							<input
								type="text"
								class="cf-input"
								placeholder={field.placeholder ?? 'Add item...'}
								value={typeof item === 'string' ? item : ''}
								oninput={(e) => repeaterSet(field.key, idx, e.currentTarget.value)}
							/>
						{/if}
						<button type="button" class="cf-repeater-remove" onclick={() => repeaterRemove(field.key, idx)} aria-label="Remove">×</button>
					</div>
				{/each}
				<button type="button" class="cf-repeater-add" onclick={() => repeaterAdd(field.key, itemFields)}>+ Add</button>
			</div>
		{:else if field.field_type === 'group'}
			<!-- Group: a labelled container for sub-fields. The sub-fields are
			rendered as separate custom_fields rows by the parent, so this is a
			visual label only. -->
			<div class="cf-group-label">{field.label}</div>
		{:else}
			<label class="cf-label" for={field.key}>{field.label}{#if field.is_required}<span class="cf-required">*</span>{/if}</label>
			<input
				id={field.key}
				type="text"
				class="cf-input"
				placeholder={field.placeholder ?? ''}
				value={values[field.key] ?? ''}
				oninput={(e) => setField(field.key, e.currentTarget.value)}
			/>
		{/if}
	</div>
{/each}
</div>

<style>
	.cf-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 12px;
	}
	.cf-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	/* Full-width field types span all grid columns. */
	.cf-field-full {
		grid-column: 1 / -1;
	}
	.cf-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.cf-required {
		color: var(--destructive, #dc2626);
		margin-left: 2px;
	}
	.cf-input {
		width: 100%;
		height: 2.375rem;
		padding: 0 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.cf-input::placeholder {
		color: var(--color-muted);
	}
	.cf-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.cf-money-wrap {
		position: relative;
	}
	.cf-money-active .cf-input {
		padding-right: 2.25rem;
	}
	.cf-price-currency {
		font-weight: 400;
		color: var(--color-muted);
	}
	.cf-money-symbol {
		position: absolute;
		right: 12px;
		top: 50%;
		transform: translateY(-50%);
		font-size: 0.875rem;
		color: var(--color-muted);
		pointer-events: none;
	}
	.cf-textarea {
		resize: vertical;
		height: auto;
		min-height: 60px;
		font-family: inherit;
	}
	.cf-checkbox {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 0.875rem;
		color: var(--color-text);
		cursor: pointer;
	}
	.cf-checkbox input {
		width: 16px;
		height: 16px;
		accent-color: var(--color-primary);
		cursor: pointer;
	}
	.cf-multiselect {
		display: flex;
		flex-direction: column;
		gap: 6px;
		max-height: 160px;
		overflow-y: auto;
		padding: 8px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
	}
	.cf-price-range {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 8px;
	}
	.cf-price-bound {
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.cf-price-bound-label {
		font-size: 10px;
		font-weight: 500;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-muted);
	}
	.cf-color {
		width: 100%;
		height: 2.375rem;
		padding: 4px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		cursor: pointer;
	}
	.cf-repeater {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.cf-repeater-row {
		display: flex;
		gap: 6px;
		align-items: center;
	}
	.cf-repeater-remove {
		flex-shrink: 0;
		width: 32px;
		height: 32px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-destructive, #dc2626);
		font-size: 1.25rem;
		line-height: 1;
		cursor: pointer;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.cf-repeater-remove:hover {
		background: var(--color-destructive-muted, #fef2f2);
	}
	.cf-repeater-add {
		align-self: flex-start;
		padding: 4px 12px;
		border: 1px dashed var(--color-border);
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-muted);
		font-size: 0.8125rem;
		cursor: pointer;
	}
	.cf-repeater-add:hover {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}
	.cf-group-label {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text);
		text-transform: uppercase;
		letter-spacing: 0.04em;
		padding: 4px 0;
		border-bottom: 1px solid var(--color-border);
		margin-bottom: 2px;
	}
</style>
