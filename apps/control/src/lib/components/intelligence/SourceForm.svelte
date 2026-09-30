<script lang="ts">
	import Modal from '$lib/components/ui/Modal.svelte';
	import Select from '$lib/components/ui/Select.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { Loader2 } from 'lucide-svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	// SourceForm is the create/edit dialog for the intel_sources registry.
	// It is a controlled dialog: the parent owns the `open` state and passes
	// an optional `source` to edit (null for create). On submit it calls
	// `onSubmit` with the field values; the parent performs the API call.

	export interface SourceFormValues {
		name: string;
		domain: string;
		source_url: string;
		category: string;
		categories: string[];
		source_type: string;
		source_kind: string;
		slug: string;
		description: string;
		logo_url: string;
		trust_level: number;
		crawl_method: string;
		crawl_frequency: string;
		crawl_frequency_mins: number;
		attribution_required: boolean;
		source_license: string;
		usage_restriction: string;
		enabled: boolean;
		persona_id: string;
		entity_types: string[];
	}

	interface SourceLike {
		id?: string;
		name: string;
		domain: string;
		source_url: string;
		category: string;
		categories?: string[] | null;
		source_type: string;
		source_kind?: string | null;
		slug?: string | null;
		description?: string | null;
		logo_url?: string | null;
		trust_level: number;
		crawl_method: string;
		crawl_frequency: string;
		crawl_frequency_mins: number;
		attribution_required: boolean;
		source_license?: string | null;
		usage_restriction?: string | null;
		enabled: boolean;
		persona_id?: string | null;
		entity_types?: string[] | null;
	}

	export interface PersonaItem {
		id: string;
		profile_name: string;
		profile_slug?: string;
		backing_username?: string;
		status?: string;
	}

	// SourceKindOption mirrors the {value,label} pairs returned by
	// /api/v1/sources/meta source_kinds (Rule 16 — backend-driven options).
	export interface SourceKindOption {
		value: string;
		label: string;
	}

	let {
		open = $bindable(false),
		source = null as SourceLike | null,
		personas = [] as PersonaItem[],
		entityTypeOptions = [] as string[],
		sourceTypeOptions = [] as string[],
		categoryOptions = [] as string[],
		sourceKindOptions = [] as SourceKindOption[],
		crawlMethodOptions = [] as string[],
		crawlFrequencyOptions = [] as string[],
		saving = false,
		errorMessage = '',
		onSubmit
	}: {
		open: boolean;
		source?: SourceLike | null;
		personas?: PersonaItem[];
		entityTypeOptions?: string[];
		sourceTypeOptions?: string[];
		categoryOptions?: string[];
		sourceKindOptions?: SourceKindOption[];
		crawlMethodOptions?: string[];
		crawlFrequencyOptions?: string[];
		saving?: boolean;
		errorMessage?: string;
		onSubmit: (values: SourceFormValues) => void;
	} = $props();

	const empty: SourceFormValues = {
		name: '',
		domain: '',
		source_url: '',
		category: 'scholarship',
		categories: ['scholarship'],
		source_type: 'rss',
		source_kind: 'publisher',
		slug: '',
		description: '',
		logo_url: '',
		trust_level: 3,
		crawl_method: 'static',
		crawl_frequency: 'daily',
		crawl_frequency_mins: 1440,
		attribution_required: false,
		source_license: '',
		usage_restriction: '',
		enabled: true,
		persona_id: '',
		entity_types: []
	};

	let form = $state<SourceFormValues>({ ...empty });

	// Re-seed the form whenever the dialog opens or the source changes.
	$effect(() => {
		if (open) {
			if (source) {
				// categories seeds from the multi-category column; legacy rows
				// with an empty categories[] fall back to the single category
				// so the multi-select always reflects the effective set.
				const cats = source.categories?.length
					? [...source.categories]
					: source.category
						? [source.category]
						: [];
				form = {
					name: source.name ?? '',
					domain: source.domain ?? '',
					source_url: source.source_url ?? '',
					category: cats[0] ?? source.category ?? 'scholarship',
					categories: cats,
					source_type: source.source_type ?? 'rss',
					source_kind: source.source_kind ?? 'publisher',
					slug: source.slug ?? '',
					description: source.description ?? '',
					logo_url: source.logo_url ?? '',
					trust_level: source.trust_level ?? 3,
					crawl_method: source.crawl_method ?? 'static',
					crawl_frequency: source.crawl_frequency ?? 'daily',
					crawl_frequency_mins: source.crawl_frequency_mins ?? 1440,
					attribution_required: !!source.attribution_required,
					source_license: source.source_license ?? '',
					usage_restriction: source.usage_restriction ?? '',
					enabled: !!source.enabled,
					persona_id: source.persona_id ?? '',
					entity_types: source.entity_types ?? []
				};
			} else {
				form = { ...empty };
			}
		}
	});

	// Category/method/frequency options come from /api/v1/sources/meta
	// (admin_categories, crawl_methods, crawl_frequencies — Rule 16). The
	// columns are free text in the DB, so the current value is appended when
	// absent to avoid clobbering legacy/seeded values on edit.
	const categoryItems = $derived.by(() => {
		const items = categoryOptions.map((v) => ({
			value: v,
			label: t(`intel.category_${v}`, v.replace(/_/g, ' '))
		}));
		if (form.category && !categoryOptions.includes(form.category)) {
			items.push({
				value: form.category,
				label: t(`intel.category_${form.category}`, form.category.replace(/_/g, ' '))
			});
		}
		return items;
	});

	// Multi-category checklist (intel-channels Phase 1): the meta catalog
	// plus every selected value not in it, so editing a source whose
	// categories predate the catalog never silently drops them.
	const categoryChecklist = $derived.by(() => {
		const seen = new Set<string>();
		const items: { value: string; label: string }[] = [];
		for (const v of [...categoryOptions, ...form.categories]) {
			if (!v || seen.has(v)) continue;
			seen.add(v);
			items.push({
				value: v,
				label: t(`intel.category_${v}`, v.replace(/_/g, ' '))
			});
		}
		return items;
	});

	// 'manual' is a backend sentinel for non-crawlable rows (video_channel,
	// entity-linked sources) — not in the meta list, but appended when
	// editing one so the current value is preserved.
	const methodItems = $derived.by(() => {
		const items = crawlMethodOptions.map((v) => ({
			value: v,
			label: t(`intel.method_${v}`, v.replace(/_/g, ' '))
		}));
		if (form.crawl_method && !crawlMethodOptions.includes(form.crawl_method)) {
			items.push({
				value: form.crawl_method,
				label: t(`intel.method_${form.crawl_method}`, form.crawl_method.replace(/_/g, ' '))
			});
		}
		return items;
	});

	// Same preserve-current-value rule as methodItems: non-crawlable rows
	// carry crawl_frequency='manual'.
	const frequencyItems = $derived.by(() => {
		const items = crawlFrequencyOptions.map((v) => ({
			value: v,
			label: t(`intel.freq_${v}`, v.replace(/_/g, ' '))
		}));
		if (form.crawl_frequency && !crawlFrequencyOptions.includes(form.crawl_frequency)) {
			items.push({
				value: form.crawl_frequency,
				label: t(`intel.freq_${form.crawl_frequency}`, form.crawl_frequency.replace(/_/g, ' '))
			});
		}
		return items;
	});

	// Source-kind options come from meta.source_kinds ({value,label} pairs —
	// who publishes the source). The meta label wins; the i18n key is the
	// fallback for values appended off-catalog.
	const kindItems = $derived.by(() => {
		const items = sourceKindOptions.map((k) => ({
			value: k.value,
			label: k.label || t(`intel.kind_${k.value}`, k.value.replace(/_/g, ' '))
		}));
		if (form.source_kind && !sourceKindOptions.some((k) => k.value === form.source_kind)) {
			items.push({
				value: form.source_kind,
				label: t(`intel.kind_${form.source_kind}`, form.source_kind.replace(/_/g, ' '))
			});
		}
		return items;
	});

	// Source-type options come from /api/v1/sources/meta (Rule 16 — no
	// hardcoded list). The column is free text in the DB, so the current
	// value is appended when absent to avoid clobbering legacy/seeded
	// values (e.g. 'government', 'web') on edit.
	const typeItems = $derived.by(() => {
		const items = sourceTypeOptions.map((v) => ({
			value: v,
			label: t(`intel.type_${v}`, v.replace(/_/g, ' '))
		}));
		if (form.source_type && !sourceTypeOptions.includes(form.source_type)) {
			items.push({
				value: form.source_type,
				label: t(`intel.type_${form.source_type}`, form.source_type.replace(/_/g, ' '))
			});
		}
		return items;
	});

	const trustItems = $derived([
		{ value: '1', label: t('intel.trust_official', '1 — Official') },
		{ value: '2', label: t('intel.trust_trusted', '2 — Trusted') },
		{ value: '3', label: t('intel.trust_aggregator', '3 — Aggregator') },
		{ value: '4', label: t('intel.trust_community', '4 — Community') }
	]);

	const isEdit = $derived(!!source?.id);
	const valid = $derived(
		form.name.trim().length > 0 &&
			form.source_url.trim().length > 0 &&
			(form.categories.length > 0 || form.category.trim().length > 0)
	);

	// Sentinel for the "No persona" option. The Autocomplete fetcher
	// includes this as a selectable item; onPersonaChange converts it
	// back to '' (empty = no persona / system default).
	const NONE_PERSONA = '__none__';

	function onPersonaChange(v: string | undefined) {
		form.persona_id = !v || v === NONE_PERSONA ? '' : v;
	}

	// Multi-category toggle: categories[] is the submitted set; category is
	// kept as the first selection for back-compat with the single-category
	// column the admin create/update endpoints still write.
	function toggleCategory(value: string, checked: boolean) {
		form.categories = checked
			? [...form.categories, value]
			: form.categories.filter((v) => v !== value);
		form.category = form.categories[0] ?? '';
	}

	function toggleEntityType(value: string, checked: boolean) {
		form.entity_types = checked
			? [...form.entity_types, value]
			: form.entity_types.filter((v) => v !== value);
	}

	// --- Autocomplete fetcher for AI Persona ---
	// Filters the personas prop (server-fetched, unbounded list) client-side.
	async function fetchPersonas(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const noneItem: AutocompleteItem = { id: NONE_PERSONA, label: t('intel.field_persona_none', 'No persona (use system default)') };
		const matched = personas
			.filter((p) => p.profile_name.toLowerCase().includes(query) || p.id.toLowerCase().includes(query))
			.map((p) => ({ id: p.id, label: p.profile_name, sublabel: p.backing_username }));
		return [noneItem, ...matched];
	}

	async function fetchPersonasInitial(): Promise<AutocompleteItem[]> {
		const noneItem: AutocompleteItem = { id: NONE_PERSONA, label: t('intel.field_persona_none', 'No persona (use system default)') };
		return [noneItem, ...personas.map((p) => ({ id: p.id, label: p.profile_name, sublabel: p.backing_username }))];
	}

	function submit(e?: SubmitEvent) {
		e?.preventDefault();
		if (!valid || saving) return;
		// Dual-write: categories[] carries the full multi-select while
		// category stays the first selection (the admin create/update
		// handlers still key off the single category field).
		onSubmit({ ...form, category: form.categories[0] ?? form.category });
	}

	const inputClass =
		'border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus';
	const labelClass = 'text-foreground block text-sm font-medium';
</script>

<Modal
	bind:open
	title={isEdit ? t('intel.edit_source', 'Edit Source') : t('intel.new_source', 'New Source')}
	description={isEdit
		? t('intel.edit_source_desc', 'Update the intelligence source registry entry.')
		: t('intel.new_source_desc', 'Register a new intelligence source for the crawl pipeline.')}
	size="max-w-2xl"
>
	{#snippet children(close)}
		<div>
			{#if errorMessage}
				<div class="bg-destructive/10 text-destructive mt-3 rounded-md p-3 text-sm">
					{errorMessage}
				</div>
			{/if}

			<form onsubmit={submit} class="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-name" class={labelClass}>
							{t('intel.field_name', 'Name')} <span class="text-destructive">*</span>
						</label>
						<input id="sf-name" type="text" bind:value={form.name} class={inputClass} placeholder="e.g. DAAD Scholarships" />
					</div>
					<div>
						<label for="sf-domain" class={labelClass}>{t('intel.field_domain', 'Domain')}</label>
						<input id="sf-domain" type="text" bind:value={form.domain} class={inputClass} placeholder="daad.de" />
					</div>
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-url" class={labelClass}>
							{t('intel.field_source_url', 'Source URL')} <span class="text-destructive">*</span>
						</label>
						<input id="sf-url" type="url" bind:value={form.source_url} class={inputClass} placeholder="https://www.daad.de/en/" />
					</div>
					<div>
						<label for="sf-logo" class={labelClass}>{t('intel.field_logo_url', 'Logo URL')}</label>
						<input id="sf-logo" type="url" bind:value={form.logo_url} class={inputClass} placeholder="https://daad.de/logo.png" />
					</div>
				</div>

				<div>
					<label for="sf-description" class={labelClass}>{t('intel.field_description', 'Description')}</label>
					<textarea id="sf-description" bind:value={form.description} class={inputClass} rows={2} placeholder={t('intel.field_description_placeholder', 'What this source publishes')}></textarea>
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-slug" class={labelClass}>{t('intel.field_slug', 'Slug')}</label>
						<input id="sf-slug" type="text" bind:value={form.slug} class={inputClass} placeholder={t('intel.field_slug_placeholder', 'auto from name')} />
					</div>
					<div>
						<label for="sf-kind" class={labelClass}>{t('intel.field_source_kind', 'Source Kind')}</label>
						<Select
							id="sf-kind"
							value={form.source_kind}
							onValueChange={(v) => { form.source_kind = v; }}
							items={kindItems}
							placeholder={t('intel.field_source_kind', 'Source Kind')}
							triggerClass="mt-1 w-full"
							zIndex={70}
						/>
					</div>
				</div>

				<div>
					<span class={labelClass}>{t('intel.field_categories', 'Categories')}</span>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('intel.field_categories_hint', 'Every domain this source publishes — the first selection is stored as the primary category.')}
					</p>
					{#if categoryChecklist.length > 0}
						<div class="mt-2 grid max-h-40 grid-cols-2 gap-x-4 gap-y-1 overflow-y-auto sm:grid-cols-3">
							{#each categoryChecklist as c (c.value)}
								<label class="flex cursor-pointer items-center gap-2 text-sm">
									<input
										type="checkbox"
										checked={form.categories.includes(c.value)}
										onchange={(e) => toggleCategory(c.value, e.currentTarget.checked)}
										class="border-border-input bg-background size-3.5 rounded-sm accent-accent-foreground"
									/>
									{c.label}
								</label>
							{/each}
						</div>
					{:else}
						<!-- Fallback when the meta catalog failed to load: keep the
							legacy single-category select rather than a dead form. -->
						<Select
							value={form.category}
							onValueChange={(v) => { form.category = v; }}
							items={categoryItems}
							placeholder={t('intel.field_category', 'Category')}
							triggerClass="mt-1 w-full"
							zIndex={70}
						/>
					{/if}
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-type" class={labelClass}>{t('intel.field_source_type', 'Source Type')}</label>
						<Select
							id="sf-type"
							value={form.source_type}
							onValueChange={(v) => { form.source_type = v; }}
							items={typeItems}
							placeholder={t('intel.field_source_type', 'Source Type')}
							triggerClass="mt-1 w-full"
							zIndex={70}
						/>
					</div>
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-trust" class={labelClass}>{t('intel.field_trust_level', 'Trust Level')}</label>
						<Select
							id="sf-trust"
							value={String(form.trust_level)}
							onValueChange={(v) => { form.trust_level = Number(v); }}
							items={trustItems}
							triggerClass="mt-1 w-full"
							zIndex={70}
						/>
					</div>
					<div>
						<label for="sf-method" class={labelClass}>{t('intel.field_crawl_method', 'Crawl Method')}</label>
						<Select
							id="sf-method"
							value={form.crawl_method}
							onValueChange={(v) => { form.crawl_method = v; }}
							items={methodItems}
							triggerClass="mt-1 w-full"
							zIndex={70}
						/>
					</div>
				</div>

				{#if form.source_type === 'video'}
					<p class="text-muted-foreground -mt-1 text-xs">
						{t('intel.type_video_hint', 'Video platform URLs register as a channel subscription (video_channel) — the channel sync imports uploads as video content; crawl method/frequency are set to manual.')}
					</p>
				{/if}

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-freq" class={labelClass}>{t('intel.field_crawl_frequency', 'Crawl Frequency')}</label>
						<Select
							id="sf-freq"
							value={form.crawl_frequency}
							onValueChange={(v) => { form.crawl_frequency = v; }}
							items={frequencyItems}
							triggerClass="mt-1 w-full"
							zIndex={70}
						/>
					</div>
					<div>
						<label for="sf-mins" class={labelClass}>{t('intel.field_crawl_mins', 'Crawl Interval (mins)')}</label>
						<input id="sf-mins" type="number" min="0" bind:value={form.crawl_frequency_mins} class={inputClass} />
					</div>
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-license" class={labelClass}>{t('intel.field_license', 'Source License')}</label>
						<input id="sf-license" type="text" bind:value={form.source_license} class={inputClass} placeholder="CC-BY-4.0" />
					</div>
					<div>
						<label for="sf-restriction" class={labelClass}>{t('intel.field_usage_restriction', 'Usage Restriction')}</label>
						<input id="sf-restriction" type="text" bind:value={form.usage_restriction} class={inputClass} placeholder="non-commercial" />
					</div>
				</div>

				<div>
					<label for="sf-persona" class={labelClass}>{t('intel.field_persona', 'AI Persona')}</label>
					<Autocomplete
						id="sf-persona"
						placeholder={t('intel.field_persona_none', 'No persona (use system default)')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchPersonas}
						initialFetcher={fetchPersonasInitial}
						selectedId={form.persona_id || null}
						selectedLabel={form.persona_id ? (personas.find((x) => x.id === form.persona_id)?.profile_name ?? form.persona_id) : null}
						onSelect={(item) => onPersonaChange(item.id)}
						onClear={() => onPersonaChange(NONE_PERSONA)}
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('intel.field_persona_hint', "The persona's backing bot user will be credited as the author of content promoted from this source.")}
					</p>
				</div>

				{#if entityTypeOptions.length > 0}
					<div>
						<span class={labelClass}>{t('intel.field_entity_types', 'Extraction domains (entity types)')}</span>
						<p class="text-muted-foreground mt-1 text-xs">
							{t('intel.field_entity_types_hint', 'Leave empty to extract under the category-derived type only. Check the domains a mixed source publishes to extract each of them.')}
						</p>
						<div class="mt-2 grid max-h-40 grid-cols-2 gap-x-4 gap-y-1 overflow-y-auto">
							{#each entityTypeOptions as et}
								<label class="flex cursor-pointer items-center gap-2 text-sm">
									<input
										type="checkbox"
										checked={form.entity_types.includes(et)}
										onchange={(e) => toggleEntityType(et, e.currentTarget.checked)}
										class="border-border-input bg-background size-3.5 rounded-sm accent-accent-foreground"
									/>
									{t(`intel.category_${et}`, et.replace(/_/g, ' '))}
								</label>
							{/each}
						</div>
					</div>
				{/if}

				<div class="flex items-center gap-6">
					<label class="flex cursor-pointer items-center gap-2 text-sm">
						<input type="checkbox" bind:checked={form.attribution_required} class="border-border-input bg-background size-3.5 rounded-sm accent-accent-foreground" />
						{t('intel.field_attribution', 'Attribution required')}
					</label>
					<label class="flex cursor-pointer items-center gap-2 text-sm">
						<input type="checkbox" bind:checked={form.enabled} class="border-border-input bg-background size-3.5 rounded-sm accent-accent-foreground" />
						{t('intel.field_enabled', 'Enabled')}
					</label>
				</div>
			</form>

			<div class="mt-6 flex justify-end gap-3">
				<button
					type="button"
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={close}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					type="button"
					class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					disabled={!valid || saving}
					onclick={() => submit()}
				>
					{#if saving}
						<Loader2 class="size-4 animate-spin" />
						{t('common.saving', 'Saving...')}
					{:else}
						{isEdit ? t('common.save', 'Save') : t('intel.create_source', 'Create Source')}
					{/if}
				</button>
			</div>
		</div>
	{/snippet}
</Modal>
