<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Gift, Users, Trash2 } from 'lucide-svelte';

	// Backend contract: internal/referral/service.go Program struct.
	type ReferralProgram = {
		id: string;
		tenant_id: string;
		referrer_reward_cents: number;
		referee_reward_cents: number;
		max_referrals: number;
		cookie_days: number;
		is_active: boolean;
		description: string;
		created_at: string;
		updated_at: string;
	};

	// Backend contract: internal/referral/service.go Referral struct.
	type Referral = {
		id: string;
		tenant_id: string;
		referrer_id: string;
		referrer_username?: string;
		referrer_display_name?: string;
		referee_id: string | null;
		referee_username?: string;
		referee_display_name?: string;
		referral_code: string;
		status: string;
		reward_paid_at: string | null;
		created_at: string;
	};

	let program = $state<ReferralProgram | null>(null);
	let referrals = $state<Referral[]>([]);
	let loading = $state(true);
	let error = $state('');
	let saving = $state(false);
	let activeTab = $state('program');
	let searchQuery = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	const tabs = $derived([
		{ value: 'program', label: t('referral.program_settings'), icon: Gift },
		{ value: 'referrals', label: t('referral.recent_referrals'), icon: Users, count: referrals.length }
	]);

	const referralColumns = $derived([
		{
			key: 'referral_code',
			label: t('referral.code'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs font-medium text-foreground">${value}</span>`
		},
		{
			key: 'referrer_display_name',
			label: t('referral.referrer'),
			sortable: true,
			render: (v: string, row: Referral) => `<span class="font-medium text-foreground">${v || row.referrer_username || 'anonymous'}</span>`
		},
		{
			key: 'referee_display_name',
			label: t('referral.referred'),
			sortable: true,
			render: (v: string, row: Referral) => `<span class="text-foreground-alt">${v || row.referee_username || (row.referee_id ? row.referee_id.slice(0, 8) + '...' : '—')}</span>`
		},
		{
			key: 'status',
			label: t('referral.status'),
			sortable: true,
			render: (value: string) => {
				const classes: Record<string, string> = {
					qualified: 'bg-success/15 text-success border-success',
					rewarded: 'bg-success/15 text-success border-success',
					pending: 'bg-warning/15 text-warning border-warning',
					expired: 'bg-danger/15 text-danger border-danger'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-surface text-foreground border'}">${value}</span>`;
			}
		},
		{
			key: 'reward_paid_at',
			label: t('referral.reward'),
			render: (value: string | null) => {
				if (value) {
					return '<span class="text-sm font-medium text-success">Granted</span>';
				}
				return '<span class="text-sm text-muted">Pending</span>';
			}
		},
		{
			key: 'created_at',
			label: t('referral.date'),
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
			const [programResult, referralsResult] = await Promise.all([
				api<ReferralProgram>('/admin/referral/program'),
				api<Referral[]>('/admin/referral/referrals')
			]);
			if (programResult.ok) {
				program = programResult.data;
			} else {
				error = programResult.error;
			}
			if (referralsResult.ok) {
				referrals = referralsResult.data ?? [];
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function updateProgram() {
		if (!program) return;
		saving = true;
		error = '';
		try {
			const result = await api<ReferralProgram>('/admin/referral/program', {
				method: 'PUT',
				body: {
					referrer_reward_cents: program.referrer_reward_cents,
					referee_reward_cents: program.referee_reward_cents,
					max_referrals: program.max_referrals,
					cookie_days: program.cookie_days,
					is_active: program.is_active,
					description: program.description
				}
			});
			if (result.ok) {
				program = result.data;
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		error = '';
		try {
			for (const id of selectedIds) {
				await api(`/admin/referral/referrals/${id}`, { method: 'DELETE' });
			}
			selectedIds = new Set();
			await load();
		} catch {
			error = t('common.error');
		}
		bulkDeleting = false;
	}
</script>

<svelte:head>
	<title>{t('referral.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('referral.title')}
	description={t('referral.description')}
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
	{:else if activeTab === 'program'}
		<!-- Program Settings -->
		{#if program}
			<Card>
				<CardContent class="p-6">
					<h3 class="mb-4 text-lg font-semibold text-foreground">{t('referral.program_settings')}</h3>
					<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
						<div>
							<label for="prog-referrer-reward" class="mb-1 block text-sm font-medium text-foreground-alt">{t('referral.referrer_reward')}</label>
							<input
								id="prog-referrer-reward"
								type="number"
								min="0"
								class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={program.referrer_reward_cents}
							/>
							<p class="mt-1 text-xs text-muted">{t('referral.cents_hint')}</p>
						</div>
						<div>
							<label for="prog-referee-reward" class="mb-1 block text-sm font-medium text-foreground-alt">{t('referral.referee_reward')}</label>
							<input
								id="prog-referee-reward"
								type="number"
								min="0"
								class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={program.referee_reward_cents}
							/>
							<p class="mt-1 text-xs text-muted">{t('referral.cents_hint')}</p>
						</div>
						<div>
							<label for="prog-max-referrals" class="mb-1 block text-sm font-medium text-foreground-alt">{t('referral.max_referrals')}</label>
							<input
								id="prog-max-referrals"
								type="number"
								min="0"
								class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={program.max_referrals}
							/>
							<p class="mt-1 text-xs text-muted">{t('referral.max_referrals_hint')}</p>
						</div>
						<div>
							<label for="prog-cookie-days" class="mb-1 block text-sm font-medium text-foreground-alt">{t('referral.cookie_days')}</label>
							<input
								id="prog-cookie-days"
								type="number"
								min="0"
								class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={program.cookie_days}
							/>
						</div>
						<div>
							<label for="prog-description" class="mb-1 block text-sm font-medium text-foreground-alt">{t('referral.description_label')}</label>
							<input
								id="prog-description"
								type="text"
								class="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
								bind:value={program.description}
							/>
						</div>
						<div class="flex items-end">
							<label class="flex items-center gap-2">
								<input
									type="checkbox"
									bind:checked={program.is_active}
									class="h-4 w-4 rounded border-border text-primary-600 focus:ring-primary"
								/>
								<span class="text-sm font-medium text-foreground-alt">{t('referral.active')}</span>
							</label>
						</div>
					</div>
					<div class="mt-4">
						<Button onclick={updateProgram} disabled={saving}>
							{saving ? t('common.saving') : t('common.save')}
						</Button>
					</div>
				</CardContent>
			</Card>
		{/if}
	{:else if activeTab === 'referrals'}
		<!-- Referrals List -->
		<Card>
			<CardContent class="p-0">
				<DataTable
					columns={referralColumns}
					data={referrals}
					pageSize={20}
					emptyMessage={t('referral.no_referrals')}
					searchable
					onsearch={(q) => { searchQuery = q; }}
					selectable
					bind:selectedIds
				>
					{#snippet bulkActions()}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
							<Trash2 class="h-3.5 w-3.5 mr-1" />
							{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.delete_selected', 'Delete Selected')} ({selectedIds.size})
						</Button>
					{/snippet}
				</DataTable>
			</CardContent>
		</Card>
	{/if}
</div>
