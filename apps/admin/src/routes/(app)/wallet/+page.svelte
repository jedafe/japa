<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { walletApi, userManagementApi } from '$lib/api';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { Wallet, DollarSign, Download, Plus, Settings, Trash2, X } from 'lucide-svelte';
	import RuleActions from './rule-actions.svelte';

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
	let searchQuery = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);
	let showRuleModal = $state(false);
	let editingRule = $state<ActivityRule | null>(null);
	let ruleForm = $state({ activity_key: '', point_value: 1, description: '', is_enabled: true });
	let ruleSaving = $state(false);

	let settings = $state<WalletSettings>({ points_to_credits_rate: 100, daily_earning_cap: 1000, enable_earning: true });
	let settingsLoading = $state(true);
	let settingsSaving = $state(false);
	let settingsError = $state('');

	let userSearch = $state('');
	let userSuggestions = $state<Array<{ id: string; username: string; email: string }>>([]);
	let userSearchLoading = $state(false);
	let userSearchTimer = $state<ReturnType<typeof setTimeout> | null>(null);
	let showSuggestions = $state(false);
	let userBalance = $state<UserBalance | null>(null);
	let balanceLoading = $state(false);
	let adjustForm = $state({ amount: 0, reason: '', ledger_type: 'points' as 'points' | 'credits' });
	let adjustSaving = $state(false);

	let errorMessage = $state('');
	let successMessage = $state('');
	let activeTab = $state('rules');

	const tabs = [
		{ value: 'rules', label: t('app.wallet.activity_point_rules', 'Activity Rules'), icon: Settings },
		{ value: 'settings', label: t('app.wallet.wallet_settings', 'Wallet Settings'), icon: Wallet },
		{ value: 'adjust', label: t('app.wallet.user_balance_adjustment', 'Balance Adjustment'), icon: DollarSign }
	];

	const adjTypeOptions = [
		{ value: 'points', label: t('app.wallet.points', 'Points') },
		{ value: 'credits', label: t('app.wallet.credits', 'Credits') }
	];

	// Predefined activity keys from Section 28 / internal/wallet/wallet.go.
	const activityKeyOptions = [
		{ value: 'post_created', label: 'post_created — Creating a post' },
		{ value: 'comment_created', label: 'comment_created — Creating a comment' },
		{ value: 'reaction_added', label: 'reaction_added — Adding a reaction' },
		{ value: 'content_flagged_removed', label: 'content_flagged_removed — Content flagged/removed' },
		{ value: 'spam_detected', label: 'spam_detected — Spam detected' },
		{ value: 'referral_completed', label: 'referral_completed — Referral completed' },
		{ value: 'daily_login', label: 'daily_login — Daily login' },
		{ value: 'profile_completed', label: 'profile_completed — Profile completed' },
		{ value: '_custom', label: '— Custom key —' }
	];
	let activityKeyMode = $state<'select' | 'custom'>('select');

	const ruleColumns = [
		{ key: 'activity_key', label: t('app.wallet.col_activity_key', 'Activity Key'), sortable: true,
			render: (v: string) => `<span class="font-mono text-xs font-medium text-foreground">${v}</span>`
		},
		{ key: 'point_value', label: t('app.wallet.col_points', 'Points'), sortable: true,
			render: (v: number) => `<span class="font-semibold">${v}</span>`
		},
		{ key: 'description', label: t('app.wallet.col_description', 'Description'),
			render: (v: string) => `<span class="text-sm text-muted">${v || '—'}</span>`
		},
		{ key: 'is_enabled', label: t('app.wallet.col_status', 'Status'),
			render: (_: any, row: any) => {
				const label = row.is_enabled ? t('app.wallet.active', 'Active') : t('common.inactive', 'Inactive');
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${row.is_enabled ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border'}">${label}</span>`;
			}
		},
		{ key: '_actions', label: t('common.actions', 'Actions'),
			component: RuleActions,
			componentProps: {
				onEdit: (row: any) => openEditRuleModal(row),
				onToggle: (row: any) => toggleRule(row),
				onDelete: (row: any) => deleteRule(row)
			}
		}
	];

	async function loadRules() {
		rulesLoading = true;
		const result = await walletApi.getRules();
		if (result.ok) {
			const data = result.data;
			let items: ActivityRule[] = Array.isArray(data) ? data as ActivityRule[] : (data as any)?.items ?? [];
			if (searchQuery) {
				const q = searchQuery.toLowerCase();
				items = items.filter((r: any) => r.activity_key?.toLowerCase().includes(q) || r.description?.toLowerCase().includes(q));
			}
			rules = items;
		}
		rulesLoading = false;
	}

	async function loadSettings() {
		settingsLoading = true;
		settingsError = '';
		const result = await walletApi.getSettings();
		if (result.ok) {
			const data = result.data as WalletSettings;
			if (data) settings = data;
		} else {
			// 404 = no settings row yet — use defaults (not an error to the user).
			if (!result.error.includes('not_found')) {
				settingsError = result.error;
			}
		}
		settingsLoading = false;
	}

	function openAddRuleModal() {
		editingRule = null;
		ruleForm = { activity_key: '', point_value: 1, description: '', is_enabled: true };
		activityKeyMode = 'select';
		showRuleModal = true;
	}

	function openEditRuleModal(rule: ActivityRule) {
		editingRule = rule;
		const isKnown = activityKeyOptions.some(o => o.value === rule.activity_key && o.value !== '_custom');
		activityKeyMode = isKnown ? 'select' : 'custom';
		ruleForm = {
			activity_key: rule.activity_key,
			point_value: rule.point_value,
			description: rule.description || '',
			is_enabled: rule.is_enabled
		};
		showRuleModal = true;
	}

	function handleActivityKeySelect(val: string) {
		if (val === '_custom') {
			activityKeyMode = 'custom';
			ruleForm.activity_key = '';
		} else {
			ruleForm.activity_key = val;
		}
	}

	async function submitRule() {
		if (!ruleForm.activity_key.trim()) return;
		ruleSaving = true;
		errorMessage = '';

		const result = editingRule
			? await walletApi.updateRule(editingRule.id, {
				activity_key: ruleForm.activity_key.trim(),
				point_value: ruleForm.point_value,
				description: ruleForm.description.trim(),
				is_enabled: ruleForm.is_enabled
			})
			: await walletApi.createRule({
				activity_key: ruleForm.activity_key.trim(),
				point_value: ruleForm.point_value,
				description: ruleForm.description.trim(),
				is_enabled: ruleForm.is_enabled
			});

		if (result.ok) {
			showRuleModal = false;
			editingRule = null;
			ruleForm = { activity_key: '', point_value: 1, description: '', is_enabled: true };
			loadRules();
		} else {
			errorMessage = result.error;
		}
		ruleSaving = false;
	}

	async function toggleRule(rule: ActivityRule) {
		errorMessage = '';
		const result = await walletApi.toggleRule(rule.id);
		if (result.ok) {
			loadRules();
		} else {
			errorMessage = result.error;
		}
	}

	async function deleteRule(rule: ActivityRule) {
		errorMessage = '';
		const result = await walletApi.deleteRule(rule.id);
		if (result.ok) {
			loadRules();
		} else {
			errorMessage = result.error;
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		errorMessage = '';
		try {
			for (const id of selectedIds) {
				await walletApi.deleteRule(id);
			}
			selectedIds = new Set();
			loadRules();
		} catch {
			errorMessage = t('app.wallet.error_delete_rules', 'Failed to delete selected rules');
		}
		bulkDeleting = false;
	}

	async function saveSettings() {
		settingsSaving = true;
		errorMessage = '';
		successMessage = '';
		const result = await walletApi.updateSettings(settings);
		if (result.ok) {
			successMessage = t('app.wallet.settings_saved', 'Settings saved successfully');
		} else {
			errorMessage = result.error;
		}
		settingsSaving = false;
	}

	// Debounced user search for autocomplete suggestions.
	function onUserSearchInput() {
		if (userSearchTimer) clearTimeout(userSearchTimer);
		const q = userSearch.trim();
		if (q.length < 2) {
			userSuggestions = [];
			showSuggestions = false;
			return;
		}
		userSearchTimer = setTimeout(async () => {
			userSearchLoading = true;
			const result = await userManagementApi.list(`search=${encodeURIComponent(q)}&page_size=5`);
			if (result.ok) {
				const data = result.data as any;
				userSuggestions = (data?.items ?? []).map((u: any) => ({
					id: u.id ?? u.user_id ?? '',
					username: u.username ?? '',
					email: u.email ?? ''
				}));
				showSuggestions = userSuggestions.length > 0;
			}
			userSearchLoading = false;
		}, 350);
	}

	function selectSuggestion(user: { id: string; username: string }) {
		userSearch = user.username;
		showSuggestions = false;
		userSuggestions = [];
		fetchBalance(user.id);
	}

	function clearUserSearch() {
		userSearch = '';
		userBalance = null;
		userSuggestions = [];
		showSuggestions = false;
	}

	async function fetchBalance(userId: string) {
		balanceLoading = true;
		userBalance = null;
		errorMessage = '';
		const result = await walletApi.getUserBalance(userId);
		if (result.ok) {
			userBalance = result.data as UserBalance;
		} else {
			errorMessage = result.error;
		}
		balanceLoading = false;
	}

	// Resolve a search term to a user ID, then fetch balance.
	// Accepts a UUID (direct) or a username/email (looked up via /admin/users).
	async function searchUser() {
		const q = userSearch.trim();
		if (!q) return;

		// If input looks like a UUID (contains hyphens and is long enough), try direct.
		if (q.length >= 36 && q.includes('-')) {
			fetchBalance(q);
			return;
		}

		// Otherwise resolve username/email → UUID via the admin users list.
		balanceLoading = true;
		userBalance = null;
		errorMessage = '';
		showSuggestions = false;

		const result = await userManagementApi.list(`search=${encodeURIComponent(q)}&page_size=1`);
		if (result.ok) {
			const data = result.data as any;
			const items = data?.items ?? [];
			if (items.length > 0) {
				fetchBalance(items[0].id);
				return;
			}
		}
		errorMessage = t('app.wallet.user_not_found', 'User not found');
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
			ledger_type: adjustForm.ledger_type
		});
		if (result.ok) {
			successMessage = t('app.wallet.adjust_success', 'Adjusted {type} by {amount}')
				.replace('{type}', adjustForm.ledger_type)
				.replace('{amount}', String(adjustForm.amount));
			adjustForm = { amount: 0, reason: '', ledger_type: 'points' };
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
	<title>{t('app.wallet.title', 'Wallet Management')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('app.wallet.title', 'Wallet Management')}
	description={t('app.wallet.description', 'Configure activity point rules, wallet settings, and user balance adjustments')}
	actions={[
		{ label: t('app.wallet.export', 'Export'), icon: Download, variant: 'outline' }
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
				<h3 class="text-lg font-semibold text-foreground">{t('app.wallet.activity_point_rules', 'Activity Point Rules')}</h3>
				<Button onclick={openAddRuleModal}>
					<Plus class="h-4 w-4 mr-2" />
					{t('app.wallet.add_rule', 'Add Rule')}
				</Button>
				</div>
			{#if rulesLoading}
				<p class="py-8 text-center text-sm text-muted">{t('common.loading', 'Loading...')}</p>
			{:else}
				<DataTable columns={ruleColumns} data={rules} pageSize={20} emptyMessage={t('app.wallet.empty_rules', 'No rules configured')} searchable onsearch={(q) => { searchQuery = q; loadRules(); }} selectable bind:selectedIds>
				{#snippet bulkActions()}
					<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
						<Trash2 class="h-3.5 w-3.5 mr-1" />
						{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.delete_selected', 'Delete Selected')} ({selectedIds.size})
					</Button>
				{/snippet}
			</DataTable>
				{/if}
			</CardContent>
		</Card>
	{:else if activeTab === 'settings'}
		<Card>
			<CardContent class="p-6">
			<h3 class="mb-4 text-lg font-semibold text-foreground">{t('app.wallet.wallet_settings', 'Wallet Settings')}</h3>
			{#if settingsError}
				<div class="mb-4 rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">{settingsError}</div>
			{/if}
			{#if settingsLoading}
				<p class="py-8 text-center text-sm text-muted">{t('common.loading', 'Loading...')}</p>
				{:else}
					<div class="max-w-md space-y-4">
						<div>
							<label for="ws-rate" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.points_to_credit_ratio_2', 'Points to Credit Ratio')}</label>
							<input id="ws-rate" type="number" min="1" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" bind:value={settings.points_to_credits_rate} />
						</div>
						<div>
							<label for="ws-cap" class="mb-1 block text-sm font-medium text-foreground-alt">{t('app.wallet.daily_earning_cap', 'Daily Earning Cap')}</label>
							<input id="ws-cap" type="number" min="0" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" bind:value={settings.daily_earning_cap} />
						</div>
						<div class="flex items-center gap-3">
							<input id="ws-earning" type="checkbox" bind:checked={settings.enable_earning} class="h-4 w-4 rounded border" />
							<label for="ws-earning" class="text-sm font-medium text-foreground-alt">{t('app.wallet.enable_earning', 'Enable Earning')}</label>
						</div>
					</div>
					<div class="mt-6 flex justify-end">
						<Button onclick={saveSettings} disabled={settingsSaving}>
							{settingsSaving ? t('common.saving', 'Saving...') : t('app.wallet.save_settings', 'Save Settings')}
						</Button>
					</div>
				{/if}
			</CardContent>
		</Card>
	{:else if activeTab === 'adjust'}
		<Card>
			<CardContent class="p-6">
			<h3 class="mb-4 text-lg font-semibold text-foreground">{t('app.wallet.user_balance_adjustment', 'User Balance Adjustment')}</h3>
			<div class="relative mb-4 flex gap-2">
				<div class="relative flex-1">
					<input type="text" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" placeholder={t('app.wallet.search_placeholder', 'Search by username or UUID...')} bind:value={userSearch} oninput={onUserSearchInput} onfocus={() => { if (userSuggestions.length > 0) showSuggestions = true; }} onblur={() => { setTimeout(() => showSuggestions = false, 200); }} onkeydown={(e) => { if (e.key === 'Enter') { showSuggestions = false; searchUser(); } }} />
					{#if userSearch && !userSearchLoading}
						<button class="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-foreground" onclick={clearUserSearch} type="button"><X class="h-4 w-4" /></button>
					{/if}
					{#if userSearchLoading}
						<span class="absolute right-8 top-1/2 -translate-y-1/2 text-xs text-muted">{t('common.searching', 'Searching...')}</span>
					{/if}
					{#if showSuggestions && userSuggestions.length > 0}
						<div class="absolute left-0 right-0 top-full z-50 mt-1 rounded-lg border bg-surface shadow-lg">
							{#each userSuggestions as user}
								<button type="button" class="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-accent" onmousedown={() => selectSuggestion(user)}>
									<span class="font-medium text-foreground">{user.username}</span>
									<span class="text-xs text-muted">{user.email}</span>
								</button>
							{/each}
						</div>
					{/if}
				</div>
				<Button onclick={searchUser} disabled={balanceLoading}>{balanceLoading ? t('common.searching', 'Searching...') : t('common.search', 'Search')}</Button>
			</div>
			{#if balanceLoading}
				<p class="py-8 text-center text-sm text-muted">{t('common.searching', 'Searching...')}</p>
				{:else if userBalance}
					<div class="mb-4 grid gap-3 sm:grid-cols-3">
						<div class="rounded-lg border bg-surface p-3"><p class="text-xs font-medium text-muted">{t('app.wallet.user', 'User')}</p><p class="text-sm font-semibold">{userBalance.username}</p></div>
						<div class="rounded-lg border bg-surface p-3"><p class="text-xs font-medium text-muted">{t('app.wallet.points', 'Points')}</p><p class="text-lg font-bold text-primary-600">{userBalance.points}</p></div>
						<div class="rounded-lg border bg-surface p-3"><p class="text-xs font-medium text-muted">{t('app.wallet.credits', 'Credits')}</p><p class="text-lg font-bold text-success-600">{userBalance.credits}</p></div>
					</div>
					<div class="grid gap-3 sm:grid-cols-3">
						<div>
							<label for="adj-amount" class="mb-1 block text-sm font-medium">{t('app.wallet.amount', 'Amount')}</label>
							<input id="adj-amount" type="number" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" bind:value={adjustForm.amount} />
						</div>
						<div>
							<span class="mb-1 block text-sm font-medium">{t('app.wallet.type', 'Type')}</span>
							<Select options={adjTypeOptions} bind:value={adjustForm.ledger_type} />
						</div>
						<div>
						<label for="adj-reason" class="mb-1 block text-sm font-medium">{t('app.wallet.reason', 'Reason')}</label>
						<input id="adj-reason" type="text" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" placeholder={t('app.wallet.reason_placeholder', 'Admin adjustment')} bind:value={adjustForm.reason} />
						</div>
					</div>
					<div class="mt-4 flex justify-end">
						<Button onclick={submitAdjust} disabled={adjustSaving || adjustForm.amount === 0 || !adjustForm.reason.trim()}>
							{adjustSaving ? t('common.adjusting', 'Adjusting...') : t('app.wallet.adjust_balance_btn', 'Adjust Balance')}
						</Button>
					</div>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>

<Modal bind:open={showRuleModal} title={editingRule ? t('app.wallet.edit_rule', 'Edit Activity Rule') : t('app.wallet.modal_add_rule', 'Add Activity Rule')}>
	<div class="space-y-4">
		<div>
			<label for="rule-key" class="mb-1 block text-sm font-medium">{t('app.wallet.activity_key', 'Activity Key')}</label>
			{#if activityKeyMode === 'select'}
				<Select options={activityKeyOptions} value={ruleForm.activity_key} onchange={handleActivityKeySelect} placeholder="Select an activity key..." />
			{:else}
				<div class="flex gap-2">
					<input id="rule-key" type="text" class="flex-1 rounded-lg border bg-surface px-3 py-2 text-sm font-mono" placeholder="custom_activity_key" bind:value={ruleForm.activity_key} />
					<Button variant="outline" size="sm" onclick={() => { activityKeyMode = 'select'; ruleForm.activity_key = ''; }}>
						<X class="h-4 w-4" />
					</Button>
				</div>
			{/if}
		</div>
		<div>
			<label for="rule-points" class="mb-1 block text-sm font-medium">{t('app.wallet.points', 'Points')}</label>
			<input id="rule-points" type="number" min="1" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" bind:value={ruleForm.point_value} />
		</div>
		<div>
			<label for="rule-desc" class="mb-1 block text-sm font-medium">{t('app.wallet.description', 'Description')}</label>
			<input id="rule-desc" type="text" class="w-full rounded-lg border bg-surface px-3 py-2 text-sm" placeholder={t('app.wallet.description_placeholder', 'Awarded for creating a post')} bind:value={ruleForm.description} />
		</div>
		<div class="flex items-center gap-2">
			<input id="rule-active" type="checkbox" bind:checked={ruleForm.is_enabled} class="h-4 w-4 rounded border" />
			<label for="rule-active" class="text-sm font-medium">{t('app.wallet.active', 'Active')}</label>
		</div>
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" onclick={() => showRuleModal = false}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={submitRule} disabled={ruleSaving}>{ruleSaving ? t('common.saving', 'Saving...') : editingRule ? t('common.save', 'Save') : t('app.wallet.add_rule', 'Add Rule')}</Button>
		</div>
	</div>
</Modal>
