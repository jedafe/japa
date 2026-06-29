<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Campaign = {
		id: string;
		name: string;
		status: 'active' | 'paused' | 'draft' | 'completed';
		impressions: number;
		clicks: number;
		conversions: number;
		budget: number;
		spent: number;
		bid_strategy: string;
		creative: {
			title: string;
			body: string;
			image_url: string;
			destination_url: string;
		};
		targeting: {
			geo: string[];
			interests: string[];
		};
		start_date: string;
		end_date: string;
		created_at: string;
	};

	let campaign = $state<Campaign | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');
	let successMessage = $state('');

	// Edit form
	let editName = $state('');
	let editTitle = $state('');
	let editBody = $state('');
	let editImageUrl = $state('');
	let editDestinationUrl = $state('');
	let editBudget = $state(0);
	let saving = $state(false);

	const campaignId = $derived($page.params.id);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Campaign>(`/admin/advertising/campaigns/${campaignId}`);
			if (result.ok) {
				campaign = result.data;
				editName = campaign.name;
				editTitle = campaign.creative?.title ?? '';
				editBody = campaign.creative?.body ?? '';
				editImageUrl = campaign.creative?.image_url ?? '';
				editDestinationUrl = campaign.creative?.destination_url ?? '';
				editBudget = campaign.budget;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('ads.load_error');
		} finally {
			loading = false;
		}
	}

	async function saveCampaign() {
		if (!campaign || !editName.trim()) return;
		saving = true;
		errorMessage = '';
		successMessage = '';
		try {
			const result = await api(`/admin/advertising/campaigns/${campaign.id}`, {
				method: 'PATCH',
				body: {
					name: editName.trim(),
					creative: {
						title: editTitle.trim() || null,
						body: editBody.trim() || null,
						image_url: editImageUrl.trim() || null,
						destination_url: editDestinationUrl.trim() || null
					},
					budget: editBudget
				}
			});
			if (result.ok) {
				successMessage = t('ads.updated');
				campaign.name = editName.trim();
				campaign.creative = {
					title: editTitle.trim(),
					body: editBody.trim(),
					image_url: editImageUrl.trim(),
					destination_url: editDestinationUrl.trim()
				};
				campaign.budget = editBudget;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('ads.update_error');
		} finally {
			saving = false;
		}
	}

	async function toggleStatus() {
		if (!campaign) return;
		const endpoint = campaign.status === 'active'
			? `/admin/advertising/campaigns/${campaign.id}/pause`
			: `/admin/advertising/campaigns/${campaign.id}/activate`;
		try {
			const result = await api(endpoint, { method: 'POST' });
			if (result.ok) {
				campaign.status = campaign.status === 'active' ? 'paused' : 'active';
			}
		} catch {
			errorMessage = t('ads.status_error');
		}
	}

	function statusColor(status: string) {
		switch (status) {
			case 'active': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'paused': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'completed': return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
			default: return 'bg-blue-500/15 text-blue-700 dark:text-blue-400';
		}
	}

	onMount(() => {
		load();
	});
</script>

<svelte:head>
	<title>{campaign?.name ?? 'Campaign'} — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4">
	<a href="/ads" class="text-sm hover:underline" style="color: var(--color-muted)">&larr; {t('ads.back')}</a>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
{#if successMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-success); background-color: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)">
		{successMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if campaign}
	<h1 class="mb-1 text-2xl font-bold tracking-tight">{campaign.name}</h1>
	<p class="mb-6 text-sm" style="color: var(--color-muted)">
		{t('ads.status_label')}
		<span class="inline-block rounded-full px-2 py-0.5 text-xs {statusColor(campaign.status)}">
			{campaign.status}
		</span>
		&middot; {campaign.bid_strategy.toUpperCase()}
	</p>

	<!-- Stats Cards -->
	<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.impressions')}</p>
			<p class="mt-1 text-2xl font-bold">{campaign.impressions.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.clicks')}</p>
			<p class="mt-1 text-2xl font-bold">{campaign.clicks.toLocaleString()}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.ctr')}</p>
			<p class="mt-1 text-2xl font-bold">
				{campaign.impressions > 0 ? ((campaign.clicks / campaign.impressions) * 100).toFixed(2) : '0.00'}%
			</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.budget_spent')}</p>
			<p class="mt-1 text-2xl font-bold">${campaign.spent?.toLocaleString() ?? '0'}</p>
			<p class="text-xs" style="color: var(--color-muted)">of ${campaign.budget.toLocaleString()}</p>
		</div>
	</div>

	<div class="grid gap-6 lg:grid-cols-3">
		<!-- Edit form -->
		<div class="lg:col-span-2">
			<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('ads.details')}</h2>
				<div class="space-y-4">
					<div>
						<label for="edit-name" class="mb-1 block text-sm font-medium">{t('ads.fields.name')}</label>
						<input
							id="edit-name"
							type="text"
							class="w-full rounded-md border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background-color: var(--color-background)"
							bind:value={editName}
						/>
					</div>
					<div class="grid gap-4 sm:grid-cols-2">
						<div>
							<label for="edit-title" class="mb-1 block text-sm font-medium">{t('ads.fields.headline')}</label>
							<input
								id="edit-title"
								type="text"
								class="w-full rounded-md border px-3 py-2 text-sm"
								style="border-color: var(--color-border); background-color: var(--color-background)"
								bind:value={editTitle}
							/>
						</div>
						<div>
							<label for="edit-image" class="mb-1 block text-sm font-medium">{t('ads.fields.image_url')}</label>
							<input
								id="edit-image"
								type="url"
								class="w-full rounded-md border px-3 py-2 text-sm"
								style="border-color: var(--color-border); background-color: var(--color-background)"
								bind:value={editImageUrl}
							/>
						</div>
					</div>
					<div>
						<label for="edit-body" class="mb-1 block text-sm font-medium">{t('ads.fields.body')}</label>
						<textarea
							id="edit-body"
							class="w-full rounded-md border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background-color: var(--color-background)"
							rows="3"
							bind:value={editBody}
						></textarea>
					</div>
					<div class="grid gap-4 sm:grid-cols-2">
						<div>
							<label for="edit-url" class="mb-1 block text-sm font-medium">{t('ads.fields.dest_url')}</label>
							<input
								id="edit-url"
								type="url"
								class="w-full rounded-md border px-3 py-2 text-sm"
								style="border-color: var(--color-border); background-color: var(--color-background)"
								bind:value={editDestinationUrl}
							/>
						</div>
						<div>
							<label for="edit-budget" class="mb-1 block text-sm font-medium">{t('ads.fields.budget')}</label>
							<input
								id="edit-budget"
								type="number"
								class="w-full rounded-md border px-3 py-2 text-sm"
								style="border-color: var(--color-border); background-color: var(--color-background)"
								min="1"
								bind:value={editBudget}
							/>
						</div>
					</div>
					<div class="flex items-center gap-3 pt-2">
						<button
							class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
							style="background-color: var(--color-primary)"
							disabled={saving || !editName.trim()}
							onclick={saveCampaign}
						>
							{saving ? t('common.saving') : t('common.save')}
						</button>
					</div>
				</div>
			</div>
		</div>

		<!-- Sidebar actions -->
		<div class="space-y-4">
			<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<h3 class="mb-3 text-sm font-semibold" style="color: var(--color-muted)">{t('ads.status')}</h3>
				<button
					class="w-full rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="border: 1px solid var(--color-border)"
					onclick={toggleStatus}
				>
					{campaign.status === 'active' ? t('ads.pause_campaign') : t('ads.resume_campaign')}
				</button>
			</div>

			<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<h3 class="mb-3 text-sm font-semibold" style="color: var(--color-muted)">{t('ads.sections.schedule')}</h3>
				<p class="text-sm">{t('ads.schedule_start').replace('{date}', campaign.start_date ?? 'Not set')}</p>
				<p class="text-sm">{t('ads.schedule_end').replace('{date}', campaign.end_date ?? 'Ongoing')}</p>
			</div>
		</div>
	</div>
{/if}
