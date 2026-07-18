<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { walletApi } from '$lib/api';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { Wallet, DollarSign, ArrowUpRight, ArrowDownLeft, Download, Plus, Settings } from 'lucide-svelte';

	type ActivityRule = {
		id: string;
		event_type: string;
		points_value: number;
		description: string;
		is_active: boolean;
	};

	type WalletSettings = {
		points_to_credit_ratio: number;
		daily_earning_cap: number;
		enable_earning: boolean;
	};

	type UserBalance = {
		user_id: string;
		username: string;
		points: number;
		credits: number;
	};

	// Activity Rules
	let rules = $state<ActivityRule[]>([]);
	let rulesLoading = $state(true);
	let showRuleModal = $state(false);
	let ruleForm = $state({ event_type: '', points_value: 0, description: '', is_active: true });
	let ruleSaving = $state(false);

	// Wallet Settings
	let settings = $state<WalletSettings>({ points_to_credit_ratio: 1, daily_earning_cap: 1000, enable_earning: true });
	let settingsLoading = $state(true);
	let settingsSaving = $state(false);

	// User Balance Adjustment
	let userSearch = $state('');
	let userBalance = $state<UserBalance | null>(null);
	let balanceLoading = $state(false);
	let adjustForm = $state({ amount: 0, reason: '', type: 'points' as 'points' | 'credits' });
	let adjustSaving = $state(false);

	let errorMessage = $state('');
	let successMessage = $state('');
	let activeTab = $state('rules');

	const tabs = [
		{ value: 'rules', label: 'Activity Rules', icon: Settings },
		{ value: 'settings', label: 'Wallet Settings', icon: Wallet },
		{ value: 'adjust', label: 'Balance Adjustment', icon: DollarSign }
	];

	const adjTypeOptions = [
		{ value: 'points', label: 'Points' },
		{ value: 'credits', label: 'Credits' }
	];

	const ruleColumns = [
		{
			key: 'event_type',
			label: 'Event Type',
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs font-medium text-foreground">${value}</span>`
		},
		{
			key: 'points_value',
			label: 'Points',
			sortable: true,
			render: (value: number) => `<span class="font-semibold">${value}</span>`
		},
		{
			key: 'description',
			label: 'Description',
			render: (value: string) => `<span class="text-sm text-muted">${value || '—'}</span>`
		},
		{
			key: 'is_active',
			label: 'Status',
			render: (_: any, row: any) => {
				const variant = row.is_active ? 'success' : 'secondary';
				const label = row.is_active ? 'Active' : 'Inactive';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${row.is_active ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border'}">${label}</span>`;
			}
		}
	];

	async function loadRules() {
		rulesLoading = true;
		const result = await walletApi.getRules();
		if (result.ok) {
			const data = result.data as { items: ActivityRule[] };
			rules = data.items ?? (Array.isArray(result.data) ? result.data as ActivityRule[] : []);
		}
		rulesLoading = false;
	}

	async function loadSettings() {
		settingsLoading = true;
		const result = await walletApi.getSettings();
		if (result.ok) {
			settings = result.data as WalletSettings;
		}
		settingsLoading = false;
	}

	async function submitRule() {
		if (!ruleForm.event_type.trim()) return;
		ruleSaving = true;
		errorMessage = '';
		const result = await walletApi.createRule({
			event_type: ruleForm.event_type.trim(),
			points_value: ruleForm.points_value,
			description: ruleForm.description.trim(),
			is_active: ruleForm.is_active
		});
		if (result.ok) {
			showRuleModal = false;
			ruleForm = { event_type: '', points_value: 0, description: '', is_active: true };
			loadRules();
		} else {
			errorMessage = result.error;
		}
		ruleSaving = false;
	}

	async function saveSettings() {
		settingsSaving = true;
		errorMessage = '';
		successMessage = '';
		const result = await walletApi.updateSettings(settings);
		if (result.ok) {
			successMessage = 'Settings saved successfully';
		} else {
			errorMessage = result.error;
		}
		settingsSaving = false;
	}

	async function searchUser() {
		if (!userSearch.trim()) return;
		balanceLoading = true;
		userBalance = null;
		errorMessage = '';
		const result = await walletApi.getUserBalance(userSearch.trim());
		if (result.ok) {
			userBalance = result.data as UserBalance;
		} else {
			errorMessage = result.error;
		}
		balanceLoading = false;
	}

	async function submitAdjust() {
		if (!userBalance || adjustForm.amount === 0 || !adjustForm.reason.trim()) return;
		adjustSaving = true;
		errorMessage = '';
		successMessage = '';
		const result = await walletApi.adjustBalance({
			user_id: userBalance.user_id,
			amount: adjustForm.amount,
			reason: adjustForm.reason.trim(),
			type: adjustForm.type
		});
		if (result.ok) {
			successMessage = `Adjusted ${adjustForm.type} by ${adjustForm.amount}`;
			adjustForm = { amount: 0, reason: '', type: 'points' };
			searchUser();
		} else {
			errorMessage = result.error;
		}
		adjustSaving = false;
	}

	$effect(() => {
		loadRules();
		loadSettings();
	});
</script>

<svelte:head>
	<title>Wallet Management — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Wallet Management"
	description="Configure activity point rules, wallet settings, and user balance adjustments"
	actions={[
		{ label: 'Export', icon: Download, variant: 'outline' }
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="rounded-lg border border-success bg-success-50 px-4 py-3 text-sm text-success-700">
			{successMessage}
		</div>
	{/if}

	<!-- Balance Overview Cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('app.wallet.points_to_credit_ratio')}</p>
						<p class="text-2xl font-bold text-foreground">{settings.points_to_credit_ratio}</p>
					</div>
					<div class="rounded-lg bg-primary-100 p-3">
						<DollarSign class="h-6 w-6 text-primary-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('app.wallet.daily_earning_cap')}</p>
						<p class="text-2xl font-bold text-foreground">{settings.daily_earning_cap}</p>
					</div>
					<div class="rounded-lg bg-warning/15 p-3">
						<ArrowDownLeft class="h-6 w-6 text-warning-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('app.wallet.earning_status')}</p>
						<p class="text-2xl font-bold {settings.enable_earning ? 'text-success-600' : 'text-muted'}">
							{settings.enable_earning ? 'Enabled' : 'Disabled'}
						</p>
					</div>
					<div class="rounded-lg {settings.enable_earning ? 'bg-success/15' : 'bg-surface'} p-3">
						<Wallet class="h-6 w-6 {settings.enable_earning ? 'text-success-600' : 'text-muted'}" />
					</div>
				</div>
			</CardContent>
		</Card>
	</div>

	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'rules'}
		<Card>
			<CardContent class="p-6">
				<div class="mb-4 flex items-center justify-between">
					<h3 class="text-lg font-semibold text-foreground">{t('app.wallet.activity_point_rules')}</h3>
					<Button onclick={() => { ruleForm = { event_type: '', points_value: 0, description: '', is_active: true }; showRuleModal = true; }}>
						<Plus class="h-4 w-4 mr-2" />
						Add Rule
					</Button>
				</div>

				{#if rulesLoading}
					<p class="py-8 text-center text-sm text-muted">{t('app.wallet.loading')}</p>
				{:else}
					<DataTable
						columns={ruleColumns}
						data={rules}
						pageSize={20}
						emptyMessage="No rules configured"
					/>
				{/if}
			</CardContent>
		</Card>
	{:else if activeTab === 'settings'}
		<Card>
			<CardContent class="p-6">
				<h3 class="mb-4 text-lg font-semibold text-foreground">{t('app.wallet.wallet_settings')}</h3>

				{#if settingsLoading}
					<p class="py-8 text-center text-sm text-muted">{t('app.wallet.loading')}</p>
				{:else}
					<div class="grid gap-4 sm:grid-cols-3">
						<div>
							<label for="ws-ratio" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.points_to_credit_ratio_2')}</label>
							<input id="ws-ratio" type="number" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={settings.points_to_credit_ratio} min="0" step="0.01" />
						</div>
						<div>
							<label for="ws-cap" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.daily_earning_cap')}</label>
							<input id="ws-cap" type="number" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={settings.daily_earning_cap} min="0" />
						</div>
						<div class="flex items-end gap-2">
							<input id="ws-earning" type="checkbox" bind:checked={settings.enable_earning} class="h-4 w-4 rounded border text-primary-600 focus:ring-primary" />
							<label for="ws-earning" class="text-sm font-medium text-foreground-alt">{t('app.wallet.enable_earning')}</label>
						</div>
					</div>
					<div class="mt-4 flex justify-end">
						<Button onclick={saveSettings} disabled={settingsSaving}>
							{settingsSaving ? 'Saving...' : 'Save Settings'}
						</Button>
					</div>
				{/if}
			</CardContent>
		</Card>
	{:else if activeTab === 'adjust'}
		<Card>
			<CardContent class="p-6">
				<h3 class="mb-4 text-lg font-semibold text-foreground">{t('app.wallet.user_balance_adjustment')}</h3>

				<!-- Search -->
				<div class="mb-4 flex gap-2">
					<input
						type="text"
						class="flex-1 rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
						placeholder="Search by username or user ID..."
						bind:value={userSearch}
						onkeydown={(e) => { if (e.key === 'Enter') searchUser(); }}
					/>
					<Button onclick={searchUser}>{t('app.wallet.search')}</Button>
				</div>

				{#if balanceLoading}
					<p class="py-8 text-center text-sm text-muted">{t('app.wallet.searching')}</p>
				{:else if userBalance}
					<!-- Current Balance -->
					<div class="mb-4 grid gap-3 sm:grid-cols-3">
						<div class="rounded-lg border border bg-surface p-3">
							<p class="text-xs font-medium text-muted">{t('app.wallet.user')}</p>
							<p class="text-sm font-semibold text-foreground">{userBalance.username}</p>
						</div>
						<div class="rounded-lg border border bg-surface p-3">
							<p class="text-xs font-medium text-muted">{t('app.wallet.points')}</p>
							<p class="text-lg font-bold text-primary-600">{userBalance.points}</p>
						</div>
						<div class="rounded-lg border border bg-surface p-3">
							<p class="text-xs font-medium text-muted">{t('app.wallet.credits')}</p>
							<p class="text-lg font-bold text-success-600">{userBalance.credits}</p>
						</div>
					</div>

					<!-- Adjustment Form -->
					<div class="grid gap-3 sm:grid-cols-3">
						<div>
							<label for="adj-amount" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.amount_credit_debit')}</label>
							<input id="adj-amount" type="number" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={adjustForm.amount} />
						</div>
						<div>
							<label for="adj-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.type')}</label>
							<Select options={adjTypeOptions} bind:value={adjustForm.type} />
						</div>
						<div>
							<label for="adj-reason" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.reason')}</label>
							<input id="adj-reason" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder="Admin adjustment" bind:value={adjustForm.reason} />
						</div>
					</div>
					<div class="mt-4 flex justify-end">
						<Button onclick={submitAdjust} disabled={adjustSaving || adjustForm.amount === 0 || !adjustForm.reason.trim()}>
							{adjustSaving ? 'Adjusting...' : 'Adjust Balance'}
						</Button>
					</div>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Add Rule Modal -->
<Modal bind:open={showRuleModal} title="Add Activity Rule">
	<div class="space-y-4">
		<div>
			<label for="rule-event" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.event_type')}</label>
			<input id="rule-event" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder="post.created" bind:value={ruleForm.event_type} />
		</div>
		<div>
			<label for="rule-points" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.points_value')}</label>
			<input id="rule-points" type="number" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" bind:value={ruleForm.points_value} />
		</div>
		<div>
			<label for="rule-desc" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.description')}</label>
			<input id="rule-desc" type="text" class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2" placeholder="Awarded for creating a post" bind:value={ruleForm.description} />
		</div>
		<div class="flex items-center gap-2">
			<input id="rule-active" type="checkbox" bind:checked={ruleForm.is_active} class="h-4 w-4 rounded border text-primary-600 focus:ring-primary" />
			<label for="rule-active" class="text-sm font-medium text-foreground-alt">{t('app.wallet.active')}</label>
		</div>
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" onclick={() => showRuleModal = false}>{t('app.wallet.cancel')}</Button>
			<Button onclick={submitRule} disabled={ruleSaving}>
				{ruleSaving ? 'Saving...' : 'Add Rule'}
			</Button>
		</div>
	</div>
</Modal>
