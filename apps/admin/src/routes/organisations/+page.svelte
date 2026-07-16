<script lang="ts">
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { organisationsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Organisation = {
		id: string;
		name: string;
		owner_user_id?: string;
		member_count: number;
		verified: boolean;
		created_at: string;
		slug?: string;
		status?: string;
	};

	let orgs = $state<Organisation[]>([]);
	let loading = $state(true);
	let filterVerified = $state('');
	let errorMessage = $state('');

	let showClaimModal = $state(false);
	let claimOrg = $state<Organisation | null>(null);
	let claimNote = $state('');

	const filterDefs = [
		{ key: 'verified', label: t('organisations.verified'), type: 'select' as const, options: ['true', 'false'] }
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterVerified) params.set('verified', filterVerified);
		const qs = params.toString();
		try {
			const result = await organisationsAdminApi.list(qs || undefined);
			if (result.ok && result.data) {
				const raw = result.data as unknown;
				orgs = Array.isArray(raw) ? raw : ((raw as { data?: Organisation[] }).data ?? []);
			} else if (!result.ok) {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error', 'Failed to load data');
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filterVerified = values.verified ?? '';
		load();
	}

	async function toggleVerify(org: Organisation) {
		errorMessage = '';
		const wasVerified = org.verified;
		try {
			const result = wasVerified
				? await organisationsAdminApi.unverify(org.id)
				: await organisationsAdminApi.verify(org.id, { verified: true });
			if (result.ok) {
				org.verified = !wasVerified;
				orgs = [...orgs];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error', 'Failed to update verification');
		}
	}

	function openClaim(org: Organisation) {
		claimOrg = org;
		claimNote = '';
		showClaimModal = true;
	}

	async function submitClaim() {
		if (!claimOrg) return;
		errorMessage = '';
		try {
			const result = await organisationsAdminApi.claim(claimOrg.id, { note: claimNote.trim() || undefined });
			if (result.ok) {
				showClaimModal = false;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error', 'Failed to submit claim');
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('organisations.title', 'Organisations')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('organisations.title', 'Organisations')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage organizations, verification status, and claim requests</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.name', 'Name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.owner', 'Owner')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.members', 'Members')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.verified', 'Verified')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.created', 'Created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each orgs as org (org.id)}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<p class="font-medium">{org.name}</p>
							{#if org.slug}
								<p class="text-xs" style="color: var(--color-muted)">{org.slug}</p>
							{/if}
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{org.owner_user_id ?? '—'}</td>
						<td class="px-4 py-3 text-xs font-medium">{(org.member_count ?? 0).toLocaleString()}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {org.verified ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{org.verified ? t('organisations.yes', 'Yes') : t('organisations.no', 'No')}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(org.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: {org.verified ? 'var(--color-warning)' : 'var(--color-success)'}"
									onclick={() => toggleVerify(org)}
								>
									{org.verified ? t('organisations.unverify', 'Unverify') : t('organisations.verify', 'Verify')}
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-primary)"
									onclick={() => openClaim(org)}
								>
									{t('organisations.claim_queue', 'Claim Queue')}
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No organisations found
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<Modal bind:open={showClaimModal} title={t('organisations.claim_queue', 'Claim Queue')} onConfirm={submitClaim} confirmLabel={t('common.confirm', 'Confirm')}>
	<p class="text-sm mb-3" style="color: var(--color-muted)">
		Review the ownership claim for <strong>{claimOrg?.name}</strong>.
	</p>
	<textarea
		class="w-full rounded-md border px-3 py-2 text-sm"
		style="border-color: var(--color-border); background-color: var(--color-surface)"
		rows="3"
		placeholder="Admin note (optional)"
		bind:value={claimNote}
	></textarea>
</Modal>
