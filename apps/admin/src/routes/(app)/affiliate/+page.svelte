<script lang="ts">
	import { api, affiliateProvidersAdminApi, affiliateProgramAdminApi, affiliateLinksAdminApi, affiliateEnhancementsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Plus, Trash2, Link2, Users, Percent, DollarSign } from 'lucide-svelte';

	type AffiliateProvider = {
		id: string;
		name: string;
		slug: string;
		description: string | null;
		provider_type: string;
		base_url: string | null;
		tracking_param: string | null;
		tracking_value: string | null;
		commission_rate: number | null;
		is_active: boolean;
		is_featured: boolean;
		created_at: string;
		updated_at: string;
	};

	type CommissionRates = {
		pro_monthly: number;
		pro_annual: number;
		course: number;
	};

	type AffiliateMember = {
		user_id: string;
		tenant_id: string;
		enrolled_at: string;
		status: string;
		total_clicks: number;
		total_conversions: number;
		total_earned_credits: number;
		payout_method: string | null;
		notes: string | null;
	};

	type AffiliateLink = {
		id: string;
		tenant_id: string;
		user_id: string;
		product_type: string;
		product_id: string | null;
		code: string;
		description: string | null;
		destination_url: string;
		click_count: number;
		conversion_count: number;
		total_earned: number;
		is_active: boolean;
		created_at: string;
	};

	type ScanResult = {
		id: string;
		content_id: string;
		url: string;
		domain: string;
		detection_method: string;
		action_taken: string;
		created_at: string;
	};

	type AuthorCommission = {
		id: string;
		content_id: string;
		author_user_id: string;
		provider_name: string | null;
		external_commission: number;
		author_share: number;
		author_commission_pct: number;
		credited: boolean;
		created_at: string;
	};

	let activeTab = $state('providers');
	let providers = $state<AffiliateProvider[]>([]);
	let members = $state<AffiliateMember[]>([]);
	let links = $state<AffiliateLink[]>([]);
	let rates = $state<CommissionRates>({ pro_monthly: 0, pro_annual: 0, course: 0 });
	let scanResults = $state<ScanResult[]>([]);
	let commissions = $state<AuthorCommission[]>([]);
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

	const tabs = $derived([
		{ value: 'providers', label: t('affiliate.providers'), icon: Link2 },
		{ value: 'members', label: t('affiliate.members'), icon: Users },
		{ value: 'links', label: t('affiliate.links'), icon: Link2 },
		{ value: 'rates', label: t('affiliate.commission_rates'), icon: Percent },
		{ value: 'scanner', label: t('affiliate.link_scanner', 'Link Scanner'), icon: Link2 },
		{ value: 'author_commissions', label: t('affiliate.author_commissions', 'Author Commissions'), icon: DollarSign }
	]);

	const providerColumns = $derived([
		{
			key: 'name',
			label: t('affiliate.name'),
			sortable: true,
			render: (value: string) => `<span class="font-medium text-foreground">${value ?? '—'}</span>`
		},
		{
			key: 'base_url',
			label: t('affiliate.base_url'),
			render: (value: string | null) => value ? `<span class="font-mono text-xs text-muted">${value}</span>` : '<span class="text-xs text-muted">—</span>'
		},
		{
			key: 'commission_rate',
			label: t('affiliate.commission_rate'),
			sortable: true,
			render: (value: number | null) => value != null ? `<span class="font-medium">${value}%</span>` : '<span class="text-xs text-muted">—</span>'
		},
		{
			key: 'is_active',
			label: t('affiliate.status'),
			render: (_: any, row: any) => {
				const activeLabel = t('affiliate.active');
				const inactiveLabel = t('affiliate.inactive');
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${row.is_active ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border'}">${row.is_active ? activeLabel : inactiveLabel}</span>`;
			}
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: ProviderActionsCell
		}
	]);

	const memberColumns = $derived([
		{
			key: 'user_id',
			label: t('affiliate.username'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs">${value}</span>`
		},
		{
			key: 'status',
			label: t('affiliate.status'),
			render: (value: string) => {
				const classes: Record<string, string> = {
					active: 'bg-success/15 text-success border-success',
					suspended: 'bg-danger/15 text-danger border-danger',
					banned: 'bg-danger/15 text-danger border-danger'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-surface text-foreground border'}">${value}</span>`;
			}
		},
		{
			key: 'total_earned_credits',
			label: t('affiliate.earnings'),
			sortable: true,
			render: (value: number) => `<span class="font-medium">${(value ?? 0).toFixed(2)}</span>`
		},
		{
			key: 'total_conversions',
			label: t('affiliate.conversions'),
			sortable: true,
			render: (value: number) => `<span>${value ?? 0}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: MemberActionsCell
		}
	]);

	const linkColumns = $derived([
		{
			key: 'user_id',
			label: t('affiliate.user'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs">${value}</span>`
		},
		{
			key: 'code',
			label: t('affiliate.code'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs font-medium text-foreground">${value}</span>`
		},
		{
			key: 'destination_url',
			label: t('affiliate.url'),
			render: (value: string) => `<span class="text-xs text-muted truncate max-w-[200px] block">${value}</span>`
		},
		{
			key: 'click_count',
			label: t('affiliate.clicks'),
			sortable: true,
			render: (value: number) => `<span class="font-medium">${(value ?? 0).toLocaleString()}</span>`
		},
		{
			key: 'conversion_count',
			label: t('affiliate.conversions'),
			sortable: true,
			render: (value: number) => `<span class="font-medium">${value ?? 0}</span>`
		},
		{
			key: 'created_at',
			label: t('affiliate.created'),
			sortable: true,
			render: (value: string) => `<span class="text-sm">${new Date(value).toLocaleDateString()}</span>`
		}
	]);

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
				const result = await affiliateProgramAdminApi.members();
				if (result.ok) {
					members = (Array.isArray(result.data) ? result.data : []) as AffiliateMember[];
					if (searchQuery) {
						const q = searchQuery.toLowerCase();
						members = members.filter((m) => m.user_id?.toLowerCase().includes(q) || m.status?.toLowerCase().includes(q));
					}
				} else {
					error = result.error;
				}
			} else if (activeTab === 'links') {
				const result = await affiliateLinksAdminApi.list();
				if (result.ok) {
					links = (Array.isArray(result.data) ? result.data : []) as AffiliateLink[];
					if (searchQuery) {
						const q = searchQuery.toLowerCase();
						links = links.filter((l) => l.code?.toLowerCase().includes(q) || l.destination_url?.toLowerCase().includes(q) || l.user_id?.toLowerCase().includes(q));
					}
				} else {
					error = result.error;
				}
			} else if (activeTab === 'rates') {
				const result = await affiliateProgramAdminApi.commissionRates();
				if (result.ok) {
					const data = result.data as any;
					rates = data?.rates ?? rates;
				} else {
					error = result.error;
				}
			} else if (activeTab === 'scanner') {
				const result = await affiliateEnhancementsAdminApi.scanResults();
				if (result.ok) {
					const data = result.data as any;
					scanResults = (Array.isArray(data) ? data : []) as ScanResult[];
				} else {
					error = result.error;
				}
			} else if (activeTab === 'author_commissions') {
				const result = await affiliateEnhancementsAdminApi.commissions();
				if (result.ok) {
					const data = result.data as any;
					commissions = (Array.isArray(data) ? data : []) as AuthorCommission[];
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
			const slug = newProvider.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
			const result = await affiliateProvidersAdminApi.create({ ...newProvider, slug });
			if (result.ok) {
				showCreateModal = false;
				newProvider = { name: '', base_url: '', commission_rate: 0 };
				load();
			} else {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function deleteProvider(id: string) {
		try {
			const result = await affiliateProvidersAdminApi.delete(id);
			if (result.ok) {
				providers = providers.filter((p) => p.id !== id);
			} else {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		}
	}

	async function updateMemberStatus(userId: string, status: string) {
		try {
			const result = await affiliateProgramAdminApi.updateMemberStatus(userId, { status });
			if (result.ok) {
				load();
			} else {
				error = result.error ?? t('common.error');
			}
		} catch {
			error = t('common.error');
		}
	}

	async function saveRates() {
		saving = true;
		try {
			const result = await affiliateProgramAdminApi.updateCommissionRates(rates);
			if (!result.ok) {
				error = result.error ?? t('common.error');
			}
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
	actions={activeTab === 'providers' && canManageAffiliate
		? [{ label: t('affiliate.add_provider'), icon: Plus, onclick: () => showCreateModal = true }]
		: []}
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
						{bulkDeleting ? t('common.deleting') : t('common.delete_selected')} ({selectedProviderIds.size})
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
						{bulkDeleting ? t('common.deleting') : t('affiliate.suspend')} ({selectedMemberIds.size})
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
				emptyMessage={t('affiliate.no_links')}
				searchable
				onsearch={(q) => { searchQuery = q; load(); }}
				selectable
				bind:selectedIds={selectedLinkIds}
			>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDeleteLinks} disabled={bulkDeleting || selectedLinkIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? t('common.deleting') : t('common.delete_selected')} ({selectedLinkIds.size})
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
						<label for="rate-monthly" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.pro_monthly')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-monthly"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={rates.pro_monthly}
							/>
							<span class="text-sm text-muted">%</span>
						</div>
					</div>
					<div>
						<label for="rate-annual" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.pro_annual')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-annual"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={rates.pro_annual}
							/>
							<span class="text-sm text-muted">%</span>
						</div>
					</div>
					<div>
						<label for="rate-course" class="mb-1 block text-sm font-medium text-foreground-alt">{t('affiliate.course')}</label>
						<div class="flex items-center gap-2">
							<input
								id="rate-course"
								type="number"
								min="0"
								max="100"
								step="0.1"
								class="w-24 rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={rates.course}
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
	{:else if activeTab === 'scanner'}
		<Card>
			<CardContent class="p-0">
				{#if scanResults.length === 0}
					<div class="flex flex-col items-center justify-center py-12 text-center">
						<p class="text-sm text-muted">{t('affiliate.no_scan_results', 'No unregistered affiliate links detected')}</p>
					</div>
				{:else}
					<div class="overflow-x-auto">
						<table class="w-full text-sm">
							<thead>
								<tr class="border-b" style="border-color: var(--color-border)">
									<th class="px-4 py-3 text-left text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.url', 'URL')}</th>
									<th class="px-4 py-3 text-left text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.domain', 'Domain')}</th>
									<th class="px-4 py-3 text-left text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.detection', 'Detection')}</th>
									<th class="px-4 py-3 text-left text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.action', 'Action')}</th>
									<th class="px-4 py-3 text-left text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.created', 'Created')}</th>
								</tr>
							</thead>
							<tbody>
								{#each scanResults as result (result.id)}
									<tr class="border-b last:border-0" style="border-color: var(--color-border)">
										<td class="px-4 py-3 font-mono text-xs truncate max-w-[300px]" style="color: var(--color-foreground)">{result.url}</td>
										<td class="px-4 py-3 text-sm" style="color: var(--color-foreground)">{result.domain}</td>
										<td class="px-4 py-3">
											<span class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs" style="border-color: var(--color-border); color: var(--color-muted)">{result.detection_method}</span>
										</td>
										<td class="px-4 py-3">
											<span class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-semibold {result.action_taken === 'approved' ? 'bg-success/15 text-success border-success' : result.action_taken === 'blocked' ? 'bg-danger/15 text-danger border-danger' : 'bg-warning/15 text-warning border-warning'}">{result.action_taken}</span>
										</td>
										<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(result.created_at).toLocaleDateString()}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</CardContent>
		</Card>
	{:else if activeTab === 'author_commissions'}
		<Card>
			<CardContent class="p-0">
				{#if commissions.length === 0}
					<div class="flex flex-col items-center justify-center py-12 text-center">
						<p class="text-sm text-muted">{t('affiliate.no_commissions', 'No author commissions recorded yet')}</p>
					</div>
				{:else}
					<div class="overflow-x-auto">
						<table class="w-full text-sm">
							<thead>
								<tr class="border-b" style="border-color: var(--color-border)">
									<th class="px-4 py-3 text-left text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.provider', 'Provider')}</th>
									<th class="px-4 py-3 text-left text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.author', 'Author')}</th>
									<th class="px-4 py-3 text-right text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.external_commission', 'External Comm.')}</th>
									<th class="px-4 py-3 text-right text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.author_share', 'Author Share')}</th>
									<th class="px-4 py-3 text-right text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.rate', 'Rate')}</th>
									<th class="px-4 py-3 text-center text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.credited', 'Credited')}</th>
									<th class="px-4 py-3 text-left text-xs font-medium" style="color: var(--color-muted)">{t('affiliate.created', 'Created')}</th>
								</tr>
							</thead>
							<tbody>
								{#each commissions as c (c.id)}
									<tr class="border-b last:border-0" style="border-color: var(--color-border)">
										<td class="px-4 py-3 font-medium" style="color: var(--color-foreground)">{c.provider_name ?? '—'}</td>
										<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-foreground)">{c.author_user_id}</td>
										<td class="px-4 py-3 text-right font-medium" style="color: var(--color-foreground)">${c.external_commission.toFixed(2)}</td>
										<td class="px-4 py-3 text-right font-medium" style="color: var(--color-success)">${c.author_share.toFixed(2)}</td>
										<td class="px-4 py-3 text-right" style="color: var(--color-muted)">{c.author_commission_pct}%</td>
										<td class="px-4 py-3 text-center">
											<span class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-semibold {c.credited ? 'bg-success/15 text-success border-success' : 'bg-warning/15 text-warning border-warning'}">{c.credited ? t('affiliate.yes', 'Yes') : t('affiliate.pending', 'Pending')}</span>
										</td>
										<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(c.created_at).toLocaleDateString()}</td>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
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
<Modal bind:open={showMemberStatusConfirm} title={t('common.confirm', 'Confirm')}>
	{#if memberStatusTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('affiliate.change_member_status_confirm', 'Change status of')} <strong class="font-mono text-xs">{memberStatusTarget.member.user_id}</strong> {t('affiliate.to', 'to')} <strong>{memberStatusTarget.status}</strong>?
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
