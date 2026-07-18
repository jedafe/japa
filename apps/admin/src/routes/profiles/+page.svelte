<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, CheckCircle, Eye } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Profile = {
		id: string;
		user_id: string;
		display_name: string;
		professional_type: string;
		is_verified: boolean;
		created_at: string;
	};

	let profiles = $state<Profile[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showViewModal = $state(false);
	let viewTarget = $state<Profile | null>(null);

	function openView(profile: Profile) {
		viewTarget = profile;
		showViewModal = true;
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Profile[]>('/profiles');
			if (result.ok) {
				profiles = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function verifyProfile(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/profiles/${id}/verify`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function unverifyProfile(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/profiles/${id}/verify`, { method: 'DELETE' });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('profiles.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('profiles.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('profiles.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('profiles.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('profiles.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('profiles.verification')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each profiles as profile (profile.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{profile.display_name}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{profile.professional_type}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {profile.is_verified ? 'bg-success/15 text-success dark:text-success' : 'bg-warning/15 text-warning dark:text-warning'}">
								{profile.is_verified ? t('profiles.verified') : t('profiles.unverified')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-2">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-muted)" onclick={() => openView(profile)}>
									<Eye class="h-3 w-3 inline mr-1" />
									{t('common.view')}
								</button>
								{#if !profile.is_verified}
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => verifyProfile(profile.id)}>
										<CheckCircle class="h-3 w-3 inline mr-1" style="color: var(--color-success)" />
										{t('profiles.verify')}
									</button>
								{:else}
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" onclick={() => unverifyProfile(profile.id)} style="color: var(--color-danger)">
										{t('profiles.unverify')}
									</button>
								{/if}
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('profiles.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<Modal bind:open={showViewModal} title={t('profiles.details', 'Profile Details')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('profiles.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.display_name}</p>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('profiles.type', 'Type')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.professional_type}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('profiles.verification', 'Verification')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.is_verified ? t('profiles.verified') : t('profiles.unverified')}</p>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('profiles.created', 'Created')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showViewModal = false)}>{t('common.close', 'Close')}</button>
	</div>
</Modal>
