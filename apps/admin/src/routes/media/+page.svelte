<script lang="ts">
	import { onMount } from 'svelte';
	import { mediaAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, CheckCircle, XCircle, Trash2, Shield } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type MediaItem = {
		id: string;
		filename: string;
		mime_type: string;
		file_size: number;
		alt_text?: string;
		status: string;
		created_at: string;
	};

	let items = $state<MediaItem[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let activeTab = $state<'all' | 'quarantined'>('all');

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deleteTargetId = $state('');
	let deleteTargetName = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = activeTab === 'quarantined'
				? await mediaAdminApi.listQuarantined()
				: await mediaAdminApi.list();
			if (result.ok) {
				items = (result.data as MediaItem[]) ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function approve(id: string) {
		errorMessage = '';
		try {
			const result = await mediaAdminApi.approve(id);
			if (result.ok) {
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function reject(id: string) {
		errorMessage = '';
		try {
			const result = await mediaAdminApi.reject(id);
			if (result.ok) {
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	function confirmDelete(id: string, filename: string) {
		deleteTargetId = id;
		deleteTargetName = filename;
		showDeleteModal = true;
	}

	async function deleteMedia() {
		errorMessage = '';
		try {
			const result = await mediaAdminApi.delete(deleteTargetId);
			if (result.ok) {
				items = items.filter((i) => i.id !== deleteTargetId);
				showDeleteModal = false;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	function formatSize(bytes: number) {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	function statusColor(status: string) {
		switch (status) {
			case 'approved':
				return 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400';
			case 'pending':
				return 'bg-amber-500/15 text-amber-700 dark:text-amber-400';
			case 'quarantined':
				return 'bg-red-500/15 text-red-700 dark:text-red-400';
			case 'rejected':
				return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
			default:
				return 'bg-slate-500/15 text-slate-700 dark:text-slate-400';
		}
	}

	onMount(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('media.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('media.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('media.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'all' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'all'; load(); }}
	>
		{t('media.all')}
	</button>
	<button
		class="flex items-center gap-1.5 rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'quarantined' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'quarantined'; load(); }}
	>
		<Shield class="h-3.5 w-3.5" />
		{t('media.quarantined')}
	</button>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button
		class="btn btn-primary flex items-center gap-1.5 text-xs"
		onclick={load}
	>
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
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('media.filename')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('media.type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('media.size')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('media.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('media.uploaded')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each items as item (item.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{item.filename}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.mime_type}</td>
						<td class="px-4 py-3 text-xs">{formatSize(item.file_size)}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {statusColor(item.status)}">
								{item.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(item.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								{#if item.status === 'quarantined' || item.status === 'pending'}
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-success)"
										onclick={() => approve(item.id)}
									>
										<CheckCircle class="h-3 w-3 inline mr-1" />
										{t('media.approve')}
									</button>
									<button
										class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-warning)"
										onclick={() => reject(item.id)}
									>
										<XCircle class="h-3 w-3 inline mr-1" />
										{t('media.reject')}
									</button>
								{/if}
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete(item.id, item.filename)}
								>
									<Trash2 class="h-3 w-3 inline mr-1" />
									{t('common.delete')}
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('media.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title={t('media.delete_confirm_title')} onConfirm={deleteMedia} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('media.delete_confirm_message')}</p>
</Modal>
