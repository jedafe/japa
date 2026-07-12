<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Subscriber = {
		id: string;
		email: string;
		name?: string;
		status: string;
		created_at: string;
	};

	type Campaign = {
		id: string;
		name: string;
		subject: string;
		status: string;
		sent_at?: string;
		recipient_count?: number;
		created_at: string;
	};

	let activeTab = $state<'subscribers' | 'campaigns'>('subscribers');
	let subscribers = $state<Subscriber[]>([]);
	let campaigns = $state<Campaign[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Campaign creation
	let showCreateModal = $state(false);
	let newCampaign = $state({ name: '', subject: '', html: '' });
	let creating = $state(false);

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			if (activeTab === 'subscribers') {
				const result = await api<{ data: Subscriber[] }>('/admin/newsletter/subscribers');
				if (result.ok) {
					subscribers = result.data.data ?? [];
				} else {
					error = result.error;
				}
			} else {
				const result = await api<{ data: Campaign[] }>('/admin/newsletter/campaigns');
				if (result.ok) {
					campaigns = result.data.data ?? [];
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

	async function deleteSubscriber(id: string) {
		if (!confirm(t('newsletter.confirm_delete_subscriber'))) return;
		try {
			await api(`/admin/newsletter/subscribers/${id}`, { method: 'DELETE' });
			subscribers = subscribers.filter((s) => s.id !== id);
		} catch {
			error = t('common.error');
		}
	}

	async function createCampaign() {
		if (!newCampaign.name || !newCampaign.subject) return;
		creating = true;
		try {
			const result = await api('/admin/newsletter/campaigns', {
				method: 'POST',
				body: newCampaign
			});
			if (result.ok) {
				showCreateModal = false;
				newCampaign = { name: '', subject: '', html: '' };
				load();
			}
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	async function sendCampaign(id: string) {
		if (!confirm(t('newsletter.confirm_send'))) return;
		try {
			await api(`/admin/newsletter/campaigns/${id}/send`, { method: 'POST' });
			load();
		} catch {
			error = t('common.error');
		}
	}

	function statusBadge(status: string) {
		switch (status) {
			case 'sent': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'sending': return 'bg-blue-500/15 text-blue-700 dark:text-blue-400';
			case 'draft': return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}
</script>

<svelte:head>
	<title>{t('newsletter.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('newsletter.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('newsletter.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'subscribers' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'subscribers'; load(); }}
	>
		{t('newsletter.subscribers')}
	</button>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'campaigns' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'campaigns'; load(); }}
	>
		{t('newsletter.campaigns')}
	</button>
</div>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
		{error}
	</div>
{/if}

{#if loading}
	<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if activeTab === 'subscribers'}
	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('newsletter.email')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('newsletter.name')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('newsletter.status')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('newsletter.joined')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each subscribers as sub (sub.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3">{sub.email}</td>
						<td class="px-4 py-3">{sub.name || '-'}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusBadge(sub.status)}">
								{sub.status}
							</span>
						</td>
						<td class="px-4 py-3">{new Date(sub.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							<button
								class="text-xs text-red-500 hover:text-red-700"
								onclick={() => deleteSubscriber(sub.id)}
							>
								{t('common.delete')}
							</button>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('newsletter.no_subscribers')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{:else}
	<div class="mb-4 flex justify-end">
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={() => { showCreateModal = true; }}
		>
			{t('newsletter.create_campaign')}
		</button>
	</div>

	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('newsletter.name')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('newsletter.subject')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('newsletter.status')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('newsletter.sent_at')}</th>
					<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each campaigns as campaign (campaign.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3">{campaign.name}</td>
						<td class="px-4 py-3">{campaign.subject}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusBadge(campaign.status)}">
								{campaign.status}
							</span>
						</td>
						<td class="px-4 py-3">{campaign.sent_at ? new Date(campaign.sent_at).toLocaleDateString() : '-'}</td>
						<td class="px-4 py-3 text-right">
							{#if campaign.status === 'draft' || campaign.status === 'ready'}
								<button
									class="text-xs text-blue-500 hover:text-blue-700"
									onclick={() => sendCampaign(campaign.id)}
								>
									{t('newsletter.send')}
								</button>
							{/if}
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('newsletter.no_campaigns')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

{#if showCreateModal}
	<Modal title={t('newsletter.create_campaign')} onclose={() => { showCreateModal = false; }}>
		<div class="space-y-4">
			<div>
				<label for="campaign-name" class="mb-1 block text-sm font-medium">{t('newsletter.name')}</label>
				<input
					id="campaign-name"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={newCampaign.name}
				/>
			</div>
			<div>
				<label for="campaign-subject" class="mb-1 block text-sm font-medium">{t('newsletter.subject')}</label>
				<input
					id="campaign-subject"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={newCampaign.subject}
				/>
			</div>
			<div>
				<label for="campaign-html" class="mb-1 block text-sm font-medium">{t('newsletter.content')}</label>
				<textarea
					id="campaign-html"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="6"
					placeholder="HTML content"
					bind:value={newCampaign.html}
				></textarea>
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
					disabled={creating || !newCampaign.name || !newCampaign.subject}
					onclick={createCampaign}
				>
					{creating ? t('common.creating') : t('common.create')}
				</button>
			</div>
		</div>
	</Modal>
{/if}
