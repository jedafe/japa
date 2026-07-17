<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { universitiesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type University = {
		id: string;
		name: string;
		slug: string;
		member_count: number;
		status: string;
		created_at: string;
		description?: string;
	};

	let universities = $state<University[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	// Create/Edit modal
	let showModal = $state(false);
	let editingUni = $state<University | null>(null);
	let form = $state({ name: '', domain: '', verified: false });
	let saving = $state(false);

	// Delete modal
	let showDeleteModal = $state(false);
	let deleteTarget = $state<University | null>(null);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await universitiesAdminApi.list();
			if (result.ok && result.data) {
				const raw = result.data as unknown;
				universities = Array.isArray(raw) ? raw : ((raw as { data?: University[] }).data ?? []);
			} else if (!result.ok) {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error', 'Failed to load data');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingUni = null;
		form = { name: '', domain: '', verified: false };
		showModal = true;
	}

	function openEdit(uni: University) {
		editingUni = uni;
		form = {
			name: uni.name,
			domain: uni.slug,
			verified: uni.status === 'active'
		};
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = '';
		const body = {
			name: form.name.trim(),
			domain: form.domain.trim(),
			verified: form.verified
		};
		try {
			const result = editingUni
				? await universitiesAdminApi.update(editingUni.id, body)
				: await universitiesAdminApi.create(body);
			if (result.ok) {
				showModal = false;
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error', 'Failed to save');
		} finally {
			saving = false;
		}
	}

	function confirmDelete(uni: University) {
		deleteTarget = uni;
		showDeleteModal = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		try {
			const result = await universitiesAdminApi.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteModal = false;
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error', 'Failed to delete');
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('universities.title', 'Universities')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('universities.title', 'Universities')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('universities.manage_university_registrations_and_veri')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('universities.create', 'Create University')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.name', 'Name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.slug')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.students', 'Members')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.created', 'Created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each universities as uni (uni.id)}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<p class="font-medium">{uni.name}</p>
						</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{uni.slug}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {uni.status === 'active' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-amber-500/15 text-amber-700 dark:text-amber-400'}">
								{uni.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs font-medium">{(uni.member_count ?? 0).toLocaleString()}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(uni.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-primary)"
									onclick={() => openEdit(uni)}
								>
									{t('common.edit', 'Edit')}
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete(uni)}
								>
									{t('common.delete', 'Delete')}
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No universities found
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal
	bind:open={showModal}
	title={editingUni ? t('universities.edit', 'Edit University') : t('universities.create', 'Create University')}
	onConfirm={submit}
	confirmLabel={saving ? t('common.saving', 'Saving...') : editingUni ? t('common.save', 'Save') : t('universities.create', 'Create')}
>
	<div class="space-y-3">
		<div>
			<label for="uni-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('universities.name', 'Name')}</label>
			<input
				id="uni-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="MIT"
				bind:value={form.name}
			/>
		</div>
		<div>
			<label for="uni-domain" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('universities.domain_slug')}</label>
			<input
				id="uni-domain"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm font-mono"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="mit.edu"
				bind:value={form.domain}
			/>
		</div>
		<div class="flex items-center gap-2">
			<input id="uni-verified" type="checkbox" bind:checked={form.verified} />
			<label for="uni-verified" class="text-sm">{t('universities.verified', 'Verified')}</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteModal} title={t('common.confirm', 'Confirm')} onConfirm={executeDelete} confirmLabel={t('common.delete', 'Delete')}>
	<p class="text-sm" style="color: var(--color-muted)">
		Are you sure you want to delete <strong>{deleteTarget?.name}</strong>?
	</p>
</Modal>
