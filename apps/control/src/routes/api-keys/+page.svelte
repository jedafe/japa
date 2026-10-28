<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Key, Plus, Copy, Check } from 'lucide-svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import Tabs from '$lib/components/ui/Tabs.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	interface AdminApiKey {
		id: string;
		name: string;
		key_prefix: string;
		user_id: string;
		rate_limit_tier: string;
		status: string;
		request_count: number;
		scopes: string[];
		created_at: string;
		last_used_at?: string;
		expires_at?: string;
		allowed_ips?: string[];
	}

	let {} = $props();

	let keys = $state<AdminApiKey[]>([]);
	let loading = $state(true);
	let filter = $state('');
	let filterTier = $state('all');
	let activeTab = $state('active');
	let errorMessage = $state('');
	let analytics = $state<any>(null);
	let analyticsLoading = $state(false);

	// L3: DataTable columns for the top-keys-by-volume analytics list
	const topKeyColumns: DataTableColumn[] = [
		{ key: 'key_prefix', label: t('api_keys.key_prefix', 'Prefix'), sortable: true, render: (val) => `<span class="font-mono text-xs text-muted-foreground">${escapeHtml(val)}</span>` },
		{ key: 'name', label: t('api_keys.name', 'Name'), sortable: true },
		{ key: 'request_count', label: t('api_keys.requests', 'Requests'), sortable: true, render: (val) => `<span class="font-mono text-muted-foreground">${Number(val).toLocaleString()}</span>` }
	];

	$effect(() => {
		load();
		loadScopes();
	});

	async function load() {
		loading = true;
		errorMessage = '';
		const result = await api<AdminApiKey[]>('/control/api-keys');
		if (result.ok) {
			keys = Array.isArray(result.data) ? result.data : [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	async function loadAnalytics() {
		analyticsLoading = true;
		const result = await api<any>('/control/api-keys/analytics');
		if (result.ok) {
			analytics = result.data;
		} else {
			errorMessage = result.error;
		}
		analyticsLoading = false;
	}

	let confirmOpen = $state(false);
	let confirmAction = $state<{ type: 'revoke' | 'suspend' | 'unsuspend' | 'unrevoke' | 'delete'; key: AdminApiKey } | null>(null);

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');
	let selectedIds = $state(new Set<string>());

	let bulkConfirmOpen = $state(false);
	let pendingBulk = $state<{ action: 'suspend' | 'revoke' | 'delete'; ids: string[] } | null>(null);

	let createModalOpen = $state(false);
	let availableScopes = $state<{ value: string; label: string; description: string }[]>([]);

	async function loadScopes() {
		const result = await api<{ value: string; label: string; description: string }[]>('/control/api-scopes');
		if (result.ok && Array.isArray(result.data)) {
			availableScopes = result.data;
		}
	}

	let createForm = $state({ name: '', scopes: [] as string[], rate_limit_tier: 'standard', expires_at: '' });
	let createLoading = $state(false);

	let oneTimeKeyModalOpen = $state(false);
	let oneTimeKeyValue = $state('');
	let copiedToClipboard = $state(false);

	let editModalOpen = $state(false);
	let editForm = $state({ id: '', name: '', scopes: [] as string[], rate_limit_tier: 'basic' });
	let editLoading = $state(false);

	const tierItems = $derived([
		{ value: 'all', label: t('api_keys.tier_all') },
		{ value: 'basic', label: t('api_keys.tier_basic') },
		{ value: 'standard', label: t('api_keys.tier_standard') },
		{ value: 'premium', label: t('api_keys.tier_premium') },
		{ value: 'unlimited', label: t('api_keys.tier_unlimited') }
	]);
	const tierOptions = $derived(tierItems.filter((i) => i.value !== 'all'));

	let filtered = $derived(
		keys
			.filter((k) => {
				if (activeTab === 'active' && k.status !== 'active') return false;
				if (filterTier !== 'all' && k.rate_limit_tier !== filterTier) return false;
				if (filter) {
					const q = filter.toLowerCase();
					if (
						!k.name?.toLowerCase().includes(q) &&
						!k.key_prefix?.toLowerCase().includes(q) &&
						!k.user_id?.toLowerCase().includes(q)
					)
						return false;
				}
				return true;
			})
	);

	let totalRows = $derived(filtered.length);

	// DataTable owns sorting + pagination (it receives the full filtered
	// list below) — no parent-side sort/slice, which would sort only the
	// visible page and misorder rows across pages.

	let rows = $derived(
		filtered.map((k) => ({
			...k,
			id: k.id
		}))
	);

	let activeCount = $derived(keys.filter((k) => k.status === 'active').length);

	function statusColor(status: string): string {
		switch (status) {
			case 'active': return 'text-emerald-500';
			case 'suspended': return 'text-amber-500';
			case 'revoked': return 'text-destructive';
			default: return 'text-muted-foreground';
		}
	}

	function statusBg(status: string): string {
		switch (status) {
			case 'active': return 'bg-emerald-500/10';
			case 'suspended': return 'bg-amber-500/10';
			case 'revoked': return 'bg-destructive/10';
			default: return 'bg-muted';
		}
	}

	function formatNumber(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
		return n.toString();
	}

	const columns: DataTableColumn[] = [
		{
			key: 'name',
			label: t('api_keys.name'),
			sortable: true,
			render: (_val: unknown, row: Record<string, unknown>) =>
				`<span class="font-medium">${escapeHtml(row.name)}</span>`
		},
		{
			key: 'key_prefix',
			label: t('api_keys.prefix'),
			render: (val: unknown) =>
				`<span class="font-mono text-xs text-muted-foreground">${escapeHtml(val)}...</span>`
		},
		{
			key: 'user_id',
			label: t('api_keys.user'),
			render: (val: unknown) =>
				val
					? `<span class="text-muted-foreground text-xs">${escapeHtml(String(val).slice(0, 8))}...</span>`
					: `<span class="text-muted-foreground text-xs">${t('api_keys.system')}</span>`
		},
		{
			key: 'rate_limit_tier',
			label: t('api_keys.tier'),
			sortable: true,
			render: (val: unknown) =>
				`<span class="bg-accent text-accent-foreground rounded-5px px-1.5 py-0.5 text-xs">${escapeHtml(val)}</span>`
		},
		{
			key: 'status',
			label: t('api_keys.status'),
			sortable: true,
			render: (val: unknown, row: Record<string, unknown>) => {
				const status = escapeHtml(val);
				return `<span class="${statusBg(status)} ${statusColor(status)} rounded-full px-2 py-0.5 text-xs font-medium">${status}</span>`;
			}
		},
		{
			key: 'request_count',
			label: t('api_keys.requests'),
			sortable: true,
			class: 'text-end',
			render: (val: unknown) =>
				`<span class="font-mono text-xs">${escapeHtml(formatNumber(Number(val)))}</span>`
		}
	];

	function toKey(row: Record<string, unknown>): AdminApiKey {
		return row as unknown as AdminApiKey;
	}

	const rowActions: DataTableRowAction[] = [
		{
			label: t('api_keys.edit_key'),
			onClick: (row) => openEditModal(toKey(row))
		},
		{
			label: t('api_keys.suspend_key'),
			onClick: (row) => openConfirm('suspend', toKey(row)),
			hidden: (row) => row.status !== 'active'
		},
		{
			label: t('api_keys.unsuspend_key'),
			onClick: (row) => openConfirm('unsuspend', toKey(row)),
			hidden: (row) => row.status !== 'suspended'
		},
		{
			label: t('api_keys.revoke_key'),
			onClick: (row) => openConfirm('revoke', toKey(row)),
			variant: 'destructive',
			hidden: (row) => row.status !== 'active'
		},
		{
			label: t('api_keys.unrevoke_key'),
			onClick: (row) => openConfirm('unrevoke', toKey(row)),
			hidden: (row) => row.status !== 'revoked'
		},
		{
			label: t('api_keys.delete_key'),
			onClick: (row) => openConfirm('delete', toKey(row)),
			variant: 'destructive'
		}
	];

	const bulkActions: DataTableBulkAction[] = [
		{
			label: t('api_keys.suspend_selected'),
			onClick: (ids: string[]) => {
				pendingBulk = { action: 'suspend', ids };
				bulkConfirmOpen = true;
			}
		},
		{
			label: t('api_keys.revoke_selected'),
			variant: 'destructive',
			onClick: (ids: string[]) => {
				pendingBulk = { action: 'revoke', ids };
				bulkConfirmOpen = true;
			}
		},
		{
			label: t('api_keys.delete_selected'),
			variant: 'destructive',
			onClick: (ids: string[]) => {
				pendingBulk = { action: 'delete', ids };
				bulkConfirmOpen = true;
			}
		}
	];

	async function confirmBulk() {
		if (!pendingBulk) return;
		const { action, ids } = pendingBulk;
		try {
			if (action === 'suspend') {
				const results = await Promise.allSettled(ids.map((id) => suspendKey(id)));
				const failed = results.filter((r) => r.status === 'rejected').length;
				if (failed > 0) errorMessage = t('api_keys.bulk_suspend_partial', '{n} key(s) could not be suspended.').replace('{n}', String(failed));
			} else if (action === 'revoke') {
				const results = await Promise.allSettled(ids.map((id) => revokeKey(id)));
				const failed = results.filter((r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value)).length;
				if (failed > 0) errorMessage = t('api_keys.bulk_revoke_partial', '{n} key(s) could not be revoked.').replace('{n}', String(failed));
			} else {
				await deleteKeysBatch(ids);
			}
		} finally {
			pendingBulk = null;
		}
	}

	async function suspendKey(id: string): Promise<boolean> {
		const result = await api(`/control/api-keys/${id}/suspend`, { method: 'POST' });
		if (result.ok) {
			keys = keys.map((k) => (k.id === id ? { ...k, status: 'suspended' } : k));
			return true;
		} else {
			errorMessage = result.error || t('api_keys.suspend_error');
			return false;
		}
	}

	async function unsuspendKey(id: string) {
		const result = await api(`/control/api-keys/${id}/suspend`, { method: 'DELETE' });
		if (result.ok) {
			keys = keys.map((k) => (k.id === id ? { ...k, status: 'active' } : k));
		} else {
			errorMessage = result.error || t('api_keys.unsuspend_error');
		}
	}

	async function unrevokeKey(id: string) {
		const result = await api(`/control/api-keys/${id}`, { method: 'PATCH', body: { status: 'active' } });
		if (result.ok) {
			keys = keys.map((k) => (k.id === id ? { ...k, status: 'active' } : k));
		} else {
			errorMessage = result.error || t('api_keys.unrevoke_error');
		}
	}

	async function revokeKey(id: string): Promise<boolean> {
		const result = await api(`/control/api-keys/${id}/revoke`, { method: 'POST' });
		if (result.ok) {
			keys = keys.map((k) => (k.id === id ? { ...k, status: 'revoked' } : k));
			return true;
		} else {
			errorMessage = result.error || t('api_keys.revoke_error');
			return false;
		}
	}

	async function deleteKey(id: string) {
		const result = await api(`/control/api-keys/${id}`, { method: 'DELETE' });
		if (result.ok) {
			keys = keys.filter((k) => k.id !== id);
		} else {
			errorMessage = result.error || t('api_keys.delete_error');
		}
	}

	async function deleteKeysBatch(ids: string[]) {
		const result = await api<{ deleted: number }>('/control/api-keys/batch-delete', {
			method: 'POST',
			body: { ids }
		});
		if (result.ok) {
			keys = keys.filter((k) => !ids.includes(k.id));
			selectedIds = new Set();
		} else {
			errorMessage = result.error || t('api_keys.delete_error');
		}
	}

	async function createKey() {
		if (!createForm.name.trim()) {
			errorMessage = t('api_keys.name_required');
			return;
		}

		createLoading = true;
		errorMessage = '';

		const body: Record<string, unknown> = { name: createForm.name.trim() };
		if (createForm.scopes.length > 0) {
			body.scopes = [...createForm.scopes];
		}
		if (createForm.rate_limit_tier && createForm.rate_limit_tier !== 'standard') {
			body.rate_limit_tier = createForm.rate_limit_tier;
		}
		if (createForm.expires_at) {
			body.expires_at = new Date(createForm.expires_at).toISOString();
		}

		const result = await api<{ id: string; key: string } & AdminApiKey>('/control/api-keys', {
			method: 'POST',
			body
		});

		createLoading = false;

		if (result.ok) {
			const { key, ...keyData } = result.data;
			oneTimeKeyValue = key;
			createModalOpen = false;
			createForm = { name: '', scopes: [], rate_limit_tier: 'standard', expires_at: '' };
			keys = [{ ...keyData }, ...keys];
			oneTimeKeyModalOpen = true;
		} else {
			errorMessage = result.error || t('api_keys.create_error');
		}
	}

	async function updateKey() {
		if (!editForm.name.trim()) {
			errorMessage = t('api_keys.name_required');
			return;
		}

		editLoading = true;
		errorMessage = '';

		const body: Record<string, unknown> = { name: editForm.name.trim() };
		if (editForm.scopes.length > 0) {
			body.scopes = [...editForm.scopes];
		}
		if (editForm.rate_limit_tier) {
			body.rate_limit_tier = editForm.rate_limit_tier;
		}

		const result = await api<{ id: string }>(`/control/api-keys/${editForm.id}`, {
			method: 'PATCH',
			body
		});

		editLoading = false;

		if (result.ok) {
			keys = keys.map((k) =>
				k.id === editForm.id
					? { ...k, name: editForm.name.trim(), scopes: (body.scopes as string[]) || k.scopes, rate_limit_tier: editForm.rate_limit_tier || k.rate_limit_tier }
					: k
			);
			editModalOpen = false;
		} else {
			errorMessage = result.error || t('api_keys.update_error');
		}
	}

	function openEditModal(key: AdminApiKey) {
		editForm = {
			id: key.id,
			name: key.name,
			scopes: [...(key.scopes || [])],
			rate_limit_tier: key.rate_limit_tier || 'basic'
		};
		editModalOpen = true;
	}

	function copyToClipboard() {
		navigator.clipboard.writeText(oneTimeKeyValue);
		copiedToClipboard = true;
		setTimeout(() => (copiedToClipboard = false), 2000);
	}

	function openConfirm(type: 'revoke' | 'suspend' | 'unsuspend' | 'unrevoke' | 'delete', key: AdminApiKey) {
		confirmAction = { type, key };
		confirmOpen = true;
	}

	function handleTabChange(value: string) {
		activeTab = value;
		page = 0;
		selectedIds = new Set();
		if (value === 'analytics' && !analytics) {
			loadAnalytics();
		}
	}

	function handleTierChange(value: string) {
		filterTier = value;
		page = 0;
		selectedIds = new Set();
	}

	// --- Autocomplete fetchers for scopes (multi-select via chips) ---

	// Filter availableScopes by query string, excluding already-selected scopes.
	async function fetchScopes(q: string, selected: string[]): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return availableScopes
			.filter((s) => !selected.includes(s.value))
			.filter((s) => s.label.toLowerCase().includes(query) || s.value.toLowerCase().includes(query))
			.map((s) => ({ id: s.value, label: s.label, sublabel: s.description }));
	}

	async function fetchScopesInitial(selected: string[]): Promise<AutocompleteItem[]> {
		return availableScopes
			.filter((s) => !selected.includes(s.value))
			.map((s) => ({ id: s.value, label: s.label, sublabel: s.description }));
	}

	function getScopeLabel(value: string): string {
		return availableScopes.find((s) => s.value === value)?.label ?? value;
	}
</script>

<PageHeader
	title={t('api_keys.title')}
	description={t('api_keys.description')}
/>

{#if errorMessage}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-xl border px-4 py-3 text-sm">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex items-center gap-3">
	<SearchInput
		bind:value={filter}
		placeholder={t('api_keys.search_placeholder')}
		class="w-72"
	/>

	<AutocompleteSelect
		bind:value={filterTier}
		options={tierItems}
		onValueChange={(v) => handleTierChange(v)}
		placeholder={t('api_keys.tier_all')}
		ariaLabel={t('api_keys.tier')}
		class="w-44"
	/>

	<span class="text-muted-foreground text-xs">
		{filtered.length !== 1 ? t('api_keys.key_count_plural').replace('{count}', String(filtered.length)) : t('api_keys.key_count').replace('{count}', String(filtered.length))}
	</span>
	<div class="flex-1"></div>
	<button
		class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-sm font-medium transition-colors hover:opacity-90"
		onclick={() => (createModalOpen = true)}
	>
		<Plus class="h-4 w-4" />
		{t('api_keys.create')}
	</button>
</div>

<Tabs
	bind:value={activeTab}
	onValueChange={handleTabChange}
	tabs={[
		{ value: 'active', label: `${t('api_keys.tab_active')} (${activeCount})` },
		{ value: 'all', label: `${t('api_keys.tab_all')} (${keys.length})` },
		{ value: 'analytics', label: t('api_keys.analytics_tab') }
	]}
/>

{#if activeTab === 'analytics'}
	<div class="mt-4">
		{#if analyticsLoading}
			<div class="text-muted-foreground py-8 text-center text-sm">{t('api_keys.loading') || 'Loading...'}</div>
		{:else if analytics}
			<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
				<div class="border-border-card bg-card rounded-xl border p-4">
					<div class="text-muted-foreground text-xs">{t('api_keys.total_requests')}</div>
					<div class="text-foreground mt-1 text-2xl font-semibold">{analytics.total_requests?.toLocaleString() || 0}</div>
				</div>
				<div class="border-border-card bg-card rounded-xl border p-4">
					<div class="text-muted-foreground text-xs">{t('api_keys.active_keys')}</div>
					<div class="mt-1 text-2xl font-semibold text-emerald-500">{analytics.active_keys || 0}</div>
				</div>
				<div class="border-border-card bg-card rounded-xl border p-4">
					<div class="text-muted-foreground text-xs">{t('api_keys.suspended_keys')}</div>
					<div class="mt-1 text-2xl font-semibold text-amber-500">{analytics.suspended_keys || 0}</div>
				</div>
				<div class="border-border-card bg-card rounded-xl border p-4">
					<div class="text-muted-foreground text-xs">{t('api_keys.revoked_keys')}</div>
					<div class="mt-1 text-2xl font-semibold text-destructive">{analytics.revoked_keys || 0}</div>
				</div>
			</div>

			{#if analytics.top_keys_by_volume?.length}
				<!-- L3: Converted top-keys-by-volume from raw divs to DataTable -->
				<div class="border-border-card bg-card mt-4 rounded-xl border p-4">
					<h3 class="text-foreground mb-3 text-sm font-medium">{t('api_keys.top_keys')}</h3>
					<DataTable
						columns={topKeyColumns}
						rows={analytics.top_keys_by_volume}
						totalRows={analytics.top_keys_by_volume.length}
						pageSize={10}
						selectable={false}
						emptyMessage={t('api_keys.no_top_keys', 'No top keys')}
					/>
				</div>
			{/if}

			{#if analytics.zero_result_keys?.length}
				<div class="border-border-card bg-card mt-4 rounded-xl border p-4">
					<h3 class="text-foreground mb-3 text-sm font-medium">{t('api_keys.zero_usage')}</h3>
					<div class="space-y-2">
						{#each analytics.zero_result_keys as key}
							<div class="flex items-center justify-between text-sm">
								<div class="flex items-center gap-2">
									<span class="font-mono text-xs text-muted-foreground">{key.key_prefix}</span>
									<span class="text-foreground">{key.name}</span>
								</div>
								<span class="text-muted-foreground text-xs">{t('api_keys.no_requests', 'No requests')}</span>
							</div>
						{/each}
					</div>
				</div>
			{:else if analytics.active_keys > 0}
				<div class="border-border-card bg-card mt-4 rounded-xl border p-4">
					<p class="text-muted-foreground text-sm">{t('api_keys.no_zero_usage')}</p>
				</div>
			{/if}

			{#if analytics.recent_activity?.length}
				<div class="border-border-card bg-card mt-4 rounded-xl border p-4">
					<h3 class="text-foreground mb-3 text-sm font-medium">{t('api_keys.recent_activity')}</h3>
					<div class="space-y-2">
						{#each analytics.recent_activity as activity}
							<div class="flex items-center justify-between text-sm">
								<span class="font-mono text-xs text-muted-foreground">{activity.key_prefix}</span>
								<div class="flex items-center gap-4">
									<span class="font-mono text-muted-foreground">{activity.request_count.toLocaleString()}</span>
									<span class="text-muted-foreground text-xs">{new Date(activity.last_used_at).toLocaleDateString()}</span>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{:else}
				<div class="border-border-card bg-card mt-4 rounded-xl border p-4">
					<p class="text-muted-foreground text-sm">{t('api_keys.no_activity')}</p>
				</div>
			{/if}
		{:else}
			<div class="text-muted-foreground py-8 text-center text-sm">{t('api_keys.empty')}</div>
		{/if}
	</div>
{:else}
<div class="mt-4">
	<DataTable
		{columns}
		{rows}
		{totalRows}
		bind:page
		bind:pageSize
		bind:sortKey
		bind:sortDirection
		bind:selectedIds
		{rowActions}
		{bulkActions}
		{loading}
		emptyMessage={filter || filterTier !== 'all' ? t('api_keys.empty_filter') : t('api_keys.empty')}
		emptyIcon={Key as any}
	/>
</div>
{/if}

{#if confirmAction}
	{#if confirmAction.type === 'revoke'}
		<ConfirmModal
			bind:open={confirmOpen}
			title={t('api_keys.revoke_title')}
			consequence={t('api_keys.revoke_confirm_detail').replace('{name}', confirmAction.key.name).replace('{prefix}', confirmAction.key.key_prefix)}
			confirmText={t('api_keys.revoke_text')}
			onConfirm={async () => { await revokeKey(confirmAction!.key.id); confirmAction = null; }}
			onCancel={() => (confirmAction = null)}
		/>
	{:else if confirmAction.type === 'suspend'}
		<ConfirmModal
			bind:open={confirmOpen}
			title={t('api_keys.suspend_title')}
			consequence={t('api_keys.suspend_confirm_detail').replace('{name}', confirmAction.key.name).replace('{prefix}', confirmAction.key.key_prefix)}
			confirmText={t('api_keys.suspend_text')}
			onConfirm={async () => { await suspendKey(confirmAction!.key.id); confirmAction = null; }}
			onCancel={() => (confirmAction = null)}
		/>
	{:else if confirmAction.type === 'unsuspend'}
		<ConfirmModal
			bind:open={confirmOpen}
			title={t('api_keys.unsuspend_title')}
			consequence={t('api_keys.unsuspend_confirm_detail').replace('{name}', confirmAction.key.name).replace('{prefix}', confirmAction.key.key_prefix)}
			confirmText={t('api_keys.unsuspend_text')}
			onConfirm={async () => { await unsuspendKey(confirmAction!.key.id); confirmAction = null; }}
			onCancel={() => (confirmAction = null)}
		/>
	{:else if confirmAction.type === 'unrevoke'}
		<ConfirmModal
			bind:open={confirmOpen}
			title={t('api_keys.unrevoke_title')}
			consequence={t('api_keys.unrevoke_confirm_detail').replace('{name}', confirmAction.key.name).replace('{prefix}', confirmAction.key.key_prefix)}
			confirmText={t('api_keys.unrevoke_text')}
			onConfirm={async () => { await unrevokeKey(confirmAction!.key.id); confirmAction = null; }}
			onCancel={() => (confirmAction = null)}
		/>
	{:else}
		<ConfirmModal
			bind:open={confirmOpen}
			title={t('api_keys.delete_title')}
			consequence={t('api_keys.delete_confirm_detail').replace('{name}', confirmAction.key.name).replace('{prefix}', confirmAction.key.key_prefix)}
			confirmText={t('api_keys.delete_text')}
			onConfirm={async () => { await deleteKey(confirmAction!.key.id); confirmAction = null; }}
			onCancel={() => (confirmAction = null)}
		/>
	{/if}
{/if}

{#if pendingBulk}
	<ConfirmModal
		bind:open={bulkConfirmOpen}
		title={pendingBulk.action === 'suspend' ? t('api_keys.bulk_suspend_title', 'Suspend selected keys') : pendingBulk.action === 'revoke' ? t('api_keys.bulk_revoke_title', 'Revoke selected keys') : t('api_keys.bulk_delete_title', 'Delete selected keys')}
		consequence={(pendingBulk.action === 'suspend' ? t('api_keys.bulk_suspend_confirm', 'Suspend {count} selected key(s)? Suspended keys stop working until unsuspended.') : pendingBulk.action === 'revoke' ? t('api_keys.bulk_revoke_confirm', 'Revoke {count} selected key(s)? Revoked keys stop working permanently.') : t('api_keys.bulk_delete_confirm', 'Permanently delete {count} selected key(s)? This cannot be undone.')).replace('{count}', String(pendingBulk.ids.length))}
		confirmText={pendingBulk.action === 'delete' ? t('api_keys.delete_text') : t('common.confirm', 'Confirm')}
		onConfirm={confirmBulk}
		onCancel={() => (pendingBulk = null)}
	/>
{/if}

<Modal
	bind:open={createModalOpen}
	onOpenChange={(isOpen) => { if (!isOpen) createForm = { name: '', scopes: [], rate_limit_tier: 'standard', expires_at: '' }; }}
	title={t('api_keys.create')}
	description={t('api_keys.create_description')}
>
	{#snippet children(close)}
			{#if errorMessage}
				<div class="bg-destructive/10 text-destructive mt-3 rounded-md p-3 text-sm">
					{errorMessage}
				</div>
			{/if}

			<div class="mt-4 space-y-4">
				<div>
					<label for="create-name" class="text-foreground block text-sm font-medium">
						{t('api_keys.name')} <span class="text-destructive">*</span>
					</label>
					<p class="text-muted-foreground mt-1 text-xs">{t('api_keys.name_hint', 'A label so you remember what this key is for.')}</p>
					<input
						id="create-name"
						type="text"
						bind:value={createForm.name}
						placeholder={t('api_keys.name_placeholder')}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
				</div>

				<div>
					<label for="create-scopes" class="text-foreground block text-sm font-medium">
						{t('api_keys.scopes')}
					</label>
					<p class="text-muted-foreground mt-1 text-xs">{t('api_keys.scopes_hint', 'The API endpoints this key can access — pick each permission it needs.')}</p>
					<div class="mt-1 flex flex-wrap gap-1.5">
						{#each createForm.scopes as scope}
							<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium">
								{scope}
								<button
									class="hover:text-destructive ms-0.5 rounded"
									onclick={() => createForm.scopes = createForm.scopes.filter((s) => s !== scope)}
								>×</button>
							</span>
						{/each}
					</div>
					<Autocomplete
						placeholder={t('api_keys.add_scope', 'Add scope...')}
						emptyMessage={t('common.no_results', 'No results')}
						fetcher={(q) => fetchScopes(q, createForm.scopes)}
						initialFetcher={() => fetchScopesInitial(createForm.scopes)}
						selectedId={null}
						onSelect={(item) => { if (!createForm.scopes.includes(item.id)) createForm.scopes = [...createForm.scopes, item.id]; }}
					/>
					<p class="text-muted-foreground mt-1 text-xs">{t('api_keys.scopes_hint')}</p>
				</div>

				<div>
					<label for="create-tier" class="text-foreground block text-sm font-medium">
						{t('api_keys.rate_limit_tier')}
					</label>
					<p class="text-muted-foreground mt-1 text-xs">{t('api_keys.tier_hint', 'The rate-limit tier for this key.')}</p>
					<AutocompleteSelect
						bind:value={createForm.rate_limit_tier}
						options={tierOptions}
						placeholder={t('api_keys.tier')}
						class="mt-1 w-full"
					/>
				</div>

				<div>
					<label for="create-expires" class="text-foreground block text-sm font-medium">
						{t('api_keys.expires_at')}
					</label>
					<input
						id="create-expires"
						type="datetime-local"
						bind:value={createForm.expires_at}
						class="border-border-input bg-background text-foreground mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
					<p class="text-muted-foreground mt-1 text-xs">{t('api_keys.expires_hint')}</p>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3">
				<button
					type="button"
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={close}
				>
					{t('api_keys.cancel')}
				</button>
				<button
					type="button"
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					disabled={createLoading || !createForm.name.trim()}
					onclick={createKey}
				>
					{createLoading ? t('api_keys.creating') : t('api_keys.create')}
				</button>
			</div>
	{/snippet}
</Modal>

<Modal
	bind:open={oneTimeKeyModalOpen}
	onOpenChange={(isOpen) => { if (!isOpen) { oneTimeKeyValue = ''; } }}
	title={t('api_keys.key_created')}
	size="lg"
>
	{#snippet children(close)}
			<div class="bg-amber-500/10 mt-3 rounded-md p-3">
				<p class="text-amber-500 text-sm font-medium">
					{t('api_keys.save_warning')}
				</p>
			</div>

			<div class="mt-4">
				<label for="one-time-key" class="text-foreground block text-sm font-medium">
					{t('api_keys.your_key')}
				</label>
				<div class="mt-1 flex items-center gap-2">
					<input
						id="one-time-key"
						type="text"
						readonly
						value={oneTimeKeyValue}
						class="border-border-input bg-background text-foreground flex-1 rounded-9px border px-3 py-2 font-mono text-sm focus:shadow-date-field-focus focus:outline-none"
					/>
					<button
						type="button"
						class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-2 text-sm font-medium transition-colors"
						onclick={copyToClipboard}
					>
						{#if copiedToClipboard}
							<Check class="h-4 w-4 text-emerald-500" />
							{t('api_keys.copied')}
						{:else}
							<Copy class="h-4 w-4" />
							{t('api_keys.copy')}
						{/if}
					</button>
				</div>
			</div>

			<div class="mt-6 flex justify-end">
				<button
					type="button"
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90"
					onclick={close}
				>
					{t('api_keys.done')}
				</button>
			</div>
	{/snippet}
</Modal>

<Modal
	bind:open={editModalOpen}
	onOpenChange={(isOpen) => { if (!isOpen) editForm = { id: '', name: '', scopes: [], rate_limit_tier: 'basic' }; }}
	title={t('api_keys.edit')}
>
	{#snippet children(close)}
			{#if errorMessage}
				<div class="bg-destructive/10 text-destructive mt-3 rounded-md p-3 text-sm">
					{errorMessage}
				</div>
			{/if}

			<div class="mt-4 space-y-4">
				<div>
					<label for="edit-name" class="text-foreground block text-sm font-medium">
						{t('api_keys.name')} <span class="text-destructive">*</span>
					</label>
					<input
						id="edit-name"
						type="text"
						bind:value={editForm.name}
						placeholder={t('api_keys.name_placeholder')}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 mt-1 block w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
					/>
				</div>

				<div>
					<label for="edit-scopes" class="text-foreground block text-sm font-medium">
						{t('api_keys.scopes')}
					</label>
					<div class="mt-1 flex flex-wrap gap-1.5">
						{#each editForm.scopes as scope}
							<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium">
								{scope}
								<button
									class="hover:text-destructive ms-0.5 rounded"
									onclick={() => editForm.scopes = editForm.scopes.filter((s) => s !== scope)}
								>×</button>
							</span>
						{/each}
					</div>
					<Autocomplete
						placeholder={t('api_keys.add_scope', 'Add scope...')}
						emptyMessage={t('common.no_results', 'No results')}
						fetcher={(q) => fetchScopes(q, editForm.scopes)}
						initialFetcher={() => fetchScopesInitial(editForm.scopes)}
						selectedId={null}
						onSelect={(item) => { if (!editForm.scopes.includes(item.id)) editForm.scopes = [...editForm.scopes, item.id]; }}
					/>
					<p class="text-muted-foreground mt-1 text-xs">{t('api_keys.scopes_hint')}</p>
				</div>

				<div>
					<label for="edit-tier" class="text-foreground block text-sm font-medium">
						{t('api_keys.rate_limit_tier')}
					</label>
					<AutocompleteSelect
						bind:value={editForm.rate_limit_tier}
						options={tierOptions}
						placeholder={t('api_keys.tier')}
						class="mt-1 w-full"
					/>
				</div>
			</div>

			<div class="mt-6 flex justify-end gap-3">
				<button
					type="button"
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={close}
				>
					{t('api_keys.cancel')}
				</button>
				<button
					type="button"
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					disabled={editLoading || !editForm.name.trim()}
					onclick={updateKey}
				>
					{editLoading ? t('api_keys.saving') : t('api_keys.save_key')}
				</button>
			</div>
	{/snippet}
</Modal>
