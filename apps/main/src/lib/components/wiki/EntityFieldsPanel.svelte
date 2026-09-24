<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Community Field Ops)
	// Read-mostly "Data" panel: renders an entity's current structured field
	// values grouped by field-group name, a pending-suggestions banner, and a
	// per-field edit affordance that opens the shared SuggestEdit dialog.
	import { page } from '$app/state';
	import { Clock, Pencil, Table2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { getEntityFields, type WikiEntityField, type WikiEntityFields } from '$lib/api/wiki';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import SuggestEdit from './SuggestEdit.svelte';

	let {
		entityType,
		entityId,
		title = undefined
	}: {
		entityType: string;
		entityId: string;
		title?: string;
	} = $props();

	const flagOn = $derived(page.data?.features?.['wiki.structured_edits'] === true);

	let schema = $state<WikiEntityFields | null>(null);
	let loading = $state(false);
	let failed = $state(false);
	let editOpen = $state(false);
	// Bumped on a successful suggestion so the panel refetches (pending_count
	// and auto-approved values change server-side).
	let reloadToken = $state(0);

	$effect(() => {
		if (!flagOn) return;
		// entityType/entityId/reloadToken are tracked deps — refetch on change.
		void entityType;
		void entityId;
		void reloadToken;
		void load();
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
			.map((g) => ({ ...g, fields: g.fields.filter((f) => f.visible) }))
			.filter((g) => g.fields.length > 0)
	);

	function formatValue(f: WikiEntityField): string {
		const v = f.value;
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
			{#if schema}
				<SuggestEdit
					{entityType}
					{entityId}
					fieldsData={schema}
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
							{/each}
						</dl>
					</section>
				{/each}
			</div>
		{/if}
	</section>
{/if}
