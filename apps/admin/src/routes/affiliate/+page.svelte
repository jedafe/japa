<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';

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

	let activeTab = $state<'providers' | 'members' | 'rates'>('providers');
	let providers = $state<AffiliateProvider[]>([]);
	let members = $state<AffiliateMember[]>([]);
	let rates = $state<CommissionRates>({ content_creation: 0, referral_signup: 0, referral_purchase: 0 });
	let loading = $state(true);
	let error = $state('');
	let showCreateModal = $state(false);
	let newProvider = $state({ name: '', base_url: '', commission_rate: 0 });
	let saving = $state(false);

	onMount(load);

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
		if (!confirm(t('affiliate.confirm_delete_provider'))) return;
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

	function statusBadge(status: string) {
		switch (status) {
			case 'active': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'suspended': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			case 'pending': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}
</script>

<svelte:head>
	<title>{t('affiliate.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('affiliate.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('affiliate.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'providers' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'providers'; load(); }}
	>
		{t('affiliate.providers')}
	</button>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'members' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'members'; load(); }}
	>
		{t('affiliate.members')}
	</button>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'rates' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'rates'; load(); }}
	>
		{t('affiliate.commission_rates')}
	</button>
</div>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
		{error}
	</div>
{/if}

{#if loading}
	<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if activeTab === 'providers'}
	<div class="mb-4 flex justify-end">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={() => { showCreateModal = true; }}
		>
			{t('affiliate.add_provider')}
		</button>
	</div>

	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('affiliate.name')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('affiliate.base_url')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('affiliate.commission_rate')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('affiliate.status')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each providers as provider (provider.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-medium">{provider.name}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{provider.base_url}</td>
						<td class="px-4 py-3">{provider.commission_rate}%</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusBadge(provider.is_active ? 'active' : 'suspended')}">
								{provider.is_active ? t('affiliate.active') : t('affiliate.inactive')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<button
								class="text-xs text-red-500 hover:text-red-700"
								onclick={() => deleteProvider(provider.id)}
							>
								{t('common.delete')}
							</button>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('affiliate.no_providers')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

{:else if activeTab === 'members'}
	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('affiliate.username')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('affiliate.status')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('affiliate.earnings')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('affiliate.conversions')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each members as member (member.user_id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-medium">{member.username}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusBadge(member.status)}">
								{member.status}
							</span>
						</td>
						<td class="px-4 py-3 text-right">${member.total_earnings.toFixed(2)}</td>
						<td class="px-4 py-3 text-right">{member.conversion_count}</td>
						<td class="px-4 py-3 text-right">
							{#if member.status === 'active'}
								<button
									class="text-xs text-red-500 hover:text-red-700"
									onclick={() => updateMemberStatus(member.user_id, 'suspended')}
								>
									{t('affiliate.suspend')}
								</button>
							{:else}
								<button
									class="text-xs text-emerald-500 hover:text-emerald-700"
									onclick={() => updateMemberStatus(member.user_id, 'active')}
								>
									{t('affiliate.activate')}
								</button>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('affiliate.no_members')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

{:else if activeTab === 'rates'}
	<div class="mx-auto max-w-lg rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="space-y-4">
			<div>
				<label for="rate-content" class="mb-1 block text-sm font-medium">{t('affiliate.content_creation')}</label>
				<div class="flex items-center gap-2">
					<input
						id="rate-content"
						type="number"
						min="0"
						max="100"
						step="0.1"
						class="w-24 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={rates.content_creation}
					/>
					<span class="text-sm" style="color: var(--color-muted)">%</span>
				</div>
			</div>
			<div>
				<label for="rate-signup" class="mb-1 block text-sm font-medium">{t('affiliate.referral_signup')}</label>
				<div class="flex items-center gap-2">
					<input
						id="rate-signup"
						type="number"
						min="0"
						max="100"
						step="0.1"
						class="w-24 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={rates.referral_signup}
					/>
					<span class="text-sm" style="color: var(--color-muted)">%</span>
				</div>
			</div>
			<div>
				<label for="rate-purchase" class="mb-1 block text-sm font-medium">{t('affiliate.referral_purchase')}</label>
				<div class="flex items-center gap-2">
					<input
						id="rate-purchase"
						type="number"
						min="0"
						max="100"
						step="0.1"
						class="w-24 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={rates.referral_purchase}
					/>
					<span class="text-sm" style="color: var(--color-muted)">%</span>
				</div>
			</div>
			<button
				class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				disabled={saving}
				onclick={saveRates}
			>
				{saving ? t('common.saving') : t('common.save')}
			</button>
		</div>
	</div>
{/if}

{#if showCreateModal}
	<Modal title={t('affiliate.add_provider')} onclose={() => { showCreateModal = false; }}>
		<div class="space-y-4">
			<div>
				<label for="provider-name" class="mb-1 block text-sm font-medium">{t('affiliate.name')}</label>
				<input
					id="provider-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={newProvider.name}
				/>
			</div>
			<div>
				<label for="provider-url" class="mb-1 block text-sm font-medium">{t('affiliate.base_url')}</label>
				<input
					id="provider-url"
					type="url"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder="https://example.com"
					bind:value={newProvider.base_url}
				/>
			</div>
			<div>
				<label for="provider-rate" class="mb-1 block text-sm font-medium">{t('affiliate.commission_rate')}</label>
				<div class="flex items-center gap-2">
					<input
						id="provider-rate"
						type="number"
						min="0"
						max="100"
						step="0.1"
						class="w-24 rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={newProvider.commission_rate}
					/>
					<span class="text-sm" style="color: var(--color-muted)">%</span>
				</div>
			</div>
			<div class="flex justify-end gap-2">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium"
					style="border: 1px solid var(--color-border)"
					onclick={() => { showCreateModal = false; }}
				>
					{t('common.cancel')}
				</button>
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving || !newProvider.name || !newProvider.base_url}
					onclick={createProvider}
				>
					{saving ? t('common.creating') : t('common.create')}
				</button>
			</div>
		</div>
	</Modal>
{/if}
