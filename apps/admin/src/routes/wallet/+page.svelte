<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { walletApi } from '$lib/api';

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

	onMount(() => {
		loadRules();
		loadSettings();
	});
</script>

<svelte:head>
	<title>Wallet Management — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Wallet Management</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Configure activity point rules, wallet settings, and user balance adjustments</p>

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

<!-- Activity Point Rules -->
<section class="mb-8">
	<div class="mb-3 flex items-center justify-between">
		<h2 class="text-lg font-semibold">Activity Point Rules</h2>
		<button
			class="rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={() => { ruleForm = { event_type: '', points_value: 0, description: '', is_active: true }; showRuleModal = true; }}
		>
			Add Rule
		</button>
	</div>

	{#if rulesLoading}
		<p class="py-4 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-color: var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Event Type</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Points</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Description</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Active</th>
					</tr>
				</thead>
				<tbody>
					{#each rules as rule}
						<tr class="border-t" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-medium font-mono text-xs">{rule.event_type}</td>
							<td class="px-4 py-3 font-semibold">{rule.points_value}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{rule.description || '—'}</td>
							<td class="px-4 py-3">
								<span class="inline-block rounded-full px-2 py-0.5 text-xs {rule.is_active ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
									{rule.is_active ? 'Active' : 'Inactive'}
								</span>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">No rules configured</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</section>

<!-- Wallet Settings -->
<section class="mb-8">
	<h2 class="mb-3 text-lg font-semibold">Wallet Settings</h2>

	{#if settingsLoading}
		<p class="py-4 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
	{:else}
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="grid gap-4 sm:grid-cols-3">
				<div>
					<label for="ws-ratio" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Points to Credit Ratio</label>
					<input id="ws-ratio" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={settings.points_to_credit_ratio} min="0" step="0.01" />
				</div>
				<div>
					<label for="ws-cap" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Daily Earning Cap</label>
					<input id="ws-cap" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={settings.daily_earning_cap} min="0" />
				</div>
				<div class="flex items-end gap-2">
					<input id="ws-earning" type="checkbox" bind:checked={settings.enable_earning} />
					<label for="ws-earning" class="text-sm">Enable Earning</label>
				</div>
			</div>
			<div class="mt-4 flex justify-end">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={settingsSaving}
					onclick={saveSettings}
				>
					{settingsSaving ? 'Saving...' : 'Save Settings'}
				</button>
			</div>
		</div>
	{/if}
</section>

<!-- User Balance Adjustment -->
<section>
	<h2 class="mb-3 text-lg font-semibold">User Balance Adjustment</h2>

	<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<!-- Search -->
		<div class="mb-4 flex gap-2">
			<input
				type="text"
				class="flex-1 rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				placeholder="Search by username or user ID..."
				bind:value={userSearch}
				onkeydown={(e) => { if (e.key === 'Enter') searchUser(); }}
			/>
			<button
				class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
				style="background-color: var(--color-primary)"
				onclick={searchUser}
			>
				Search
			</button>
		</div>

		{#if balanceLoading}
			<p class="py-4 text-center text-sm" style="color: var(--color-muted)">Searching...</p>
		{:else if userBalance}
			<!-- Current Balance -->
			<div class="mb-4 grid gap-3 sm:grid-cols-3">
				<div class="rounded-md border p-3" style="border-color: var(--color-border); background-color: var(--color-background)">
					<p class="text-xs font-medium" style="color: var(--color-muted)">User</p>
					<p class="text-sm font-semibold">{userBalance.username}</p>
				</div>
				<div class="rounded-md border p-3" style="border-color: var(--color-border); background-color: var(--color-background)">
					<p class="text-xs font-medium" style="color: var(--color-muted)">Points</p>
					<p class="text-lg font-bold" style="color: var(--color-primary)">{userBalance.points}</p>
				</div>
				<div class="rounded-md border p-3" style="border-color: var(--color-border); background-color: var(--color-background)">
					<p class="text-xs font-medium" style="color: var(--color-muted)">Credits</p>
					<p class="text-lg font-bold" style="color: var(--color-success)">{userBalance.credits}</p>
				</div>
			</div>

			<!-- Adjustment Form -->
			<div class="grid gap-3 sm:grid-cols-3">
				<div>
					<label for="adj-amount" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Amount (+ credit, - debit)</label>
					<input id="adj-amount" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={adjustForm.amount} />
				</div>
				<div>
					<label for="adj-type" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Type</label>
					<select id="adj-type" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" bind:value={adjustForm.type}>
						<option value="points">Points</option>
						<option value="credits">Credits</option>
					</select>
				</div>
				<div>
					<label for="adj-reason" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Reason</label>
					<input id="adj-reason" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background)" placeholder="Admin adjustment" bind:value={adjustForm.reason} />
				</div>
			</div>
			<div class="mt-4 flex justify-end">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={adjustSaving || adjustForm.amount === 0 || !adjustForm.reason.trim()}
					onclick={submitAdjust}
				>
					{adjustSaving ? 'Adjusting...' : 'Adjust Balance'}
				</button>
			</div>
		{/if}
	</div>
</section>

<!-- Add Rule Modal -->
<Modal bind:open={showRuleModal} title="Add Activity Rule" onConfirm={submitRule} confirmLabel={ruleSaving ? 'Saving...' : 'Add Rule'}>
	<div class="space-y-3">
		<div>
			<label for="rule-event" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Event Type</label>
			<input id="rule-event" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="post.created" bind:value={ruleForm.event_type} />
		</div>
		<div>
			<label for="rule-points" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Points Value</label>
			<input id="rule-points" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={ruleForm.points_value} />
		</div>
		<div>
			<label for="rule-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Description</label>
			<input id="rule-desc" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="Awarded for creating a post" bind:value={ruleForm.description} />
		</div>
		<div class="flex items-center gap-2">
			<input id="rule-active" type="checkbox" bind:checked={ruleForm.is_active} />
			<label for="rule-active" class="text-sm">Active</label>
		</div>
	</div>
</Modal>
