<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { RefreshCw, CheckCircle, Eye } from 'lucide-svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Button } from '$lib/components/ui/button';
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
	let searchQuery = $state('');

	const filterDefs = [
		{
			key: 'is_verified',
			label: 'Verification',
			options: [
				{ value: 'true', label: 'Verified' },
				{ value: 'false', label: 'Unverified' }
			]
		}
	];

	let filterValues = $state<Record<string, string>>({});

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterValues = values;
	}

	function handleClear() {
		searchQuery = '';
		filterValues = {};
	}

	let showViewModal = $state(false);
	let viewTarget = $state<Profile | null>(null);

	const canModerate = $derived(hasPermission('admin.moderation'));

	const filteredProfiles = $derived(
		(() => {
			let items = searchQuery
				? profiles.filter(
						(p) =>
							p.display_name.toLowerCase().includes(searchQuery.toLowerCase()) ||
							p.professional_type.toLowerCase().includes(searchQuery.toLowerCase())
					)
				: profiles;
			if (filterValues.is_verified) {
				const val = filterValues.is_verified === 'true';
				items = items.filter((p) => p.is_verified === val);
			}
			return items;
		})()
	);

	const columns = [
		{ key: 'display_name', label: t('profiles.name'), sortable: true,
			render: (v: string) => `<span class="font-medium">${v}</span>`
		},
		{ key: 'professional_type', label: t('profiles.type'), sortable: true,
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{ key: 'is_verified', label: t('profiles.verification'), sortable: true,
			render: (v: boolean) => {
				const cls = v ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning';
				const text = v ? t('profiles.verified') : t('profiles.unverified');
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${text}</span>`;
			}
		},
		{ key: 'actions', label: t('common.actions'), class: 'w-36', componentSnippet: ProfileActionsCell }
	];

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

<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		{columns}
		data={filteredProfiles}
		pageSize={25}
		searchable
		searchPlaceholder={t('profiles.search', 'Search profiles...')}
		onsearch={(q) => { searchQuery = q; }}
		emptyMessage={t('profiles.empty')}
	/>
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

{#snippet ProfileActionsCell({ value, row }: { value: any; row: Profile })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			<Eye class="h-3 w-3 inline mr-1" />
			{t('common.view')}
		</Button>
		{#if canModerate && !row.is_verified}
			<Button variant="ghost" size="sm" onclick={() => verifyProfile(row.id)}>
				<CheckCircle class="h-3 w-3 inline mr-1" style="color: var(--color-success)" />
				{t('profiles.verify')}
			</Button>
		{:else if canModerate}
			<Button variant="ghost" size="sm" onclick={() => unverifyProfile(row.id)} style="color: var(--color-danger)">
				{t('profiles.unverify')}
			</Button>
		{/if}
	</div>
{/snippet}
