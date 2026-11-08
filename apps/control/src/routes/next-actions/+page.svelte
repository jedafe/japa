<!-- §122 Next-Action Suggestions — rule management -->
<script lang="ts">
	import PageHeader from '#lib/components/PageHeader.svelte';
	import DataTable from '#lib/components/DataTable.svelte';
	import AlertBanner from '#lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '#lib/components/ui/ConfirmModal.svelte';
	import SearchInput from '#lib/components/ui/SearchInput.svelte';
	import { t, translations } from '#lib/stores/i18n.svelte.js';
	import { escapeHtml } from '#lib/utils.js';
	import { Plus, RefreshCw } from 'lucide-svelte';
	import { listRules, getStats, updateRule, deleteRule, type NextActionRule, type NextActionStat } from '#lib/api/nextactions.js';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '#lib/api/types.js';
	import { goto } from '$app/navigation';

	let rules = $state<NextActionRule[]>([]);
	let stats = $state<NextActionStat[]>([]);
	let loading = $state(true);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');
	let selectedIds = $state(new Set<string>());

	// Server-side list state — the endpoint paginates/searches/sorts, so the
	// DataTable runs in remote mode (rows render as-is, footer shows server
	// total). searchApplied snapshots `search` after the debounce so the
	// page/sort effect never tracks raw keystrokes.
	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let totalRows = $state(0);
	let search = $state('');
	let searchApplied = $state('');

	// Single-state confirm modal: destructive bulk ops and deletes route here
	// (ConfirmModal is the control-app convention — never native confirm()).
	let confirmAction = $state<
		| { kind: 'delete'; rule: NextActionRule }
		| { kind: 'bulk_delete' | 'bulk_enable' | 'bulk_disable'; ids: string[] }
		| null
	>(null);
	let confirmOpen = $state(false);

	function openConfirm(action: NonNullable<typeof confirmAction>) {
		confirmAction = action;
		confirmOpen = true;
	}

	const columns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
			{ key: 'slug', label: t('nextactions.col_slug', 'Slug'), sortable: true,
			  render: (v: unknown) => `<span class="font-mono text-xs">${escapeHtml(v)}</span>` },
			{ key: 'name', label: t('nextactions.col_name', 'Name'), sortable: true,
			  render: (v: unknown) => escapeHtml(v) },
			{ key: 'enabled', label: t('nextactions.col_enabled', 'Enabled'), sortable: true,
			  render: (v: unknown) => v
				? `<span class="text-emerald-500">✓</span>`
				: `<span class="text-muted-foreground">—</span>` },
			{ key: 'upsell', label: t('nextactions.col_upsell', 'Upsell'), sortable: true,
			  render: (v: unknown) => v
				? `<span class="text-amber-500">✓</span>`
				: `<span class="text-muted-foreground">—</span>` },
			{ key: 'priority', label: t('nextactions.col_priority', 'Priority'), sortable: true },
			{ key: 'channels', label: t('nextactions.col_channels', 'Channels'),
			  render: (v: unknown) => `<span class="text-muted-foreground text-xs">${escapeHtml(Array.isArray(v) ? v.join(', ') : '')}</span>` }
		];
	});

	function fail(msg: string | undefined, fallback: string) {
		message = msg || fallback;
		messageType = 'error';
	}

	// PATCH requires the whole rule body (ruleRequest has no pointer fields),
	// so a toggle resubmits the row with only `enabled` flipped.
	async function setEnabled(row: NextActionRule, enabled: boolean) {
		message = '';
		const r = await updateRule(row.id, { ...row, enabled });
		if (r.ok) await load(searchApplied);
		else fail(r.error, t('nextactions.toggle_failed', 'Failed to update rule'));
	}

	async function applyBulk(action: 'bulk_delete' | 'bulk_enable' | 'bulk_disable', ids: string[]) {
		let failed: number;
		if (action === 'bulk_delete') {
			const results = await Promise.all(ids.map((id) => deleteRule(id)));
			failed = results.filter((r) => !r.ok).length;
		} else {
			const enabled = action === 'bulk_enable';
			const results = await Promise.all(
				ids.map((id) => {
					const rule = rules.find((r) => r.id === id);
					return rule ? updateRule(id, { ...rule, enabled }) : Promise.resolve({ ok: false as const, error: 'not found' });
				})
			);
			failed = results.filter((r) => !r.ok).length;
		}
		await load(searchApplied);
		if (failed > 0) {
			fail('', t('nextactions.bulk_failed', '{n} rule(s) could not be updated.').replace('{n}', String(failed)));
		} else {
			message = t('nextactions.bulk_ok', '{n} rule(s) updated.').replace('{n}', String(ids.length));
			messageType = 'success';
		}
	}

	async function confirmCurrent() {
		const action = confirmAction;
		confirmAction = null;
		if (!action) return;
		message = '';
		if (action.kind === 'delete') {
			const r = await deleteRule(action.rule.id);
			if (r.ok) {
				await load(searchApplied);
				message = t('nextactions.deleted', 'Rule deleted');
				messageType = 'success';
			} else {
				fail(r.error, t('nextactions.delete_failed', 'Delete failed'));
			}
		} else {
			await applyBulk(action.kind, action.ids);
		}
	}

	const rowActions = $derived.by<DataTableRowAction[]>(() => {
		translations();
		return [
			{
				label: t('common.edit', 'Edit'),
				onClick: (row) => goto(`/next-actions/${row.id}`)
			},
			{
				label: t('nextactions.enable', 'Enable'),
				onClick: (row) => void setEnabled(row as unknown as NextActionRule, true),
				hidden: (row) => row.enabled === true
			},
			{
				label: t('nextactions.disable', 'Disable'),
				onClick: (row) => void setEnabled(row as unknown as NextActionRule, false),
				hidden: (row) => row.enabled !== true
			},
			{
				label: t('common.delete', 'Delete'),
				onClick: (row) => openConfirm({ kind: 'delete', rule: row as unknown as NextActionRule }),
				variant: 'destructive'
			}
		];
	});

	const bulkActions = $derived.by<DataTableBulkAction[]>(() => {
		translations();
		return [
			{
				label: t('nextactions.enable_selected', 'Enable selected'),
				onClick: (ids: string[]) => openConfirm({ kind: 'bulk_enable', ids })
			},
			{
				label: t('nextactions.disable_selected', 'Disable selected'),
				onClick: (ids: string[]) => openConfirm({ kind: 'bulk_disable', ids })
			},
			{
				label: t('nextactions.delete_selected', 'Delete selected'),
				variant: 'destructive',
				onClick: (ids: string[]) => openConfirm({ kind: 'bulk_delete', ids })
			}
		];
	});

	async function load(q: string) {
		loading = true;
		try {
			const [r, s] = await Promise.all([
				listRules({ q, sort: sortKey, sort_dir: sortDirection, page_offset: page * pageSize, page_size: pageSize }),
				getStats()
			]);
			if (!r.ok) {
				fail(r.error, t('nextactions.load_failed', 'Failed to load rules'));
			} else if (r.data) {
				rules = r.data.data ?? [];
				totalRows = r.data.total ?? rules.length;
			}
			if (s.ok) stats = s.data ?? [];
		} catch {
			fail('', t('nextactions.load_failed', 'Failed to load rules'));
		} finally {
			loading = false;
		}
	}

	// Refetch on page/page-size/sort/searchApplied changes — the endpoint owns
	// those semantics in remote mode (audit page convention).
	$effect(() => {
		void page;
		void pageSize;
		void sortKey;
		void sortDirection;
		void searchApplied;
		void load(searchApplied);
	});

	// Debounced search-as-you-type: snapshot `search` into `searchApplied`
	// (which the load effect tracks), resetting to page 0 on new queries.
	let searchSeq = 0;
	$effect(() => {
		const q = search;
		const seq = ++searchSeq;
		const t = setTimeout(() => {
			if (seq !== searchSeq) return;
			page = 0;
			searchApplied = q;
		}, 250);
		return () => clearTimeout(t);
	});

	const statColumns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
			{ key: 'slug', label: t('nextactions.col_rule', 'Rule'), sortable: true,
			  render: (v: unknown) => `<span class="font-mono text-xs">${escapeHtml(v)}</span>` },
			{ key: 'shown', label: t('nextactions.col_served', 'Served'), sortable: true },
			{ key: 'clicked', label: t('nextactions.col_clicked', 'Clicked'), sortable: true },
			{ key: 'dismissed', label: t('nextactions.col_dismissed', 'Dismissed'), sortable: true },
			{ key: 'completed', label: t('nextactions.col_completed', 'Completed'), sortable: true }
		];
	});

	const confirmCopy = $derived.by(() => {
		const a = confirmAction;
		if (!a) return { title: '', consequence: '' };
		if (a.kind === 'delete') {
			return {
				title: t('nextactions.delete_title', 'Delete rule'),
				consequence: t('nextactions.delete_confirm', 'Permanently delete rule "{slug}"? Served/dismissed state for users is kept but the suggestion stops rendering.').replace('{slug}', a.rule.slug)
			};
		}
		const n = String(a.ids.length);
		if (a.kind === 'bulk_enable') {
			return { title: t('nextactions.enable_selected', 'Enable selected'), consequence: t('nextactions.bulk_enable_confirm', 'Enable {n} selected rule(s)?').replace('{n}', n) };
		}
		if (a.kind === 'bulk_disable') {
			return { title: t('nextactions.disable_selected', 'Disable selected'), consequence: t('nextactions.bulk_disable_confirm', 'Disable {n} selected rule(s)? They stop rendering for all users.').replace('{n}', n) };
		}
		return { title: t('nextactions.delete_selected', 'Delete selected'), consequence: t('nextactions.bulk_delete_confirm', 'Permanently delete {n} selected rule(s)? This cannot be undone.').replace('{n}', n) };
	});
</script>

<PageHeader title={t('nextactions.title', 'Next-Action Suggestions')} description={t('nextactions.description', 'Manage contextual next-step suggestion rules (§122)')} />

<div class="mb-3 flex items-center gap-2">
	<button onclick={() => goto('/next-actions/new')} class="bg-primary text-primary-foreground inline-flex items-center gap-1 rounded-[var(--radius)] px-3 py-1.5 text-sm">
		<Plus size={14} /> {t('nextactions.new_rule', 'New rule')}
	</button>
</div>

{#if message}
	<AlertBanner variant={messageType} {message} />
{/if}

<div class="mb-3 flex items-center gap-2">
	<SearchInput bind:value={search} placeholder={t('nextactions.filter_ph', 'Filter rules…')} class="w-64" />
	<button onclick={() => load(searchApplied)} class="text-muted-foreground hover:text-foreground p-1" aria-label={t('common.refresh', 'Refresh')}><RefreshCw size={14} /></button>
</div>

<DataTable {columns} rows={rules} {totalRows} {loading} {rowActions} {bulkActions} bind:selectedIds bind:page bind:pageSize bind:sortKey bind:sortDirection />

{#if confirmAction}
	<ConfirmModal
		bind:open={confirmOpen}
		title={confirmCopy.title}
		consequence={confirmCopy.consequence}
		onConfirm={confirmCurrent}
		onCancel={() => (confirmAction = null)}
	/>
{/if}

{#if stats.length > 0}
	<h2 class="text-foreground mt-8 mb-2 text-sm font-medium">{t('nextactions.funnel_title', 'Funnel (lifetime)')}</h2>
	<p class="text-muted-foreground mb-2 text-xs">{t('nextactions.funnel_caption', 'Served = suggestions returned by the API (a suggestion may be served without rendering); Completed = real completions via completion_signal or an explicit complete call, not CTA clicks.')}</p>
	<DataTable columns={statColumns} rows={stats} totalRows={stats.length} selectable={false} pageSizeOptions={[10, 25, 50]} getRowId={(row) => (row as NextActionStat).slug} />
{/if}
