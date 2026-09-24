<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
	// "Suggest an edit" — opens a dialog with the entity's editable field
	// schema (GET /wiki/entities/{type}/{id}/fields) and submits changed
	// fields as set_value ops to POST /wiki/suggest. Renders nothing unless
	// the wiki.structured_edits feature flag is on.
	import { page } from '$app/state';
	import { Dialog } from 'bits-ui';
	import { Pencil, X, Send, Loader2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import CustomFieldGroup from '$lib/components/create/panel/CustomFieldGroup.svelte';
	import {
		getEntityFields,
		suggestFields,
		type SuggestFieldsResult,
		type WikiEntityField,
		type WikiEntityFieldGroup,
		type WikiEntityFields,
		type WikiFieldOp
	} from '$lib/api/wiki';
	import type { CustomFieldDef } from '$lib/types';

	let {
		entityType,
		entityId,
		label = undefined,
		fieldsData = undefined,
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
			if (schema) seedValues(schema);
			else if (fieldsData === undefined && !loadingFields) void loadSchema();
		}
		wasOpen = open;
	});

	function seedValues(d: WikiEntityFields | null) {
		const v: Record<string, unknown> = {};
		if (d) {
			for (const g of d.groups) {
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
		(schema?.groups ?? []).some((g) => g.fields.some((f) => f.visible))
	);

	function formatValue(v: unknown): string {
		if (v === null || v === undefined || v === '') return '—';
		if (typeof v === 'boolean') return v ? t('common.yes', 'Yes') : t('common.no', 'No');
		if (typeof v === 'number') return String(v);
		if (typeof v === 'string') return v;
		if (Array.isArray(v)) {
			return v
				.map((i) => (typeof i === 'object' && i !== null ? JSON.stringify(i) : String(i)))
				.join(', ');
		}
		return JSON.stringify(v);
	}

	function changedOps(): WikiFieldOp[] {
		const ops: WikiFieldOp[] = [];
		if (!schema) return ops;
		for (const g of schema.groups) {
			for (const f of g.fields) {
				if (!f.visible || !f.can_edit) continue;
				const cur = values[f.key] ?? null;
				const init = initialValues[f.key] ?? null;
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
		const ops = changedOps();
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

	<Dialog.Root bind:open>
		<Dialog.Portal>
			<Dialog.Overlay
				class="fixed inset-0 z-40 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out"
			/>
			<Dialog.Content
				class="fixed left-1/2 top-1/2 z-50 flex max-h-[85vh] w-full max-w-[560px] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-[12px] border border-[var(--color-border)] bg-[var(--color-bg)] shadow-xl"
			>
				<div class="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-4">
					<Dialog.Title class="text-base font-semibold text-[var(--color-text)]">
						{label ?? t('wiki.suggest_edit', 'Suggest an edit')}
					</Dialog.Title>
					<Dialog.Close class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
						<X size={18} />
					</Dialog.Close>
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
					{:else if schema && !hasVisibleFields}
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
							{#each schema.groups as group (group.id)}
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
											/>
										{/if}
										{#if readonly.length > 0}
											<dl class="mt-3 space-y-1.5">
												{#each readonly as f (f.id)}
													<div class="flex items-baseline justify-between gap-3 text-sm">
														<dt class="text-[var(--color-muted)]">{f.label}</dt>
														<dd class="text-right text-[var(--color-text)]">
															{formatValue(f.value)}
														</dd>
													</div>
												{/each}
											</dl>
										{/if}
									</section>
								{/if}
							{/each}

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
								<Dialog.Close
									class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
								>
									{t('wiki.cancel', 'Cancel')}
								</Dialog.Close>
								<button
									type="submit"
									disabled={submitting}
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
								>
									{#if submitting}
										<Loader2 size={14} class="animate-spin" />
									{:else}
										<Send size={14} />
									{/if}
									{t('wiki.suggest_submit', 'Submit suggestion')}
								</button>
							</div>
						</form>
					{/if}
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
{/if}
