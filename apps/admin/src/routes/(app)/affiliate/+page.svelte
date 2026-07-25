<script lang="ts">
	import { api, affiliateProvidersAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Plus, Trash2, Link2, Users, Percent } from 'lucide-svelte';

	type AffiliateProvider = {
		id: string;
		name: string;
		base_url: string;
		commission_rate: number;
		is_active: boolean;
		created_at: string;
	};

	type CommissionRates = {
		content_creation: number;
		referral_signup: number;
		referral_purchase: number;
	};

	type AffiliateMember = {
		user_id: string;
		username: string;
		status: string;
		total_earnings: number;
		conversion_count: number;
		joined_at: string;
	};

	type AffiliateLink = {
		id: string;
		user_id: string;
		code: string;
		url: string;
		clicks: number;
		conversions: number;
		created_at: string;
	};

	let activeTab = $state('providers');
	let providers = $state<AffiliateProvider[]>([]);
	let members = $state<AffiliateMember[]>([]);
	let links = $state<AffiliateLink[]>([]);
	let rates = $state<CommissionRates>({ content_creation: 0, referral_signup: 0, referral_purchase: 0 });
	let loading = $state(true);
	let error = $state('');
	let showCreateModal = $state(false);
	let newProvider = $state({ name: '', base_url: '', commission_rate: 0 });
	let saving = $state(false);
	let searchQuery = $state("");

	let selectedProviderIds = $state<Set<string>>(new Set());
	let selectedMemberIds = $state<Set<string>>(new Set());
	let selectedLinkIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const canManageAffiliate = $derived(hasPermission('affiliate.manage'));

	// Delete provider confirm
	let showDeleteProviderConfirm = $state(false);
	let deleteProviderTarget = $state<AffiliateProvider | null>(null);

	// Member status confirm
	let showMemberStatusConfirm = $state(false);
	let memberStatusTarget = $state<{ member: AffiliateMember; status: string } | null>(null);

	const tabs = [
		{ value: 'providers', label: 'Providers', icon: Link2 },
		{ value: 'members', label: 'Members', icon: Users },
		{ value: 'links', label: 'Links', icon: Link2 },
		{ value: 'rates', label: 'Commission Rates', icon: Percent }
	];

	const providerColumns = [
		{
			key: 'name',
			label: 'Name',
			sortable: true,
			render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
		},
		{
			key: 'base_url',
			label: 'Base URL',
			render: (value: string) => `<span class="font-mono text-xs text-muted">${value}</span>`
		},
		{
			key: 'commission_rate',
			label: 'Commission',
			sortable: true,
			render: (value: number) => `<span class="font-medium">${value}%</span>`
		},
		{
			key: 'is_active',
			label: 'Status',
			render: (_: any, row: any) => {
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${row.is_active ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border'}">${row.is_active ? 'Active' : 'Inactive'}</span>`;
			}
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: ProviderActionsCell
		}
	];

	const memberColumns = [
		{
			key: 'username',
			label: 'Username',
			sortable: true,
			render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
		},
		{
			key: 'status',
			label: 'Status',
			render: (value: string) => {
				const classes: Record<string, string> = {
					active: 'bg-success/15 text-success border-success',
					suspended: 'bg-danger/15 text-danger border-danger',
					pending: 'bg-warning/15 text-warning border-warning'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-surface text-foreground border'}">${value}</span>`;
			}
		},
		{
			key: 'total_earnings',
			label: 'Earnings',
			sortable: true,
			render: (value: number) => `<span class="font-medium">$${(value ?? 0).toFixed(2)}</span>`
		},
		{
			key: 'conversion_count',
			label: 'Conversions',
			sortable: true,
			render: (value: number) => `<span>${value ?? 0}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: MemberActionsCell
		}
	];

	const linkColumns = [
		{
			key: 'user_id',
			label: 'User',
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs">${value}</span>`
		},
		{
			key: 'code',
			label: 'Code',
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs font-medium text-foreground">${value}</span>`
		},
		{
			key: 'url',
			label: 'URL',
			render: (value: string) => `<span class="text-xs text-muted truncate max-w-[200px] block">${value}</span>`
		},
		{
			key: 'clicks',
			label: 'Clicks',
			sortable: true,
			render: (value: number) => `<span class="font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'conversions',
			label: 'Conversions',
			sortable: true,
			render: (value: number) => `<span class="font-medium">${value ?? 0}</span>`
		},
		{
			key: 'created_at',
			label: 'Created',
			sortable: true,
			render: (value: string) => `<span class="text-sm">${new Date(value).toLocaleDateString()}</span>`
		}
	];

	$effect(() => {
		load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			if (activeTab === 'providers') {
				const result = await affiliateProvidersAdminApi.list();
				if (result.ok) {
					providers = (Array.isArray(result.data) ? result.data : []) as AffiliateProvider[];
					if (searchQuery) {
						const q = searchQuery.toLowerCase();
						providers = providers.filter((p) => p.name?.toLowerCase().includes(q) || (p.is_active ? 'active' : 'inactive').includes(q));
					}
				} else {
					error = result.error;
				}
			} else if (activeTab === 'members') {
				const result = await api<AffiliateMember[]>('/admin/affiliate-program/members');
				if (result.ok) {
					members = result.data ?? [];
					if (searchQuery) {
						const q = searchQuery.toLowerCase();
						members = members.filter((m: any) => m.username?.toLowerCase().includes(q) || m.status?.toLowerCase().includes(q));
					}
				} else {
					error = result.error;
				}
			} else if (activeTab === 'links') {
				const result = await api<AffiliateLink[]>('/admin/affiliate-links');
				if (result.ok) {
					links = result.data ?? [];
					if (searchQuery) {
						const q = searchQuery.toLowerCase();
						links = links.filter((l: any) => l.code?.toLowerCase().includes(q) || l.url?.toLowerCase().includes(q) || l.user_id?.toLowerCase().includes(q));
					}
				} else {
					error = result.error;
				}
			} else {
				const result = await api<CommissionRates>('/admin/affiliate-program/commission-rates');
				if (result.ok) {
					rates = result.data;
				} else {
					error = result.error;
				}
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function createProvider() {
		if (!newProvider.name || !newProvider.base_url) return;
		saving = true;
		try {
			await affiliateProvidersAdminApi.create(newProvider);
			showCreateModal = false;
			newProvider = { name: '', base_url: '', commission_rate: 0 };
			load();
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function deleteProvider(id: string) {
		try {
			await affiliateProvidersAdminApi.delete(id);
			providers = providers.filter((p) => p.id !== id);
		} catch {
			error = t('common.error');
		}
	}

	async function updateMemberStatus(userId: string, status: string) {
		try {
			await api(`/admin/affiliate-program/members/${userId}`, {
				method: 'PATCH',
				body: { status }
			});
			load();
		} catch {
			error = t('common.error');
		}
	}

	async function saveRates() {
		saving = true;
		try {
			await api('/admin/affiliate-program/commission-rates', {
				method: 'PATCH',
				body: rates
			});
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDeleteProvider(provider: AffiliateProvider) {
		deleteProviderTarget = provider;
		showDeleteProviderConfirm = true;
	}

	async function executeDeleteProvider() {
		if (!deleteProviderTarget) return;
		await deleteProvider(deleteProviderTarget.id);
		showDeleteProviderConfirm = false;
		deleteProviderTarget = null;
	}

	function confirmMemberStatus(member: AffiliateMember, status: string) {
		memberStatusTarget = { member, status };
		showMemberStatusConfirm = true;
	}

	async function executeMemberStatus() {
		if (!memberStatusTarget) return;
		await updateMemberStatus(memberStatusTarget.member.user_id, memberStatusTarget.status);
		showMemberStatusConfirm = false;
		memberStatusTarget = null;
	}

	async function bulkDeleteProviders() {
		if (selectedProviderIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const id of selectedProviderIds) {
				await affiliateProvidersAdminApi.delete(id);
			}
			selectedProviderIds = new Set();
			load();
		} catch {
			error = t('common.error');
		}
		bulkDeleting = false;
	}

	async function bulkDeleteMembers() {
		if (selectedMemberIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const userId of selectedMemberIds) {
				await updateMemberStatus(userId, 'suspended');
			}
			selectedMemberIds = new Set();
			load();
		} catch {
			error = t('common.error');
		}
		bulkDeleting = false;
	}

	async function bulkDeleteLinks() {
		if (selectedLinkIds.size === 0) return;
		bulkDeleting = true;
		try {
			for (const id of selectedLinkIds) {
				await api(`/admin/affiliate-links/${id}`, { method: 'DELETE' });
			}
			selectedLinkIds = new Set();
			load();
		} catch {
			error = t('common.error');
		}
		bulkDeleting = false;
	}
</script>

<svelte:head>
	<title>{t('affiliate.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('affiliate.title')}
	description={t('affiliate.description')}
	actions={[
		...(canManageAffiliate ? [{ label: t('affiliate.add_provider'), icon: Plus, onclick: () => showCreateModal = true }] : [])
	]}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	{#if loading}
		<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
	{:else if activeTab === 'providers'}
		<Card>
			<CardContent class="p-0">
			<DataTable
				columns={providerColumns}
				data={providers}
				pageSize={20}
				emptyMessage={t('affiliate.no_providers')}
				searchable
				onsearch={(q) => { searchQuery = q; load(); }}
				selectable
				bind:selectedIds={selectedProviderIds}
			>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDeleteProviders} disabled={bulkDeleting || selectedProviderIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? 'Deleting...' : 'Delete Selected'} ({selectedProviderIds.size})
					</Button>
				{/snippet}
			</DataTable>
			</CardContent>
		</Card>
	{:else if activeTab === 'members'}
		<Card>
			<CardContent class="p-0">
			<DataTable
				columns={memberColumns}
				data={members}
				pageSize={20}
				emptyMessage={t('affiliate.no_members')}
				searchable
				onsearch={(q) => { searchQuery = q; load(); }}
				selectable
				bind:selectedIds={selectedMemberIds}
			>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDeleteMembers} disabled={bulkDeleting || selectedMemberIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? 'Deleting...' : 'Suspend Selected'} ({selectedMemberIds.size})
					</Button>
				{/snippet}
			</DataTable>
			</CardContent>
		</Card>
	{:else if activeTab === 'links'}
		<Card>
			<CardContent class="p-0">
			<DataTable
				columns={linkColumns}
				data={links}
				pageSize={20}
				emptyMessage="No affiliate links found"
				searchable
				onsearch={(q) => { searchQuery = q; load(); }}
				selectable
				bind:selectedIds={selectedLinkIds}
			>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDeleteLinks} disabled={bulkDeleting || selectedLinkIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? 'Deleting...' : 'Delete Selected'} ({selectedLinkIds.size})
					</Button>
				{/snippet}
			</DataTable>
			</CardContent>
		</Card>
	{:else if activeTab === 'rates'}
		<Card>
			<CardContent class="p-6">
				<h3 class="mb-4 text-lg font-semibold text-foreground">{t('affiliate.commission_rates')}</h3>
				<div class="mx-auto max-w-lg space-y-4">
					<div>
						<label for="rate-content" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.content_creation')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-content"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={rates.content_creation}
							/>
							<span class="text-sm text-muted">%</span>
						</div>
					</div>
					<div>
						<label for="rate-signup" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.referral_signup')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-signup"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={rates.referral_signup}
							/>
							<span class="text-sm text-muted">%</span>
						</div>
					</div>
					<div>
						<label for="rate-purchase" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.referral_purchase')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-purchase"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={rates.referral_purchase}
							/>
							<span class="text-sm text-muted">%</span>
						</div>
					</div>
					<Button onclick={saveRates} disabled={saving}>
						{saving ? t('common.saving') : t('common.save')}
					</Button>
				</div>
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Add Provider Modal -->
<Modal bind:open={showCreateModal} title={t('affiliate.add_provider')}>
	<div class="space-y-4">
		<div>
			<label for="provider-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.name')}</label>
			<input
				id="provider-name"
				type="text"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				bind:value={newProvider.name}
			/>
		</div>
		<div>
			<label for="provider-url" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.base_url')}</label>
			<input
				id="provider-url"
				type="url"
				class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
				placeholder="https://example.com"
				bind:value={newProvider.base_url}
			/>
		</div>
		<div>
			<label for="provider-rate" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.commission_rate')}</label>
			<div class="flex items-center gap-2">
				<input
					id="provider-rate"
					type="number"
					min="0"
					max="100"
					step="0.1"
					class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={newProvider.commission_rate}
				/>
				<span class="text-sm text-muted">%</span>
			</div>
		</div>
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" onclick={() => showCreateModal = false}>{t('common.cancel')}</Button>
			<Button onclick={createProvider} disabled={saving || !newProvider.name || !newProvider.base_url}>
				{saving ? t('common.creating') : t('common.create')}
			</Button>
		</div>
	</div>
</Modal>

<!-- Delete Provider Confirm -->
<Modal bind:open={showDeleteProviderConfirm} title={t('common.confirmDelete', 'Confirm Delete')}>
	{#if deleteProviderTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('affiliate.delete_provider_confirm', 'Delete provider')} <strong>{deleteProviderTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showDeleteProviderConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDeleteProvider}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>

<!-- Member Status Confirm -->
<Modal bind:open={showMemberStatusConfirm} title={t('common.confirmDelete', 'Confirm Change')}>
	{#if memberStatusTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('affiliate.change_member_status_confirm', 'Change status of')} <strong>{memberStatusTarget.member.username}</strong> {t('affiliate.to', 'to')} <strong>{memberStatusTarget.status}</strong>?
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => showMemberStatusConfirm = false}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={executeMemberStatus}>{t('common.confirm', 'Confirm')}</Button>
	</div>
</Modal>

<!-- ProviderActionsCell component -->
{#snippet ProviderActionsCell({ value, row }: { value: any; row: AffiliateProvider })}
	<div class="flex items-center gap-1">
		{#if canManageAffiliate}
			<Button variant="ghost" size="sm" onclick={() => confirmDeleteProvider(row)} style="color: var(--color-danger)" title={t('common.delete')}>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}

<!-- MemberActionsCell component -->
{#snippet MemberActionsCell({ value, row }: { value: any; row: AffiliateMember })}
	<div class="flex items-center gap-1">
		{#if canManageAffiliate}
			<Button variant="ghost" size="sm" onclick={() => confirmMemberStatus(row, row.status === 'active' ? 'suspended' : 'active')} style={row.status === 'active' ? 'color: var(--color-danger)' : 'color: var(--color-success)'} title={row.status === 'active' ? t('common.suspend') : t('common.activate')}>
				{row.status === 'active' ? t('common.suspend') : t('common.activate')}
			</Button>
		{/if}
	</div>
{/snippet}
