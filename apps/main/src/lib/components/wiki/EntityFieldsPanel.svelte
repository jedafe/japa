<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
	// Read-mostly "Data" panel: renders an entity's current structured field
	// values grouped by field-group name, a pending-suggestions banner, and a
	// per-field edit affordance that opens the shared SuggestEdit dialog.
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { Clock, History, Pencil, Table2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import {
		getEntityFields,
		getEntityRevisions,
		type WikiEntityField,
		type WikiEntityFields,
		type WikiEntityRevision
	} from '$lib/api/wiki';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import SuggestEdit from './SuggestEdit.svelte';
	import { formatWikiFieldValue, isMediaFieldType, mediaFieldItems } from '$lib/utils/wikiFieldValue';
	import type { CurrencyInfo } from '$lib/utils/currency';

	let {
		entityType,
		entityId,
		title = undefined,
		groupId = undefined,
		groupSlug = undefined
	}: {
		entityType: string;
		entityId: string;
		title?: string;
		/** §80 Wiki Types: when set (a bound wiki category), the panel renders
		 *  only that group instead of every group assigned to the entity. */
		groupId?: string;
		/** The bound group's slug — forwarded to SuggestEdit so field
		 *  proposals land in the category's group even when it has no
		 *  fields yet (empty groups are absent from the schema). */
		groupSlug?: string;
	} = $props();

	const flagOn = $derived(page.data?.features?.['wiki.structured_edits'] === true);

	let schema = $state<WikiEntityFields | null>(null);
	let loading = $state(false);
	let failed = $state(false);
	let editOpen = $state(false);
	// Bumped on a successful suggestion so the panel refetches (pending_count
	// and auto-approved values change server-side).
	let reloadToken = $state(0);

	// §80 entity edit history — GET /wiki/entities/{type}/{id}/revisions,
	// fetched lazily when the disclosure opens.
	let historyOpen = $state(false);
	let history = $state<WikiEntityRevision[]>([]);
	let historyLoading = $state(false);
	let historyFailed = $state(false);
	let historyLoaded = $state(false);

	async function loadHistory() {
		historyLoading = true;
		historyFailed = false;
		try {
			const res = await getEntityRevisions(entityType, entityId, { limit: 20 });
			if (res === null) {
				historyFailed = true;
			} else {
				history = res.items;
				historyLoaded = true;
			}
		} catch {
			historyFailed = true;
		} finally {
			historyLoading = false;
		}
	}

	async function toggleHistory() {
		historyOpen = !historyOpen;
		if (historyOpen && !historyLoaded && !historyLoading) {
			await loadHistory();
		}
	}

	$effect(() => {
		if (!flagOn) return;
		// entityType/entityId/reloadToken are tracked deps — refetch on change.
		void entityType;
		void entityId;
		void reloadToken;
		void load();
		// Refresh an already-open history after a new suggestion lands.
		// historyOpen is untracked so toggling the disclosure doesn't re-run
		// the whole effect.
		if (untrack(() => historyOpen)) void loadHistory();
	});

	async function load() {
		loading = true;
		failed = false;
		const data = await getEntityFields(entityType, entityId);
		loading = false;
		if (data) {
			schema = data;
		} else {
			failed = true;
		}
	}

	const visibleGroups = $derived(
		(schema?.groups ?? [])
			.filter((g) => !groupId || g.id === groupId)
			.map((g) => ({ ...g, fields: g.fields.filter((f) => f.visible) }))
			.filter((g) => g.fields.length > 0)
	);

	// SuggestEdit receives the group-scoped schema too — on a bound wiki
	// page the dialog must not offer fields from the entity's other groups
	// (e.g. Healthcare fields on a Cost of Living page). Unbound pages keep
	// the full entity schema.
	const scopedSchema = $derived<WikiEntityFields | null>(
		schema && groupId ? { ...schema, groups: schema.groups.filter((g) => g.id === groupId) } : schema
	);

	// §58 — active currency list from the root layout for symbol resolution
	// on currency/price_range field values; the entity's resolved country
	// currency is the display fallback when a field doesn't declare
	// {currency_code} in options.
	const currencies = $derived(page.data.currencies as CurrencyInfo[] | undefined);
	const contextCurrency = $derived(schema?.entity_currency_code ?? '');

	function formatValue(f: WikiEntityField): string {
		return formatWikiFieldValue(f, currencies, contextCurrency);
	}
</script>

{#if flagOn}
	<section
		class="mt-8 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
	>
		<div class="mb-3 flex items-center justify-between gap-2">
			<h2 class="flex items-center gap-1.5 text-sm font-semibold text-[var(--color-text)]">
				<Table2 size={14} class="text-[var(--color-muted)]" />
				{title ?? t('wiki.data_panel_title', 'Details')}
			</h2>
			{#if scopedSchema}
				<SuggestEdit
					{entityType}
					{entityId}
					fieldsData={scopedSchema}
					proposeGroupSlug={groupSlug ??
						(scopedSchema.groups.length === 1 ? scopedSchema.groups[0].slug : undefined)}
					bind:open={editOpen}
					onsubmitted={() => (reloadToken += 1)}
				/>
			{/if}
		</div>

		{#if schema && schema.pending_count > 0}
			<p
				class="mb-3 flex items-center gap-1.5 rounded-[var(--radius)] bg-amber-500/10 px-3 py-2 text-xs font-medium text-amber-600"
			>
				<Clock size={12} />
				{t('wiki.pending_banner', '{count} suggested edit(s) awaiting review.', {
					count: schema.pending_count
				})}
			</p>
		{/if}

		{#if loading && !schema}
			<LoadingSkeleton variant="lines" count={4} />
		{:else if failed}
			<div class="flex items-center gap-3 text-sm text-[var(--color-muted)]">
				<span>{t('wiki.load_error', 'Wiki could not be loaded.')}</span>
				<button
					type="button"
					onclick={() => void load()}
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.retry', 'Try again')}
				</button>
			</div>
		{:else if schema && visibleGroups.length === 0}
			<p class="text-sm text-[var(--color-muted)]">
				{t('wiki.fields_empty', 'No editable fields for this entry.')}
			</p>
		{:else if schema}
			<div class="space-y-4">
				{#each visibleGroups as group (group.id)}
					<section>
						<h3
							class="mb-1.5 border-b border-[var(--color-border)] pb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]"
						>
							{group.name}
						</h3>
						<dl class="divide-y divide-[var(--color-border)]">
							{#each group.fields as f (f.id)}
								{#if isMediaFieldType(f.field_type)}
									<!-- §80 attractions: image fields render as a thumbnail
									     grid (the attraction gallery), not text. -->
									{@const media = mediaFieldItems(f.value)}
									{#if media.length > 0}
										<div class="py-1.5">
											<div class="flex items-baseline justify-between gap-3 text-sm">
												<dt class="text-[var(--color-muted)]">{f.label}</dt>
												{#if f.can_edit}
													<button
														type="button"
														onclick={() => (editOpen = true)}
														class="shrink-0 text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
														aria-label={t('wiki.suggest_edit', 'Suggest an edit')}
														title={t('wiki.suggest_edit', 'Suggest an edit')}
													>
														<Pencil size={12} />
													</button>
												{/if}
											</div>
											<dd class="mt-1.5 grid grid-cols-2 gap-2 sm:grid-cols-3">
												{#each media as m (m.id || m.url)}
													<!-- §80 — v1 thumbnails link the full media URL. -->
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
															class="aspect-[4/3] w-full rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
														/>
													</a>
												{/each}
											</dd>
										</div>
									{/if}
								{:else}
								<div class="flex items-baseline justify-between gap-3 py-1.5 text-sm">
									<dt class="shrink-0 text-[var(--color-muted)]">{f.label}</dt>
									<dd class="flex min-w-0 items-baseline gap-2 text-right">
										<span class="truncate text-[var(--color-text)]">{formatValue(f)}</span>
										{#if f.can_edit}
											<button
												type="button"
												onclick={() => (editOpen = true)}
												class="shrink-0 text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
												aria-label={t('wiki.suggest_edit', 'Suggest an edit')}
												title={t('wiki.suggest_edit', 'Suggest an edit')}
											>
												<Pencil size={12} />
											</button>
										{/if}
									</dd>
								</div>
								{/if}
							{/each}
						</dl>
					</section>
				{/each}
			</div>
		{/if}

		{#if schema}
			<div class="mt-4 border-t border-[var(--color-border)] pt-3">
				<button
					type="button"
					onclick={toggleHistory}
					class="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
				>
					<History size={12} />
					{t('wiki.entity_history', 'Edit history')}
				</button>
				{#if historyOpen}
					{#if historyLoading}
						<p class="mt-2 text-xs text-[var(--color-muted)]">
							{t('common.loading', 'Loading')}…
						</p>
					{:else if historyFailed}
						<p class="mt-2 text-xs text-[var(--color-muted)]">
							{t('wiki.entity_history_error', 'Failed to load edit history.')}
						</p>
					{:else if history.length === 0}
						<p class="mt-2 text-xs text-[var(--color-muted)]">
							{t('wiki.entity_history_empty', 'No suggested edits yet.')}
						</p>
					{:else}
						<ul class="mt-2 space-y-2">
							{#each history as rev (rev.id)}
								<li class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-xs">
									<div class="flex items-center gap-2">
										<span
											class="rounded-full px-2 py-0.5 font-medium {rev.status === 'approved'
												? 'bg-green-500/10 text-green-600'
												: rev.status === 'rejected'
													? 'bg-red-500/10 text-red-600'
													: 'bg-amber-500/10 text-amber-600'}"
										>
											{rev.status === 'approved'
												? t('wiki.revision_approved', 'Approved')
												: rev.status === 'rejected'
													? t('wiki.revision_rejected', 'Rejected')
													: t('wiki.revision_pending', 'Pending')}
										</span>
										<span class="ml-auto text-[var(--color-muted)]">
											{new Date(rev.created_at).toLocaleDateString()}
										</span>
									</div>
									{#if rev.summary}
										<p class="mt-1 text-[var(--color-text)]">{rev.summary}</p>
									{:else if rev.status === 'pending'}
										<p class="mt-1 text-[var(--color-muted)]">
											{t(
												'wiki.entity_history_review_pending',
												'Pending review — contents hidden until approved.'
											)}
										</p>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}
				{/if}
			</div>
		{/if}
	</section>
{/if}
