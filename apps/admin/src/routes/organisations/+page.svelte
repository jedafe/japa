<script lang="ts">
	import { onMount } from 'svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
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
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let filterVerified = $state('');
	let errorMessage = $state('');

	// Claim queue modal
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
		try {
			const result = await api<{ data: Organisation[] }>(`/organizations?${params}`);
			if (result.ok) {
				orgs = Array.isArray(result.data) ? result.data : (result.data.data ?? []);
				totalPages = 1;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filterVerified = values.verified ?? '';
		page = 0;
		load();
	}

	async function toggleVerify(org: Organisation) {
		errorMessage = '';
		try {
			const result = org.verified
				? await api(`/admin/organizations/${org.id}/verify`, { method: 'DELETE' })
				: await api(`/admin/organizations/${org.id}/verify`, { method: 'POST', body: { verified: true } });
			if (result.ok) {
				org.verified = !org.verified;
				orgs = orgs;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
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
			const result = await api(`/admin/organizations/${claimOrg.id}/claim`, {
				method: 'POST',
				body: { note: claimNote.trim() }
			});
			if (result.ok) {
				showClaimModal = false;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('organisations.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('organisations.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage organizations, verification status, and claim requests</p>

<FilterBar filters={filterDefs} onApply={handleFilter} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.owner')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.members')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.verified')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('organisations.created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each orgs as org}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<p class="font-medium">{org.name}</p>
							{#if org.domain}
								<p class="text-xs" style="color: var(--color-muted)">{org.domain}</p>
							{/if}
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{org.owner_user_id ?? '—'}</td>
						<td class="px-4 py-3 text-xs font-medium">{org.member_count.toLocaleString()}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {org.verified ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{org.verified ? t('organisations.yes') : t('organisations.no')}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{org.created_at}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: {org.verified ? 'var(--color-warning)' : 'var(--color-success)'}"
									onclick={() => toggleVerify(org)}
								>
									{org.verified ? t('organisations.unverify') : t('organisations.verify')}
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openClaim(org)}
								>
									{t('organisations.claim_queue')}
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

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page === 0} onclick={() => handlePageChange(page - 1)}>&larr;</button>
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => handlePageChange(page + 1)}>&rarr;</button>
		</div>
	</div>
{/if}

<!-- Claim Queue Modal -->
<Modal bind:open={showClaimModal} title={t('organisations.claim_queue')} onConfirm={submitClaim} confirmLabel={t('common.confirm')}>
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
