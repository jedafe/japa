<script lang="ts">
	import { Dialog, Select } from 'bits-ui';
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
		source_type: string;
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
		source_type: string;
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

	let {
		open = $bindable(false),
		source = null as SourceLike | null,
		personas = [] as PersonaItem[],
		entityTypeOptions = [] as string[],
		saving = false,
		errorMessage = '',
		onSubmit
	}: {
		open: boolean;
		source?: SourceLike | null;
		personas?: PersonaItem[];
		entityTypeOptions?: string[];
		saving?: boolean;
		errorMessage?: string;
		onSubmit: (values: SourceFormValues) => void;
	} = $props();

	const empty: SourceFormValues = {
		name: '',
		domain: '',
		source_url: '',
		category: 'scholarship',
		source_type: 'web',
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
				form = {
					name: source.name ?? '',
					domain: source.domain ?? '',
					source_url: source.source_url ?? '',
					category: source.category ?? 'scholarship',
					source_type: source.source_type ?? 'web',
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

	const categoryItems = $derived([
		{ value: 'scholarship', label: t('intel.category_scholarship', 'Scholarship') },
		{ value: 'job', label: t('intel.category_job', 'Job') },
		{ value: 'visa', label: t('intel.category_visa', 'Visa') },
		{ value: 'fx', label: t('intel.category_fx', 'FX') },
		{ value: 'remittance', label: t('intel.category_remittance', 'Remittance') },
		{ value: 'news', label: t('intel.category_news', 'News') },
		{ value: 'travel_alert', label: t('intel.category_travel_alert', 'Travel Alert') },
		// INTEL-010 fix: add the 9 new domain categories (ADR-112 expansion).
		{ value: 'housing', label: t('intel.category_housing', 'Housing') },
		{ value: 'cost_of_living', label: t('intel.category_cost_of_living', 'Cost of Living') },
		{ value: 'travel_routes', label: t('intel.category_travel_routes', 'Travel Routes') },
		{ value: 'grants', label: t('intel.category_grants', 'Grants') },
		{ value: 'admission', label: t('intel.category_admission', 'Admission') },
		{ value: 'banking', label: t('intel.category_banking', 'Banking') },
		{ value: 'work_career', label: t('intel.category_work_career', 'Work & Career') },
		{ value: 'health_insurance', label: t('intel.category_health_insurance', 'Health Insurance') },
		{ value: 'culture_lifestyle', label: t('intel.category_culture_lifestyle', 'Culture & Lifestyle') },
		{ value: 'internship', label: t('intel.category_internship', 'Internship') },
		{ value: 'import_export', label: t('intel.category_import_export', 'Import & Export') }
	]);

	const methodItems = $derived([
		{ value: 'static', label: t('intel.method_static', 'Static (HTML)') },
		{ value: 'rss', label: t('intel.method_rss', 'RSS / Atom') },
		{ value: 'api', label: t('intel.method_api', 'REST API') },
		{ value: 'pdf', label: t('intel.method_pdf', 'PDF') },
		{ value: 'ocr', label: t('intel.method_ocr', 'OCR') }
	]);

	const frequencyItems = $derived([
		{ value: 'hourly', label: t('intel.freq_hourly', 'Hourly') },
		{ value: 'daily', label: t('intel.freq_daily', 'Daily') },
		{ value: 'weekly', label: t('intel.freq_weekly', 'Weekly') },
		{ value: 'monthly', label: t('intel.freq_monthly', 'Monthly') }
	]);

	const trustItems = $derived([
		{ value: '1', label: t('intel.trust_official', '1 — Official') },
		{ value: '2', label: t('intel.trust_trusted', '2 — Trusted') },
		{ value: '3', label: t('intel.trust_aggregator', '3 — Aggregator') },
		{ value: '4', label: t('intel.trust_community', '4 — Community') }
	]);

	const isEdit = $derived(!!source?.id);
	const valid = $derived(form.name.trim().length > 0 && form.source_url.trim().length > 0);

	// Sentinel for the "No persona" option. The Autocomplete fetcher
	// includes this as a selectable item; onPersonaChange converts it
	// back to '' (empty = no persona / system default).
	const NONE_PERSONA = '__none__';

	function onPersonaChange(v: string | undefined) {
		form.persona_id = !v || v === NONE_PERSONA ? '' : v;
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

	function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!valid || saving) return;
		onSubmit({ ...form });
	}

	const inputClass =
		'border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus';
	const labelClass = 'text-foreground block text-sm font-medium';
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover focus:outline-none fixed left-1/2 top-1/2 z-50 w-full max-w-2xl -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				{isEdit ? t('intel.edit_source', 'Edit Source') : t('intel.new_source', 'New Source')}
			</Dialog.Title>
			<p class="text-muted-foreground mt-1 text-sm">
				{isEdit
					? t('intel.edit_source_desc', 'Update the intelligence source registry entry.')
					: t('intel.new_source_desc', 'Register a new intelligence source for the crawl pipeline.')}
			</p>

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

				<div>
					<label for="sf-url" class={labelClass}>
						{t('intel.field_source_url', 'Source URL')} <span class="text-destructive">*</span>
					</label>
					<input id="sf-url" type="url" bind:value={form.source_url} class={inputClass} placeholder="https://www.daad.de/en/" />
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-category" class={labelClass}>{t('intel.field_category', 'Category')}</label>
						<Select.Root
							type="single"
							value={form.category}
							onValueChange={(v) => { if (v !== undefined) form.category = v; }}
						>
							<Select.Trigger class={inputClass}>
								<Select.Value placeholder={t('intel.field_category', 'Category')} />
							</Select.Trigger>
							<Select.Portal>
								<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border">
									<Select.Viewport class="p-1">
										{#each categoryItems as c}
											<Select.Item class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none" value={c.value} label={c.label}>
												{c.label}
											</Select.Item>
										{/each}
									</Select.Viewport>
								</Select.Content>
							</Select.Portal>
						</Select.Root>
					</div>
					<div>
						<label for="sf-type" class={labelClass}>{t('intel.field_source_type', 'Source Type')}</label>
						<input id="sf-type" type="text" bind:value={form.source_type} class={inputClass} placeholder="web" />
					</div>
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-trust" class={labelClass}>{t('intel.field_trust_level', 'Trust Level')}</label>
						<Select.Root
							type="single"
							value={String(form.trust_level)}
							onValueChange={(v) => { if (v !== undefined) form.trust_level = Number(v); }}
						>
							<Select.Trigger class={inputClass}>
								<Select.Value />
							</Select.Trigger>
							<Select.Portal>
								<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border">
									<Select.Viewport class="p-1">
										{#each trustItems as tr}
											<Select.Item class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none" value={tr.value} label={tr.label}>
												{tr.label}
											</Select.Item>
										{/each}
									</Select.Viewport>
								</Select.Content>
							</Select.Portal>
						</Select.Root>
					</div>
					<div>
						<label for="sf-method" class={labelClass}>{t('intel.field_crawl_method', 'Crawl Method')}</label>
						<Select.Root
							type="single"
							value={form.crawl_method}
							onValueChange={(v) => { if (v !== undefined) form.crawl_method = v; }}
						>
							<Select.Trigger class={inputClass}>
								<Select.Value />
							</Select.Trigger>
							<Select.Portal>
								<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border">
									<Select.Viewport class="p-1">
										{#each methodItems as m}
											<Select.Item class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none" value={m.value} label={m.label}>
												{m.label}
											</Select.Item>
										{/each}
									</Select.Viewport>
								</Select.Content>
							</Select.Portal>
						</Select.Root>
					</div>
				</div>

				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="sf-freq" class={labelClass}>{t('intel.field_crawl_frequency', 'Crawl Frequency')}</label>
						<Select.Root
							type="single"
							value={form.crawl_frequency}
							onValueChange={(v) => { if (v !== undefined) form.crawl_frequency = v; }}
						>
							<Select.Trigger class={inputClass}>
								<Select.Value />
							</Select.Trigger>
							<Select.Portal>
								<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border">
									<Select.Viewport class="p-1">
										{#each frequencyItems as f}
											<Select.Item class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none" value={f.value} label={f.label}>
												{f.label}
											</Select.Item>
										{/each}
									</Select.Viewport>
								</Select.Content>
							</Select.Portal>
						</Select.Root>
					</div>
					<div>
						<label for="sf-mins" class={labelClass}>{t('intel.field_crawl_mins', 'Crawl Interval (mins)')}</label>
						<input id="sf-mins" type="number" min="1" bind:value={form.crawl_frequency_mins} class={inputClass} />
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
				<Dialog.Close class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors">
					{t('common.cancel', 'Cancel')}
				</Dialog.Close>
				<button
					type="button"
					class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					disabled={!valid || saving}
					onclick={(e) => submit(e as unknown as SubmitEvent)}
				>
					{#if saving}
						<Loader2 class="size-4 animate-spin" />
						{t('common.saving', 'Saving...')}
					{:else}
						{isEdit ? t('common.save', 'Save') : t('intel.create_source', 'Create Source')}
					{/if}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
