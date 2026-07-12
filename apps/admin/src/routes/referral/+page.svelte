<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

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

	onMount(load);

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

	function statusBadge(status: string) {
		switch (status) {
			case 'completed': return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'pending': return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'cancelled': return 'bg-red-500/15 text-red-700 dark:text-red-400';
			default: return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}
</script>

<svelte:head>
	<title>{t('referral.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('referral.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('referral.description')}</p>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
		{error}
	</div>
{/if}

{#if loading}
	<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<!-- Program Settings -->
	{#if program}
		<div class="mb-8 rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-4 text-lg font-semibold">{t('referral.program_settings')}</h2>
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
				<div>
					<label for="prog-name" class="mb-1 block text-sm font-medium">{t('referral.name')}</label>
					<input
						id="prog-name"
						type="text"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={program.name}
					/>
				</div>
				<div>
					<label for="prog-reward-type" class="mb-1 block text-sm font-medium">{t('referral.reward_type')}</label>
					<select
						id="prog-reward-type"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={program.reward_type}
					>
						<option value="credits">{t('referral.credits')}</option>
						<option value="discount">{t('referral.discount')}</option>
						<option value="points">{t('referral.points')}</option>
					</select>
				</div>
				<div>
					<label for="prog-reward-value" class="mb-1 block text-sm font-medium">{t('referral.reward_value')}</label>
					<input
						id="prog-reward-value"
						type="number"
						min="0"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={program.reward_value}
					/>
				</div>
				<div class="flex items-end">
					<label class="flex items-center gap-2">
						<input
							type="checkbox"
							bind:checked={program.is_active}
							class="rounded"
						/>
						<span class="text-sm font-medium">{t('referral.active')}</span>
					</label>
				</div>
			</div>
			<div class="mt-4">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving}
					onclick={updateProgram}
				>
					{saving ? t('common.saving') : t('common.save')}
				</button>
			</div>
		</div>
	{/if}

	<!-- Referrals List -->
	<h2 class="mb-4 text-lg font-semibold">{t('referral.recent_referrals')}</h2>
	<div class="rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium">{t('referral.referrer')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('referral.referred')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('referral.status')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('referral.reward')}</th>
					<th class="px-4 py-3 text-left font-medium">{t('referral.date')}</th>
				</tr>
			</thead>
			<tbody>
				{#each referrals as ref (ref.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-medium">{ref.referrer_username}</td>
						<td class="px-4 py-3">{ref.referred_username}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusBadge(ref.status)}">
								{ref.status}
							</span>
						</td>
						<td class="px-4 py-3">
							{#if ref.reward_granted}
								<span class="text-emerald-600 dark:text-emerald-400">{t('referral.granted')}</span>
							{:else}
								<span style="color: var(--color-muted)">{t('referral.pending')}</span>
							{/if}
						</td>
						<td class="px-4 py-3">{new Date(ref.created_at).toLocaleDateString()}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('referral.no_referrals')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
