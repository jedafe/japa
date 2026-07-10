<script lang="ts">
	import { onMount } from 'svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type University = {
		id: string;
		name: string;
		domain: string;
		verified: boolean;
		student_count: number;
		created_at: string;
		logo_url?: string;
	};

	let universities = $state<University[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
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
		const params = new URLSearchParams({ page: String(page) });
		try {
			const result = await api<{ items: University[]; page: number; total_pages: number }>(`/admin/universities?${params}`);
			if (result.ok) {
				universities = result.data.items;
				totalPages = result.data.total_pages;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
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
			domain: uni.domain,
			verified: uni.verified
		};
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim() || !form.domain.trim()) return;
		saving = true;
		errorMessage = '';
		const body = {
			name: form.name.trim(),
			domain: form.domain.trim(),
			verified: form.verified
		};
		const result = editingUni
			? await api(`/admin/universities/${editingUni.id}`, { method: 'PATCH', body })
			: await api('/admin/universities', { method: 'POST', body });
		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(uni: University) {
		deleteTarget = uni;
		showDeleteModal = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		const result = await api(`/admin/universities/${deleteTarget.id}`, { method: 'DELETE' });
		if (result.ok) {
			showDeleteModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('universities.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('universities.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage university registrations and verification</p>

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('universities.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.domain')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.verified')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.students')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('universities.created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each universities as uni}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<p class="font-medium">{uni.name}</p>
						</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{uni.domain}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {uni.verified ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{uni.verified ? t('universities.yes') : t('universities.no')}
							</span>
						</td>
						<td class="px-4 py-3 text-xs font-medium">{uni.student_count.toLocaleString()}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{uni.created_at}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openEdit(uni)}
								>
									{t('common.edit')}
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete(uni)}
								>
									{t('common.delete')}
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

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page === 0} onclick={() => handlePageChange(page - 1)}>&larr;</button>
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => handlePageChange(page + 1)}>&rarr;</button>
		</div>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal
	bind:open={showModal}
	title={editingUni ? t('universities.edit') : t('universities.create')}
	onConfirm={submit}
	confirmLabel={saving ? t('common.saving') : editingUni ? t('common.save') : t('universities.create')}
>
	<div class="space-y-3">
		<div>
			<label for="uni-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('universities.name')}</label>
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
			<label for="uni-domain" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('universities.domain')}</label>
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
			<label for="uni-verified" class="text-sm">{t('universities.verified')}</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteModal} title={t('common.confirm')} onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">
		Are you sure you want to delete <strong>{deleteTarget?.name}</strong>?
	</p>
</Modal>
