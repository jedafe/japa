<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { Gift, Users } from 'lucide-svelte';

	type ReferralProgram = {
		id: string;
		name: string;
		reward_type: string;
		reward_value: number;
		is_active: boolean;
		created_at: string;
	};

	type Referral = {
		id: string;
		referrer_user_id: string;
		referrer_username: string;
		referred_user_id: string;
		referred_username: string;
		status: string;
		reward_granted: boolean;
		created_at: string;
	};

	let program = $state<ReferralProgram | null>(null);
	let referrals = $state<Referral[]>([]);
	let loading = $state(true);
	let error = $state('');
	let saving = $state(false);
	let activeTab = $state('program');

	const tabs = $derived([
		{ value: 'program', label: 'Program Settings', icon: Gift },
		{ value: 'referrals', label: 'Recent Referrals', icon: Users, count: referrals.length }
	]);

	const referralColumns = [
		{
			key: 'referrer_username',
			label: 'Referrer',
			sortable: true,
			render: (value: string) => `<span class="font-medium text-gray-900">${value}</span>`
		},
		{
			key: 'referred_username',
			label: 'Referred',
			sortable: true,
			render: (value: string) => `<span class="text-gray-700">${value}</span>`
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			render: (value: string) => {
				const classes: Record<string, string> = {
					completed: 'bg-success-100 text-success-800 border-success-200',
					pending: 'bg-warning-100 text-warning-800 border-warning-200',
					cancelled: 'bg-danger-100 text-danger-800 border-danger-200'
				};
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${classes[value] || 'bg-gray-100 text-gray-800 border-gray-200'}">${value}</span>`;
			}
		},
		{
			key: 'reward_granted',
			label: 'Reward',
			render: (_: any, row: any) => {
				if (row.reward_granted) {
					return '<span class="text-sm font-medium text-success-600">Granted</span>';
				}
				return '<span class="text-sm text-gray-400">Pending</span>';
			}
		},
		{
			key: 'created_at',
			label: 'Date',
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
			const [programResult, referralsResult] = await Promise.all([
				api<ReferralProgram>('/admin/referral/program'),
				api<Referral[]>('/admin/referral/referrals')
			]);
			if (programResult.ok) {
				program = programResult.data;
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
		try {
			await api('/admin/referral/program', {
				method: 'PUT',
				body: program
			});
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
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
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	{#if loading}
		<p class="py-8 text-center text-sm text-gray-500">{t('common.loading')}</p>
	{:else if activeTab === 'program'}
		<!-- Program Settings -->
		{#if program}
			<Card>
				<CardContent class="p-6">
					<h3 class="mb-4 text-lg font-semibold text-gray-900">{t('referral.program_settings')}</h3>
					<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
						<div>
							<label for="prog-name" class="mb-1 block text-sm font-medium text-gray-700">{t('referral.name')}</label>
							<input
								id="prog-name"
								type="text"
								class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
								bind:value={program.name}
							/>
						</div>
						<div>
						<label for="prog-reward-type" class="mb-1 block text-sm font-medium text-gray-700">{t('referral.reward_type')}</label>
						<Select
							options={[
								{ value: 'credits', label: t('referral.credits') },
								{ value: 'discount', label: t('referral.discount') },
								{ value: 'points', label: t('referral.points') }
							]}
							bind:value={program.reward_type}
						/>
						</div>
						<div>
							<label for="prog-reward-value" class="mb-1 block text-sm font-medium text-gray-700">{t('referral.reward_value')}</label>
							<input
								id="prog-reward-value"
								type="number"
								min="0"
								class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
								bind:value={program.reward_value}
							/>
						</div>
						<div class="flex items-end">
							<label class="flex items-center gap-2">
								<input
									type="checkbox"
									bind:checked={program.is_active}
									class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
								/>
								<span class="text-sm font-medium text-gray-700">{t('referral.active')}</span>
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
				/>
			</CardContent>
		</Card>
	{/if}
</div>
