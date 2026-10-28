<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { Loader2 } from 'lucide-svelte';

	// ChannelEditor is the create/edit dialog for one intel_source_channels
	// row. It is a controlled dialog: the parent (ChannelsTable) owns the
	// `open` state and performs the API call via onSubmit. On PATCH the
	// parent diffs ChannelSubmitValues against the original row — the
	// backend treats entity_types/relevance_* keys as explicit-set.

	// ChannelSubmitValues is the fully-parsed form output: provider_config is
	// a decoded JSON object (undefined when the textarea is empty) and the
	// relevance lists are arrays split from the comma-separated inputs.
	export interface ChannelSubmitValues {
		label: string;
		entry_url: string;
		crawl_method: string;
		pipeline: string;
		entity_types: string[];
		provider_config?: Record<string, unknown>;
		crawl_frequency_mins: number;
		enabled: boolean;
		relevance_mode: string;
		relevance_topics: string[];
		relevance_url_patterns: string[];
	}

	// ChannelLike is the seed shape for editing — a subset of the admin
	// channel entry returned by GET /admin/intel/channels/{id}.
	export interface ChannelLike {
		id?: string;
		label?: string;
		entry_url?: string;
		crawl_method?: string;
		entity_types?: string[] | null;
		pipeline?: string;
		provider_config?: Record<string, unknown> | null;
		crawl_frequency_mins?: number;
		enabled?: boolean;
		relevance_mode?: string;
		relevance_topics?: string[] | null;
		relevance_url_patterns?: string[] | null;
	}

	interface FormState {
		label: string;
		entry_url: string;
		crawl_method: string;
		pipeline: string;
		entity_types: string[];
		provider_config_text: string;
		crawl_frequency_mins: number;
		enabled: boolean;
		relevance_mode: string;
		relevance_topics_text: string;
		relevance_url_patterns_text: string;
	}

	let {
		open = $bindable(false),
		channel = null as ChannelLike | null,
		entityTypeOptions = [] as string[],
		crawlMethodOptions = [] as string[],
		pipelineOptions = [] as string[],
		saving = false,
		errorMessage = '',
		onSubmit
	}: {
		open: boolean;
		channel?: ChannelLike | null;
		entityTypeOptions?: string[];
		crawlMethodOptions?: string[];
		pipelineOptions?: string[];
		saving?: boolean;
		errorMessage?: string;
		onSubmit: (values: ChannelSubmitValues) => void;
	} = $props();

	const empty: FormState = {
		label: '',
		entry_url: '',
		crawl_method: 'static',
		pipeline: 'extract',
		entity_types: [],
		provider_config_text: '',
		crawl_frequency_mins: 1440,
		enabled: true,
		relevance_mode: 'filtered',
		relevance_topics_text: '',
		relevance_url_patterns_text: ''
	};

	let form = $state<FormState>({ ...empty });
	let jsonError = $state('');

	// Re-seed whenever the dialog opens or the channel changes.
	$effect(() => {
		if (open) {
			jsonError = '';
			if (channel) {
				form = {
					label: channel.label ?? '',
					entry_url: channel.entry_url ?? '',
					crawl_method: channel.crawl_method ?? 'static',
					pipeline: channel.pipeline ?? 'extract',
					entity_types: channel.entity_types ?? [],
					provider_config_text:
						channel.provider_config && Object.keys(channel.provider_config).length > 0
							? JSON.stringify(channel.provider_config, null, 2)
							: '',
					crawl_frequency_mins: channel.crawl_frequency_mins ?? 1440,
					enabled: channel.enabled ?? true,
					relevance_mode: channel.relevance_mode ?? 'filtered',
					relevance_topics_text: (channel.relevance_topics ?? []).join(', '),
					relevance_url_patterns_text: (channel.relevance_url_patterns ?? []).join(', ')
				};
			} else {
				form = { ...empty };
			}
		}
	});

	// Channel crawl methods: the meta crawl_methods list (Rule 16) plus the
	// wider intel_source_channels.crawl_method CHECK set (migration 000681 —
	// probe-materialized methods like jsonld/sitemap an admin must be able
	// to re-select). Any current value outside both is appended on edit.
	const CHANNEL_METHODS = [
		'static',
		'rss',
		'api',
		'pdf',
		'ocr',
		'render',
		'jsonld',
		'tabular',
		'sitemap',
		'spider',
		'ical',
		'git',
		'social',
		'search',
		'wayback',
		'xhr_sniff',
		'oneshot',
		'manual',
		'email',
		'webhook'
	];

	const methodItems = $derived.by(() => {
		const seen = new Set<string>();
		const items: { value: string; label: string }[] = [];
		for (const v of [...crawlMethodOptions, ...CHANNEL_METHODS, form.crawl_method]) {
			if (!v || seen.has(v)) continue;
			seen.add(v);
			items.push({ value: v, label: t(`intel.method_${v}`, v.replace(/_/g, ' ')) });
		}
		return items;
	});

	const pipelineItems = $derived.by(() => {
		const seen = new Set<string>();
		const items: { value: string; label: string }[] = [];
		for (const v of [...pipelineOptions, form.pipeline]) {
			if (!v || seen.has(v)) continue;
			seen.add(v);
			items.push({ value: v, label: t(`intel.pipeline_${v}`, v) });
		}
		return items;
	});

	// relevance_mode is a fixed CHECK set (open|filtered|custom) — same
	// hardcoded-option precedent as the trust levels in SourceForm.
	const relevanceItems = $derived([
		{ value: 'open', label: t('intel.relevance_open', 'Open — process every item') },
		{ value: 'filtered', label: t('intel.relevance_filtered', 'Filtered — gate on source defaults') },
		{ value: 'custom', label: t('intel.relevance_custom', 'Custom — gate on topics / URL patterns') }
	]);

	// Inbound/oneshot methods are never scheduled (ListDueChannels excludes
	// them) — a crawl frequency is meaningless, so it must not block submit.
	const SCHEDULED_METHODS = new Set(['email', 'webhook', 'manual', 'oneshot']);
	const isScheduled = $derived(!SCHEDULED_METHODS.has(form.crawl_method));

	const isEdit = $derived(!!channel?.id);
	const valid = $derived(
		form.label.trim().length > 0 &&
			form.entry_url.trim().length > 0 &&
			form.crawl_method.length > 0 &&
			form.pipeline.length > 0 &&
			(!isScheduled || form.crawl_frequency_mins > 0) &&
			jsonError === ''
	);

	function toggleEntityType(value: string, checked: boolean) {
		form.entity_types = checked
			? [...form.entity_types, value]
			: form.entity_types.filter((v) => v !== value);
	}

	// splitList parses a comma-separated input into a trimmed, deduped array.
	function splitList(text: string): string[] {
		const seen = new Set<string>();
		const out: string[] = [];
		for (const part of text.split(/[,\n]/)) {
			const v = part.trim();
			if (v && !seen.has(v)) {
				seen.add(v);
				out.push(v);
			}
		}
		return out;
	}

	function validateProviderConfig(): Record<string, unknown> | undefined {
		const text = form.provider_config_text.trim();
		if (!text) {
			jsonError = '';
			return undefined;
		}
		try {
			const parsed = JSON.parse(text);
			if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
				jsonError = t('intel.provider_config_object', 'Provider config must be a JSON object.');
				return undefined;
			}
			jsonError = '';
			return parsed as Record<string, unknown>;
		} catch {
			jsonError = t('intel.invalid_json', 'Provider config must be valid JSON.');
			return undefined;
		}
	}

	function submit(e?: SubmitEvent) {
		e?.preventDefault();
		const providerConfig = validateProviderConfig();
		if (form.provider_config_text.trim() && providerConfig === undefined) return;
		if (!valid || saving) return;
		onSubmit({
			label: form.label.trim(),
			entry_url: form.entry_url.trim(),
			crawl_method: form.crawl_method,
			pipeline: form.pipeline,
			entity_types: form.entity_types,
			provider_config: providerConfig,
			crawl_frequency_mins: form.crawl_frequency_mins,
			enabled: form.enabled,
			relevance_mode: form.relevance_mode,
			relevance_topics: splitList(form.relevance_topics_text),
			relevance_url_patterns: splitList(form.relevance_url_patterns_text)
		});
	}

	const inputClass =
		'border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus';
	const labelClass = 'text-foreground block text-sm font-medium';
</script>

<Modal
	bind:open
	title={isEdit ? t('intel.edit_channel', 'Edit Channel') : t('intel.new_channel', 'Add Channel')}
	description={isEdit
		? t('intel.edit_channel_desc', 'Update this channel entry point. Only changed fields are sent.')
		: t('intel.new_channel_desc', 'Register a new entry point (URL + method + pipeline) under this source.')}
	size="max-w-2xl"
	zIndex={60}
>
	{#snippet children(close)}
		<div>
			{#if errorMessage}
				<div class="bg-destructive/10 text-destructive mt-3 rounded-md p-3 text-sm">
					{errorMessage}
				</div>
			{/if}

			<form onsubmit={submit} class="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pe-1">
				<div class="grid grid-cols-2 gap-4">
					<div>
						<label for="ce-label" class={labelClass}>
							{t('intel.channel_label', 'Label')} <span class="text-destructive">*</span>
						</label>
						<HintIcon text={t('intel.channel_label_hint', 'A name for this channel — e.g. RSS feed — scholarships.')} />
						<input id="ce-label" type="text" bind:value={form.label} class={inputClass} placeholder={t('intel.channel_label_placeholder', 'e.g. RSS feed — scholarships')} />
					</div>
					<div>
						<label for="ce-url" class={labelClass}>
							{t('intel.channel_entry_url', 'Entry URL')} <span class="text-destructive">*</span>
						</label>
						<HintIcon text={t('intel.channel_url_hint', 'The feed or page URL this channel reads.')} />
						<input id="ce-url" type="text" bind:value={form.entry_url} class={inputClass} placeholder="https://example.org/feed.xml" />
					</div>
				</div>

				<div class="grid grid-cols-3 gap-4">
					<div>
						<label for="ce-method" class={labelClass}>{t('intel.field_crawl_method', 'Method')}</label>
						<HintIcon text={t('intel.crawl_method_hint', 'How content is fetched — RSS, HTML parse, API.')} />
						<AutocompleteSelect
							id="ce-method"
							value={form.crawl_method}
							onValueChange={(v) => { form.crawl_method = v; }}
							options={methodItems}
							class="mt-1 w-full"
						/>
					</div>
					<div>
						<label for="ce-pipeline" class={labelClass}>{t('intel.channel_pipeline', 'Pipeline')}</label>
						<HintIcon text={t('intel.pipeline_hint', 'Which ingestion pipeline processes the results.')} />
						<AutocompleteSelect
							id="ce-pipeline"
							value={form.pipeline}
							onValueChange={(v) => { form.pipeline = v; }}
							options={pipelineItems}
							class="mt-1 w-full"
						/>
					</div>
					<div>
						<label for="ce-mins" class={labelClass}>{t('intel.field_crawl_mins', 'Crawl Interval (mins)')}</label>
						<HintIcon text={t('intel.crawl_mins_hint', 'Minutes between crawls — higher saves quota.')} />
						<input id="ce-mins" type="number" min="1" bind:value={form.crawl_frequency_mins} disabled={!isScheduled} class={inputClass} />
						{#if !isScheduled}
							<p class="text-muted-foreground mt-1 text-xs">
								{t('intel.channel_not_scheduled', 'Inbound — triggered by incoming deliveries, not scheduled.')}
							</p>
						{/if}
					</div>
				</div>

				{#if entityTypeOptions.length > 0}
					<div>
						<span class={labelClass}>{t('intel.field_entity_types', 'Entity types')}</span>
						<HintIcon text={t('intel.channel_entity_types_hint', 'Schemas that validate records from this channel — required for extract and curate pipelines.')} />
						<div class="mt-2 grid max-h-40 grid-cols-2 gap-x-4 gap-y-1 overflow-y-auto sm:grid-cols-3">
							{#each entityTypeOptions as et (et)}
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

				<div>
					<label for="ce-provider" class={labelClass}>{t('intel.channel_provider_config', 'Provider config (JSON)')}</label>
					<textarea
						id="ce-provider"
						bind:value={form.provider_config_text}
						class="{inputClass} font-mono text-xs"
						rows={4}
						placeholder={'{"feed_url": "https://…", "headers": {…}}'}
						oninput={() => { if (jsonError) validateProviderConfig(); }}
					></textarea>
					{#if jsonError}
						<p class="text-destructive mt-1 text-xs">{jsonError}</p>
					{:else}
						<HintIcon text={t('intel.channel_provider_config_hint', 'Optional per-method config (selectors, endpoints, headers). Leave empty for defaults.')} />
					{/if}
				</div>

				<div class="grid grid-cols-1 gap-4">
					<div>
						<label for="ce-relevance" class={labelClass}>{t('intel.channel_relevance_mode', 'Relevance mode')}</label>
						<AutocompleteSelect
							id="ce-relevance"
							value={form.relevance_mode}
							onValueChange={(v) => { form.relevance_mode = v; }}
							options={relevanceItems}
							class="mt-1 w-full"
						/>
					</div>
				</div>

				{#if form.relevance_mode !== 'open'}
					<div class="grid grid-cols-2 gap-4">
						<div>
							<label for="ce-topics" class={labelClass}>{t('intel.channel_relevance_topics', 'Relevance topics')}</label>
							<input
								id="ce-topics"
								type="text"
								bind:value={form.relevance_topics_text}
								class={inputClass}
								placeholder={t('intel.csv_placeholder', 'comma, separated, values')}
							/>
							<p class="text-muted-foreground mt-1 text-xs">
								{form.relevance_mode === 'custom'
									? t('intel.channel_relevance_topics_hint', 'Items must mention one of these topics to pass the gate.')
									: t('intel.channel_relevance_topics_filtered_hint', 'Added to the category-map defaults when gating items.')}
							</p>
						</div>
						<div>
							<label for="ce-patterns" class={labelClass}>{t('intel.channel_relevance_url_patterns', 'Relevance URL patterns')}</label>
							<input
								id="ce-patterns"
								type="text"
								bind:value={form.relevance_url_patterns_text}
								class={inputClass}
								placeholder={t('intel.csv_placeholder', 'comma, separated, values')}
							/>
							<HintIcon text={t('intel.channel_relevance_patterns_hint', 'Substring patterns matched against item URLs.')} />
						</div>
					</div>
					<HintIcon text={t('intel.channel_relevance_cache_hint', 'Verdicts are cached for 7 days — saved topic changes apply to newly crawled items.')} />
				{/if}

				<div class="flex items-center gap-6">
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
						{isEdit ? t('common.save', 'Save') : t('intel.create_channel', 'Create Channel')}
					{/if}
				</button>
			</div>
		</div>
	{/snippet}
</Modal>
