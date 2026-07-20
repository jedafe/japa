<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { walletApi } from '$lib/api';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { Wallet, DollarSign, Download, Plus, Settings } from 'lucide-svelte';

	type ActivityRule = {
		id: string;
		activity_key: string;
		point_value: number;
		description: string;
		is_enabled: boolean;
	};

	type WalletSettings = {
		points_to_credits_rate: number;
		daily_earning_cap: number;
		enable_earning: boolean;
	};

	type UserBalance = {
		user_id: string;
		username: string;
		points: number;
		credits: number;
	};

	let rules = $state<ActivityRule[]>([]);
	let rulesLoading = $state(true);
	let showRuleModal = $state(false);
	let ruleForm = $state({ activity_key: '', point_value: 1, description: '', is_enabled: true });
	let ruleSaving = $state(false);

	let settings = $state<WalletSettings>({ points_to_credits_rate: 100, daily_earning_cap: 1000, enable_earning: true });
	let settingsLoading = $state(true);
	let settingsSaving = $state(false);

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
		{ key: 'activity_key', label: 'Activity Key', sortable: true,
			render: (v: string) => `<span class="font-mono text-xs font-medium text-foreground">${v}</span>`
		},
		{ key: 'point_value', label: 'Points', sortable: true,
			render: (v: number) => `<span class="font-semibold">${v}</span>`
		},
		{ key: 'description', label: 'Description',
			render: (v: string) => `<span class="text-sm text-muted">${v || '—'}</span>`
		},
		{ key: 'is_enabled', label: 'Status',
			render: (_: any, row: any) => {
				const label = row.is_enabled ? 'Active' : 'Inactive';
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${row.is_enabled ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border'}">${label}</span>`;
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
			const data = result.data as WalletSettings;
			if (data) settings = data;
		}
		settingsLoading = false;
	}

	async function submitRule() {
		if (!ruleForm.activity_key.trim()) return;
		ruleSaving = true;
		errorMessage = '';
		const result = await walletApi.createRule({
			activity_key: ruleForm.activity_key.trim(),
			point_value: ruleForm.point_value,
			description: ruleForm.description.trim(),
			is_enabled: ruleForm.is_enabled
		});
		if (result.ok) {
			showRuleModal = false;
			ruleForm = { activity_key: '', point_value: 1, description: '', is_enabled: true };
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

	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'rules'}
		<Card>
			<CardContent class="p-6">
				<div class="mb-4 flex items-center justify-between">
					<h3 class="text-lg font-semibold text-foreground">Activity Point Rules</h3>
					<Button onclick={() => { ruleForm = { activity_key: '', point_value: 1, description: '', is_enabled: true }; showRuleModal = true; }}>
						<Plus class="h-4 w-4 mr-2" />
						Add Rule
					</Button>
				</div>
				{#if rulesLoading}
					<p class="py-8 text-center text-sm text-muted">Loading...</p>
				{:else}
					<DataTable columns={ruleColumns} data={rules} pageSize={20} emptyMessage="No rules configured" />
				{/if}
			</CardContent>
		</Card>
	{:else if activeTab === 'settings'}
		<Card>
			<CardContent class="p-6">
				<h3 class="mb-4 text-lg font-semibold text-foreground">Wallet Settings</h3>
				{#if settingsLoading}
					<p class="py-8 text-center text-sm text-muted">Loading...</p>
				{:else}
					<div class="max-w-md space-y-4">
						<div>
							<label for="ws-rate" class="mb-1 block text-sm font-medium text-foreground-alt">Points to Credits Rate</label>
							<input id="ws-rate" type="number" min="1" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" bind:value={settings.points_to_credits_rate} />
						</div>
						<div>
							<label for="ws-cap" class="mb-1 block text-sm font-medium text-foreground-alt">Daily Earning Cap</label>
							<input id="ws-cap" type="number" min="0" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" bind:value={settings.daily_earning_cap} />
						</div>
						<div class="flex items-center gap-3">
							<input id="ws-earning" type="checkbox" bind:checked={settings.enable_earning} class="h-4 w-4 rounded border" />
							<label for="ws-earning" class="text-sm font-medium text-foreground-alt">Enable Earning</label>
						</div>
					</div>
					<div class="mt-6 flex justify-end">
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
				<h3 class="mb-4 text-lg font-semibold text-foreground">User Balance Adjustment</h3>
				<div class="mb-4 flex gap-2">
					<input type="text" class="flex-1 rounded-lg border bg-surface px-3 py-2 text-sm" placeholder="Search by username or ID..." bind:value={userSearch} onkeydown={(e) => { if (e.key === 'Enter') searchUser(); }} />
					<Button onclick={searchUser}>Search</Button>
				</div>
				{#if balanceLoading}
					<p class="py-8 text-center text-sm text-muted">Searching...</p>
				{:else if userBalance}
					<div class="mb-4 grid gap-3 sm:grid-cols-3">
						<div class="rounded-lg border bg-surface p-3"><p class="text-xs font-medium text-muted">User</p><p class="text-sm font-semibold">{userBalance.username}</p></div>
						<div class="rounded-lg border bg-surface p-3"><p class="text-xs font-medium text-muted">Points</p><p class="text-lg font-bold text-primary-600">{userBalance.points}</p></div>
						<div class="rounded-lg border bg-surface p-3"><p class="text-xs font-medium text-muted">Credits</p><p class="text-lg font-bold text-success-600">{userBalance.credits}</p></div>
					</div>
					<div class="grid gap-3 sm:grid-cols-3">
						<div>
							<label for="adj-amount" class="mb-1 block text-sm font-medium">Amount</label>
							<input id="adj-amount" type="number" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" bind:value={adjustForm.amount} />
						</div>
						<div>
							<label class="mb-1 block text-sm font-medium">Type</label>
							<Select options={adjTypeOptions} bind:value={adjustForm.type} />
						</div>
						<div>
							<label for="adj-reason" class="mb-1 block text-sm font-medium">Reason</label>
							<input id="adj-reason" type="text" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" placeholder="Admin adjustment" bind:value={adjustForm.reason} />
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

<Modal bind:open={showRuleModal} title="Add Activity Rule">
	<div class="space-y-4">
		<div>
			<label for="rule-key" class="mb-1 block text-sm font-medium">Activity Key</label>
			<input id="rule-key" type="text" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" placeholder="post.created" bind:value={ruleForm.activity_key} />
		</div>
		<div>
			<label for="rule-points" class="mb-1 block text-sm font-medium">Points</label>
			<input id="rule-points" type="number" min="1" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" bind:value={ruleForm.point_value} />
		</div>
		<div>
			<label for="rule-desc" class="mb-1 block text-sm font-medium">Description</label>
			<input id="rule-desc" type="text" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" placeholder="Awarded for creating a post" bind:value={ruleForm.description} />
		</div>
		<div class="flex items-center gap-2">
			<input id="rule-active" type="checkbox" bind:checked={ruleForm.is_enabled} class="h-4 w-4 rounded border" />
			<label for="rule-active" class="text-sm font-medium">Active</label>
		</div>
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" onclick={() => showRuleModal = false}>Cancel</Button>
			<Button onclick={submitRule} disabled={ruleSaving}>{ruleSaving ? 'Saving...' : 'Add Rule'}</Button>
		</div>
	</div>
</Modal>
