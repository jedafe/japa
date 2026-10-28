<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
	// "Suggest an edit" — opens a dialog with the entity's editable field
	// schema (GET /wiki/entities/{type}/{id}/fields) and submits changed
	// fields as set_value ops to POST /wiki/suggest. Renders nothing unless
	// the wiki.structured_edits feature flag is on.
	import { page } from '$app/state';
	import { Modal } from '$lib/components/ui';
	import { Pencil, X, Send, Loader2, Plus, Trash2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import CustomFieldGroup from '$lib/components/create/panel/CustomFieldGroup.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import {
		getEntityFields,
		getFieldTypes,
		suggestFields,
		type SuggestFieldsResult,
		type WikiEntityField,
		type WikiEntityFieldGroup,
		type WikiEntityFields,
		type WikiFieldDefInput,
		type WikiFieldOp
	} from '$lib/api/wiki';
	import { formatWikiFieldValue } from '$lib/utils/wikiFieldValue';
	import type { CurrencyInfo } from '$lib/utils/currency';
	import type { CustomFieldDef } from '$lib/types';

	let {
		entityType,
		entityId,
		label = undefined,
		fieldsData = undefined,
		groupId = undefined,
		proposeGroupSlug = undefined,
		open = $bindable(false),
		showTrigger = true,
		onsubmitted = undefined
	}: {
		entityType: string;
		entityId: string;
		/** Trigger button label override. */
		label?: string;
		/** Prefetched schema — a parent that already loaded GET .../fields can
		 *  pass the result to share one request. */
		fieldsData?: WikiEntityFields | null;
		/** §80 Wiki Types: on a bound wiki page the entity schema spans
		 *  every class-wide group — the dialog must only show the page's
		 *  bound group (matches the old EntityFieldsPanel scoping). */
		groupId?: string;
		/** §80 Wiki Types: the bound category group's slug. Field proposals
		 *  target it implicitly — members never pick a group. */
		proposeGroupSlug?: string;
		open?: boolean;
		/** When false only the dialog renders — open it via bind:open. */
		showTrigger?: boolean;
		onsubmitted?: (result: SuggestFieldsResult) => void;
	} = $props();

	const flagOn = $derived(page.data?.features?.['wiki.structured_edits'] === true);
	const isAuthed = $derived(page.data?.user != null);

	// Synced from the fieldsData prop (or a self-fetch) by the effect below.
	let schema = $state<WikiEntityFields | null>(null);
	let loadingFields = $state(false);
	let loadFailed = $state(false);
	let values = $state<Record<string, unknown>>({});
	// Snapshot of the seeded values — compared per-field at submit to build
	// the changed-only ops array. Non-reactive; only read inside submit().
	let initialValues: Record<string, unknown> = {};
	let summary = $state('');
	let submitting = $state(false);
	let submitted = $state(false);
	let autoApproved = $state(false);
	let errorKey = $state('');

	const pendingCount = $derived(schema?.pending_count ?? 0);
	// §80 Wiki Types — a bound page only edits its own group. The space
	// schema carries every class-wide group (all topic pages share the
	// space's assignments), so the dialog renders scopedGroups, not the
	// raw schema.
	const scopedGroups = $derived(
		groupId ? (schema?.groups ?? []).filter((g) => g.id === groupId) : (schema?.groups ?? [])
	);
	// Propose-tier ops UI: the schema reports can_propose (wiki.fields.propose
	// or manage).
	const canPropose = $derived(schema?.can_propose === true);
	// §80 Super-Admin Direct Publish — the schema reports can_delete only
	// for super_admin group members; they get a per-field delete affordance
	// that queues delete_field ops alongside the value edits.
	const canDelete = $derived(schema?.can_delete === true);

	// ── Delete-field queue (super_admin) ───────────────────────────────
	// Ops queued by the delete affordance — submitted alongside set_value
	// and propose ops; the apply transaction removes the field definition
	// and cascades its stored values.
	let deleteOpen = $state(false);
	let deleteOps = $state<WikiFieldOp[]>([]);
	let deleteLabels = $state<string[]>([]);

	function queueDeleteField(group: WikiEntityFieldGroup, f: WikiEntityField) {
		if (deleteOps.some((o) => o.group_slug === group.slug && o.field_key === f.key)) return;
		deleteOps = [
			...deleteOps,
			{
				op: 'delete_field',
				entity_type: entityType,
				entity_id: entityId,
				group_slug: group.slug,
				field_key: f.key
			}
		];
		deleteLabels = [...deleteLabels, f.label];
	}

	function unqueueDelete(i: number) {
		deleteOps = deleteOps.filter((_, j) => j !== i);
		deleteLabels = deleteLabels.filter((_, j) => j !== i);
	}

	// Fields still deletable in a group — visible, non-system (delete_field
	// refuses is_system server-side) fields not already queued.
	const deletableFields = (g: WikiEntityFieldGroup): WikiEntityField[] =>
		g.fields.filter(
			(f) =>
				f.visible &&
				!f.system &&
				!deleteOps.some((o) => o.group_slug === g.slug && o.field_key === f.key)
		);

	// ── "Propose a new field" form state ─────────────────────────────────
	let proposeOpen = $state(false);
	let fieldTypes = $state<Array<{ value: string; label: string }>>([]);
	let pfLabel = $state('');
	let pfKey = $state('');
	let pfKeyTouched = $state(false);
	let pfType = $state('');
	let pfOptions = $state('');
	let pfGroupSlug = $state('');
	let pfSection = $state('');
	let pfError = $state('');
	// Ops queued by the propose form — submitted alongside set_value ops.
	let proposedOps = $state<WikiFieldOp[]>([]);
	let proposedLabels = $state<string[]>([]);

	function slugifyFieldKey(s: string): string {
		return s
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9]+/g, '_')
			.replace(/^_+|_+$/g, '');
	}

	function onPfLabelInput(e: Event) {
		pfLabel = (e.currentTarget as HTMLInputElement).value;
		if (!pfKeyTouched) pfKey = slugifyFieldKey(pfLabel);
	}

	function onPfKeyInput(e: Event) {
		pfKeyTouched = true;
		pfKey = slugifyFieldKey((e.currentTarget as HTMLInputElement).value);
	}

	async function togglePropose() {
		proposeOpen = !proposeOpen;
		if (proposeOpen && fieldTypes.length === 0) {
			fieldTypes = await getFieldTypes();
			if (fieldTypes.length > 0 && !pfType) pfType = fieldTypes[0].value;
		}
	}

	// Choice-bearing types: select/multiselect require options at submit;
	// suggest accepts an optional static list alongside its option sources;
	// price_parts collects its part columns as comma-separated labels.
	const CHOICE_TYPES = new Set(['select', 'multiselect', 'suggest', 'price_parts']);

	// §80: the proposal target group is implicit — the bound category group
	// (proposeGroupSlug) when the page binds one, else the schema's sole
	// group. A picker only renders when several groups could apply.
	const resolvedProposeGroup = $derived(
		proposeGroupSlug ?? (scopedGroups.length === 1 ? scopedGroups[0].slug : '')
	);
	const showGroupPicker = $derived(!resolvedProposeGroup && scopedGroups.length > 1);
	const pfOptionsRequired = $derived(
		pfType === 'select' || pfType === 'multiselect' || pfType === 'price_parts'
	);

	// §80 Itemized Pricing: section tokens already used in the proposal's
	// target group — offered as autocomplete suggestions so a new item slots
	// into an existing item table instead of fragmenting sections.
	const existingSections = $derived.by(() => {
		const target = resolvedProposeGroup || pfGroupSlug;
		const seen = new Set<string>();
		for (const g of scopedGroups) {
			if (target && g.slug !== target) continue;
			for (const f of g.fields) {
				if (f.section) seen.add(f.section);
			}
		}
		return [...seen].sort();
	});

	const sectionItems = $derived<AutocompleteItem[]>(
		existingSections.map((s) => ({ id: s, label: s }))
	);
	async function sectionFetcher(q: string): Promise<AutocompleteItem[]> {
		const query = q.trim().toLowerCase();
		return sectionItems.filter((i) => i.id.toLowerCase().includes(query));
	}
	async function sectionInitial(): Promise<AutocompleteItem[]> {
		return sectionItems;
	}

	function addProposedField() {
		pfError = '';
		const label = pfLabel.trim();
		const key = pfKey.trim();
		if (!label || !key) {
			pfError = 'wiki.missing_fields';
			return;
		}
		if (!pfType) {
			pfError = 'wiki.missing_fields';
			return;
		}
		const options = pfOptions
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean)
			.map((s) => ({ value: slugifyFieldKey(s), label: s }));
		if (pfOptionsRequired && options.length === 0) {
			pfError = 'wiki.propose_options_required';
			return;
		}
		const field: WikiFieldDefInput = { key, label, field_type: pfType };
		// §80 Wiki Topic Pages — price_parts members propose the column
		// list as comma-separated labels; parts default to money (admins
		// refine kinds/primary_part in the Admin Panel).
		if (pfType === 'price_parts') {
			field.options = {
				parts: options.map((o) => ({ key: o.value, label: o.label, kind: 'money' }))
			};
		} else if (options.length > 0) {
			field.options = options;
		}
		// §80: a section token places the field in that item-table section;
		// slugified to the ^[a-z0-9_]+$ token the backend validates.
		const section = slugifyFieldKey(pfSection);
		if (section) field.section = section;
		const targetGroup = resolvedProposeGroup || pfGroupSlug;
		if (!targetGroup) {
			pfError = 'wiki.missing_fields';
			return;
		}
		proposedOps = [
			...proposedOps,
			{ op: 'propose_field', group_slug: targetGroup, field }
		];
		proposedLabels = [...proposedLabels, label];
		pfLabel = '';
		pfKey = '';
		pfKeyTouched = false;
		pfOptions = '';
		pfSection = '';
	}

	$effect(() => {
		if (fieldsData !== undefined) {
			schema = fieldsData;
			return;
		}
		// Prefetch on mount so the pending-count badge renders without the
		// dialog being opened. Retries are triggered by re-opening.
		if (!flagOn || schema || loadingFields || loadFailed) return;
		void loadSchema();
	});

	// Reset transient state on each closed→open transition — whether via the
	// trigger button or a parent driving bind:open (per-field edit pencils).
	// Guarded by wasOpen so a schema refresh while open (e.g. the post-submit
	// pending_count bump) doesn't wipe the form or the success state.
	let wasOpen = false;
	$effect(() => {
		if (open && !wasOpen) {
			submitted = false;
			autoApproved = false;
			errorKey = '';
			summary = '';
			proposedOps = [];
			proposedLabels = [];
			proposeOpen = false;
			pfError = '';
			deleteOps = [];
			deleteLabels = [];
			deleteOpen = false;
			if (schema) seedValues(schema);
			else if (fieldsData === undefined && !loadingFields) void loadSchema();
		}
		wasOpen = open;
	});

	function seedValues(d: WikiEntityFields | null) {
		const v: Record<string, unknown> = {};
		if (d) {
			for (const g of groupId ? d.groups.filter((g) => g.id === groupId) : d.groups) {
				for (const f of g.fields) {
					if (!f.visible) continue;
					let val = f.value ?? '';
					// value_date arrives as RFC3339; <input type="date"> needs YYYY-MM-DD.
					if (f.field_type === 'date' && typeof val === 'string' && val.length > 10) {
						val = val.slice(0, 10);
					}
					v[f.key] = val;
				}
			}
		}
		values = v;
		initialValues = { ...v };
	}

	async function loadSchema() {
		loadingFields = true;
		loadFailed = false;
		const data = await getEntityFields(entityType, entityId);
		loadingFields = false;
		if (data) {
			schema = data;
			seedValues(data);
		} else {
			loadFailed = true;
		}
	}

	function openDialog() {
		open = true;
	}

	function toDef(f: WikiEntityField): CustomFieldDef {
		return {
			id: f.id,
			key: f.key,
			label: f.label,
			field_type: f.field_type,
			is_required: false,
			options: (f.options as CustomFieldDef['options']) ?? null
		};
	}

	const editableDefs = (g: WikiEntityFieldGroup): CustomFieldDef[] =>
		g.fields.filter((f) => f.visible && f.can_edit).map(toDef);

	const readOnlyFields = (g: WikiEntityFieldGroup): WikiEntityField[] =>
		g.fields.filter((f) => f.visible && !f.can_edit);

	const hasVisibleFields = $derived(
		scopedGroups.some((g) => g.fields.some((f) => f.visible))
	);

	// §58 — layout-level currency reference for currency/price_range display.
	const currencies = $derived((page.data?.currencies ?? null) as CurrencyInfo[] | null);

	function formatValue(f: WikiEntityField): string {
		const v = f.value;
		if (v === null || v === undefined || v === '') return '—';
		if (f.field_type === 'currency' || f.field_type === 'price_range' || f.field_type === 'price_parts') {
			return formatWikiFieldValue(f, currencies, schema?.entity_currency_code ?? '', schema?.entity_currency_fx);
		}
		if (typeof v === 'boolean') return v ? t('common.yes', 'Yes') : t('common.no', 'No');
		if (typeof v === 'number') return String(v);
		if (typeof v === 'string') return v;
		// §80 media fields resolve to {id,url,alt} objects — render alt text
		// or a count instead of a JSON blob.
		if (f.field_type === 'image_single' || f.field_type === 'image_multi' || f.field_type === 'media') {
			const items = Array.isArray(v) ? v : [v];
			const labels = items
				.map((i) => (i && typeof i === 'object' ? ((i as { alt?: string }).alt ?? '') : ''))
				.filter(Boolean);
			if (labels.length > 0) return labels.join(', ');
			return t('wiki.media_count', '{count} image(s)', { count: items.length });
		}
		if (Array.isArray(v)) {
			return v
				.map((i) => (typeof i === 'object' && i !== null ? JSON.stringify(i) : String(i)))
				.join(', ');
		}
		return JSON.stringify(v);
	}

	// Controls emit '' when cleared (select/date/time/datetime/text inputs)
	// and NaN for emptied number inputs — both mean "no value", which the
	// backend expects as JSON null. Normalizing before the change check also
	// keeps a cleared-but-already-empty field from producing a no-op
	// set_value.
	function normValue(v: unknown): unknown {
		if (v === '' || v === null || v === undefined) return null;
		if (typeof v === 'number' && Number.isNaN(v)) return null;
		if (Array.isArray(v) && v.length === 0) return null;
		return v;
	}

	function changedOps(): WikiFieldOp[] {
		const ops: WikiFieldOp[] = [];
		for (const g of scopedGroups) {
			for (const f of g.fields) {
				if (!f.visible || !f.can_edit) continue;
				// <input type="datetime-local"> carries the wall-clock time
				// with no zone; emit UTC RFC3339 so the backend stores the
				// instant the user saw, not the same digits read as UTC.
				const norm = (v: unknown) => {
					const n = normValue(v);
					if (n !== null && f.field_type === 'datetime') {
						const d = new Date(n as string);
						if (!Number.isNaN(d.getTime())) return d.toISOString();
					}
					return n;
				};
				const cur = norm(values[f.key]);
				const init = norm(initialValues[f.key]);
				if (JSON.stringify(cur) === JSON.stringify(init)) continue;
				ops.push({
					op: 'set_value',
					entity_type: entityType,
					entity_id: entityId,
					field_key: f.key,
					value: cur
				});
			}
		}
		return ops;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		const ops = [...changedOps(), ...proposedOps, ...deleteOps];
		if (ops.length === 0) {
			errorKey = 'wiki.suggest_no_changes';
			return;
		}
		submitting = true;
		const res = await suggestFields({
			entity_type: entityType,
			entity_id: entityId,
			ops,
			summary: summary.trim()
		});
		submitting = false;
		if (res.data) {
			submitted = true;
			autoApproved = res.data.auto_approved;
			if (!autoApproved && schema) {
				schema = { ...schema, pending_count: schema.pending_count + 1 };
			}
			onsubmitted?.(res.data);
		} else {
			// Surface the backend's structured error code (e.g. forbidden).
			errorKey = res.error ? `wiki.${res.error.code}` : 'wiki.suggest_failed';
		}
	}
</script>

{#if flagOn}
	{#if showTrigger}
		<button
			type="button"
			onclick={openDialog}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<Pencil size={14} />
			{label ?? t('wiki.suggest_edit', 'Suggest an edit')}
			{#if pendingCount > 0}
				<span
					class="rounded-full bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600"
				>
					{t('wiki.suggest_pending_badge', '{count} pending', { count: pendingCount })}
				</span>
			{/if}
		</button>
	{/if}

	<Modal bind:open hideHeader ariaTitle={label ?? t('wiki.suggest_edit', 'Suggest an edit')} padded={false} overlayClass="backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out" class="bg-[var(--color-bg)]" size="max-w-[560px]">
		{#snippet children(close)}
				<div class="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-4">
					<h2 class="text-base font-semibold text-[var(--color-text)]">
						{label ?? t('wiki.suggest_edit', 'Suggest an edit')}
					</h2>
					<button type="button" onclick={close} aria-label={t('common.close', 'Close')} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
						<X size={18} />
					</button>
				</div>

				<div class="flex-1 overflow-y-auto px-5 py-4">
					{#if !isAuthed}
						<p class="text-sm text-[var(--color-muted)]">
							{t('wiki.suggest_sign_in', 'Sign in to suggest an edit.')}
						</p>
						<a
							href="/login"
							class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90"
						>
							{t('common.sign_in', 'Sign in')}
						</a>
					{:else if submitted}
						<div class="rounded-[12px] border border-green-500/30 bg-green-500/10 p-4 text-center">
							<p class="text-sm font-medium text-green-600">
								{autoApproved
									? t('wiki.suggest_approved', 'Your changes were applied.')
									: t('wiki.suggest_submitted', 'Your suggestion has been submitted for review.')}
							</p>
						</div>
					{:else if loadingFields}
						<div class="flex items-center gap-2 py-6 text-sm text-[var(--color-muted)]">
							<Loader2 size={14} class="animate-spin" />
							{t('wiki.fields_loading', 'Loading fields…')}
						</div>
					{:else if loadFailed}
						<div class="py-4 text-sm text-[var(--color-muted)]">
							<p>{t('wiki.load_error', 'Wiki could not be loaded.')}</p>
							<button
								type="button"
								onclick={() => void loadSchema()}
								class="mt-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('common.retry', 'Try again')}
							</button>
						</div>
					{:else if schema && !hasVisibleFields && !(canPropose && (resolvedProposeGroup !== '' || showGroupPicker))}
						<p class="py-4 text-sm text-[var(--color-muted)]">
							{t('wiki.fields_empty', 'No editable fields for this entry.')}
						</p>
					{:else if schema}
						<p class="mb-4 text-xs text-[var(--color-muted)]">
							{t(
								'wiki.suggest_edit_body',
								'Propose changes to these fields. A moderator will review your suggestion.'
							)}
						</p>
						<form onsubmit={submit} class="space-y-5" novalidate>
							{#each scopedGroups as group (group.id)}
								{@const defs = editableDefs(group)}
								{@const readonly = readOnlyFields(group)}
								{#if defs.length > 0 || readonly.length > 0}
									<section>
										<h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
											{group.name}
										</h3>
										{#if defs.length > 0}
											<CustomFieldGroup
												fields={defs}
												{values}
												onvalues={(v) => (values = v)}
												onedit={() => {}}
												contextCurrencyCode={schema?.entity_currency_code ?? ''}
											/>
										{/if}
										{#if readonly.length > 0}
											<dl class="mt-3 space-y-1.5">
												{#each readonly as f (f.id)}
													<div class="flex items-baseline justify-between gap-3 text-sm">
														<dt class="text-[var(--color-muted)]">{f.label}</dt>
														<dd class="text-end text-[var(--color-text)]">
															{formatValue(f)}
														</dd>
													</div>
												{/each}
											</dl>
										{/if}
									</section>
								{/if}
							{/each}

							{#if canPropose && (resolvedProposeGroup !== '' || showGroupPicker)}
								<section class="rounded-[var(--radius)] border border-dashed border-[var(--color-border)] p-3">
									<button
										type="button"
										onclick={togglePropose}
										class="flex w-full items-center gap-1.5 text-start text-xs font-medium text-[var(--color-text)]"
									>
										<Plus size={13} class="text-[var(--color-muted)]" />
										{t('wiki.propose_field', 'Propose a new field')}
									</button>
									{#if proposeOpen}
										<p class="mt-2 text-xs text-[var(--color-muted)]">
											{t('wiki.propose_field_hint', "Suggest a field that isn't listed yet.")}
										</p>
										<div class="mt-3 space-y-3">
											<div class="flex flex-col gap-1">
												<label for="pf-label" class="text-xs font-medium text-[var(--color-text)]">
													{t('wiki.field_label', 'Field label')}
												</label>
												<p class="mt-1 text-xs text-[var(--color-muted)]">{t('wiki.suggest.label_hint', 'The human-readable name shown to readers.')}</p>
												<input
													id="pf-label"
													type="text"
													value={pfLabel}
													oninput={onPfLabelInput}
													class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
												/>
											</div>
											<div class="flex flex-col gap-1">
												<label for="pf-key" class="text-xs font-medium text-[var(--color-text)]">
													{t('wiki.field_key', 'Field key')}
												</label>
												<p class="mt-1 text-xs text-[var(--color-muted)]">{t('wiki.suggest.key_hint', 'The internal key — lowercase, no spaces.')}</p>
												<input
													id="pf-key"
													type="text"
													value={pfKey}
													oninput={onPfKeyInput}
													class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 font-mono text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
												/>
											</div>
											<div class="flex flex-col gap-1">
												<label for="pf-type" class="text-xs font-medium text-[var(--color-text)]">
													{t('wiki.field_type', 'Field type')}
												</label>
												<p class="mt-1 text-xs text-[var(--color-muted)]">{t('wiki.suggest.type_hint', 'The kind of value — text, date, number.')}</p>
												<AutocompleteSelect
													id="pf-type"
													bind:value={pfType}
													options={fieldTypes.map((ft) => ({ value: ft.value, label: t(`wiki.field_type_${ft.value}`, ft.label) }))}
												/>
											</div>
											{#if CHOICE_TYPES.has(pfType)}
												<div class="flex flex-col gap-1">
													<label for="pf-options" class="text-xs font-medium text-[var(--color-text)]">
														{t('wiki.propose_options', 'Options')}{#if pfOptionsRequired}<span class="text-red-500">*</span>{/if}
													</label>
													<p class="mt-1 text-xs text-[var(--color-muted)]">{t('wiki.suggest.options_hint', 'Allowed values, comma-separated.')}</p>
													<input
														id="pf-options"
														type="text"
														bind:value={pfOptions}
														placeholder={t('wiki.propose_options_hint', 'Comma-separated choices')}
														class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
													/>
												</div>
											{/if}
											<div class="flex flex-col gap-1">
												<label for="pf-section" class="text-xs font-medium text-[var(--color-text)]">
													{t('wiki.field_section', 'Section')}
												</label>
												<Autocomplete
													id="pf-section"
													freeText
													placeholder={t('wiki.field_section_hint', 'Optional — groups the field into an item table')}
													emptyMessage={t('common.no_results', 'No results found')}
													fetcher={sectionFetcher}
													initialFetcher={sectionInitial}
													selectedId={null}
													selectedLabel={pfSection}
													onSelect={(i) => (pfSection = i.id)}
													onFreeTextBlur={(q) => (pfSection = q)}
												/>
											</div>
											{#if showGroupPicker}
												<div class="flex flex-col gap-1">
													<label for="pf-group" class="text-xs font-medium text-[var(--color-text)]">
														{t('wiki.field_group', 'Group')}
													</label>
													<AutocompleteSelect
														id="pf-group"
														bind:value={pfGroupSlug}
														options={[
															{ value: '', label: '—' },
															...scopedGroups.map((g) => ({ value: g.slug, label: g.name }))
														]}
													/>
												</div>
											{/if}
											{#if pfError}
												<p class="text-xs text-red-500" role="alert">{t(pfError, pfError)}</p>
											{/if}
											<button
												type="button"
												onclick={addProposedField}
												class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
											>
												<Plus size={13} /> {t('wiki.propose_add', 'Add proposal')}
											</button>
										</div>
									{/if}
									{#if proposedOps.length > 0}
										<ul class="mt-3 space-y-1.5">
											{#each proposedLabels as label, i (i)}
												<li
													class="flex items-center justify-between gap-2 rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2.5 py-1.5 text-xs text-[var(--color-text)]"
												>
													<span class="min-w-0 truncate">
														{t('wiki.propose_added', 'Proposed: {label}').replace('{label}', label)}
													</span>
													<button
														type="button"
														onclick={() => {
															proposedOps = proposedOps.filter((_, j) => j !== i);
															proposedLabels = proposedLabels.filter((_, j) => j !== i);
														}}
														class="shrink-0 text-[var(--color-muted)] transition-colors hover:text-red-500"
														aria-label={t('wiki.propose_remove', 'Remove')}
													>
														<Trash2 size={12} />
													</button>
												</li>
											{/each}
										</ul>
									{/if}
								</section>
							{/if}

							{#if canDelete && hasVisibleFields}
								<!-- §80 Super-Admin Direct Publish — delete_field queue.
								     A clicked field moves into the marked-for-deletion list;
								     the ops submit with everything else in one revision. -->
								<section class="rounded-[var(--radius)] border border-dashed border-red-500/40 p-3">
									<button
										type="button"
										onclick={() => (deleteOpen = !deleteOpen)}
										class="flex w-full items-center gap-1.5 text-start text-xs font-medium text-red-600"
									>
										<Trash2 size={13} />
										{t('wiki.delete_fields', 'Delete fields')}
									</button>
									{#if deleteOpen}
										<p class="mt-2 text-xs text-[var(--color-muted)]">
											{t(
												'wiki.delete_fields_hint',
												'Deleting a field removes the field and every stored value. Marked fields are deleted when you submit.'
											)}
										</p>
										<div class="mt-3 space-y-3">
											{#each scopedGroups as group (group.id)}
												{@const deletable = deletableFields(group)}
												{#if deletable.length > 0}
													<div>
														<h4 class="mb-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
															{group.name}
														</h4>
														<ul class="divide-y divide-[var(--color-border)]">
															{#each deletable as f (f.id)}
																<li class="flex items-center justify-between gap-3 py-1.5 text-sm">
																	<span class="min-w-0 truncate text-[var(--color-text)]">{f.label}</span>
																	<button
																		type="button"
																		onclick={() => queueDeleteField(group, f)}
																		class="shrink-0 text-[var(--color-muted)] transition-colors hover:text-red-500"
																		aria-label={t('wiki.delete_field_mark', 'Mark {label} for deletion', { label: f.label })}
																		title={t('wiki.delete_field_mark', 'Mark {label} for deletion', { label: f.label })}
																	>
																		<Trash2 size={13} />
																	</button>
																</li>
															{/each}
														</ul>
													</div>
												{/if}
											{/each}
										</div>
									{/if}
									{#if deleteLabels.length > 0}
										<ul class="mt-3 space-y-1.5">
											{#each deleteLabels as label, i (i)}
												<li
													class="flex items-center justify-between gap-2 rounded-[var(--radius)] bg-red-500/10 px-2.5 py-1.5 text-xs text-red-600"
												>
													<span class="min-w-0 truncate">
														{t('wiki.delete_queued', 'Will be deleted: {label}', { label })}
													</span>
													<button
														type="button"
														onclick={() => unqueueDelete(i)}
														class="shrink-0 text-red-600/70 transition-colors hover:text-red-600"
														aria-label={t('wiki.propose_remove', 'Remove')}
													>
														<X size={12} />
													</button>
												</li>
											{/each}
										</ul>
									{/if}
								</section>
							{/if}

							<div class="flex flex-col gap-1.5">
								<label for="suggest-summary" class="text-sm font-medium text-[var(--color-text)]">
									{t('wiki.edit_summary_label', 'Edit summary')}
								</label>
								<input
									id="suggest-summary"
									type="text"
									value={summary}
									placeholder={t('wiki.edit_summary_placeholder', 'Briefly describe your changes')}
									oninput={(e) => (summary = e.currentTarget.value)}
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
								/>
							</div>

							{#if errorKey}
								<p
									class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500"
									role="alert"
								>
									{t(errorKey, t('wiki.suggest_failed', 'Failed to submit suggestion. Please try again.'))}
								</p>
							{/if}

							<div class="flex justify-end gap-2 pt-1">
								<button
									type="button"
									onclick={close}
									class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
								>
									{t('wiki.cancel', 'Cancel')}
								</button>
								<button
									type="submit"
									disabled={submitting}
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
								>
									{#if submitting}
										<Loader2 size={14} class="animate-spin" />
									{:else}
										<Send class="rtl:-scale-x-100" size={14} />
									{/if}
									{t('wiki.suggest_submit', 'Submit suggestion')}
								</button>
							</div>
						</form>
					{/if}
				</div>
		{/snippet}
	</Modal>
{/if}
