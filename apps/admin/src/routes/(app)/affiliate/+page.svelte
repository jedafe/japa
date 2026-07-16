<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
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
			render: (value: string) => `<span class="font-medium text-gray-900">${value}</span>`
		},
		{
			key: 'base_url',
			label: 'Base URL',
			render: (value: string) => `<span class="font-mono text-xs text-gray-500">${value}</span>`
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
				const variant = row.is_active ? 'success' : 'secondary';
				const label = row.is_active ? 'Active' : 'Inactive';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${row.is_active ? 'bg-success-100 text-success-800 border-success-200' : 'bg-gray-100 text-gray-800 border-gray-200'}">${label}</span>`;
			}
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: any) => `
				<button onclick="window.__deleteProvider('${row.id}')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Delete</button>
			`
		}
	];

	const memberColumns = [
		{
			key: 'username',
			label: 'Username',
			sortable: true,
			render: (value: string) => `<span class="font-medium text-gray-900">${value}</span>`
		},
		{
			key: 'status',
			label: 'Status',
			render: (value: string) => {
				const classes: Record<string, string> = {
					active: 'bg-success-100 text-success-800 border-success-200',
					suspended: 'bg-danger-100 text-danger-800 border-danger-200',
					pending: 'bg-warning-100 text-warning-800 border-warning-200'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-gray-100 text-gray-800 border-gray-200'}">${value}</span>`;
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
			render: (_: any, row: any) => {
				if (row.status === 'active') {
					return `<button onclick="window.__updateMemberStatus('${row.user_id}', 'suspended')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50">Suspend</button>`;
				}
				return `<button onclick="window.__updateMemberStatus('${row.user_id}', 'active')" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-success-700 hover:bg-success-50">Activate</button>`;
			}
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
			render: (value: string) => `<span class="font-mono text-xs font-medium text-gray-900">${value}</span>`
		},
		{
			key: 'url',
			label: 'URL',
			render: (value: string) => `<span class="text-xs text-gray-500 truncate max-w-[200px] block">${value}</span>`
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
		(window as any).__deleteProvider = deleteProvider;
		(window as any).__updateMemberStatus = updateMemberStatus;
		return () => {
			delete (window as any).__deleteProvider;
			delete (window as any).__updateMemberStatus;
		};
	});

	async function load() {
		loading = true;
		error = '';
		try {
			if (activeTab === 'providers') {
				const result = await api<AffiliateProvider[]>('/admin/affiliate-providers');
				if (result.ok) {
					providers = result.data ?? [];
				} else {
					error = result.error;
				}
			} else if (activeTab === 'members') {
				const result = await api<AffiliateMember[]>('/admin/affiliate-program/members');
				if (result.ok) {
					members = result.data ?? [];
				} else {
					error = result.error;
				}
			} else if (activeTab === 'links') {
				const result = await api<AffiliateLink[]>('/admin/affiliate-links');
				if (result.ok) {
					links = result.data ?? [];
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
			await api('/admin/affiliate-providers', { method: 'POST', body: newProvider });
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
			await api(`/admin/affiliate-providers/${id}`, { method: 'DELETE' });
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

	// Expose functions for DataTable inline actions
	if (typeof window !== 'undefined') {
		(window as any).__deleteProvider = deleteProvider;
		(window as any).__updateMemberStatus = updateMemberStatus;
	}
</script>

<svelte:head>
	<title>{t('affiliate.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('affiliate.title')}
	description={t('affiliate.description')}
	actions={[
		{ label: t('affiliate.add_provider'), icon: Plus, onclick: () => showCreateModal = true }
	]}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	{#if loading}
		<p class="py-8 text-center text-sm text-gray-500">{t('common.loading')}</p>
	{:else if activeTab === 'providers'}
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={providerColumns}
					data={providers}
					pageSize={20}
					emptyMessage={t('affiliate.no_providers')}
				/>
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
				/>
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
				/>
			</CardContent>
		</Card>
	{:else if activeTab === 'rates'}
		<Card>
			<CardContent class="p-6">
				<h3 class="mb-4 text-lg font-semibold text-gray-900">{t('affiliate.commission_rates')}</h3>
				<div class="mx-auto max-w-lg space-y-4">
					<div>
						<label for="rate-content" class="mb-1 block text-sm font-medium text-gray-700">{t('affiliate.content_creation')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-content"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
								bind:value={rates.content_creation}
							/>
							<span class="text-sm text-gray-500">%</span>
						</div>
					</div>
					<div>
						<label for="rate-signup" class="mb-1 block text-sm font-medium text-gray-700">{t('affiliate.referral_signup')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-signup"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
								bind:value={rates.referral_signup}
							/>
							<span class="text-sm text-gray-500">%</span>
						</div>
					</div>
					<div>
						<label for="rate-purchase" class="mb-1 block text-sm font-medium text-gray-700">{t('affiliate.referral_purchase')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-purchase"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
								bind:value={rates.referral_purchase}
							/>
							<span class="text-sm text-gray-500">%</span>
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
			<label for="provider-name" class="mb-1 block text-sm font-medium text-gray-700">{t('affiliate.name')}</label>
			<input
				id="provider-name"
				type="text"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				bind:value={newProvider.name}
			/>
		</div>
		<div>
			<label for="provider-url" class="mb-1 block text-sm font-medium text-gray-700">{t('affiliate.base_url')}</label>
			<input
				id="provider-url"
				type="url"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
				placeholder="https://example.com"
				bind:value={newProvider.base_url}
			/>
		</div>
		<div>
			<label for="provider-rate" class="mb-1 block text-sm font-medium text-gray-700">{t('affiliate.commission_rate')}</label>
			<div class="flex items-center gap-2">
				<input
					id="provider-rate"
					type="number"
					min="0"
					max="100"
					step="0.1"
					class="w-24 rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
					bind:value={newProvider.commission_rate}
				/>
				<span class="text-sm text-gray-500">%</span>
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
