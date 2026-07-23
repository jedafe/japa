<script lang="ts">
	import { api, userManagementApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	const canManage = $derived(hasPermission('admin.broadcast'));
	import { Select } from '$lib/components/ui/select';
	import Modal from '$lib/components/Modal.svelte';
	import { Button } from '$lib/components/ui/button';
	import { DataTable } from '$lib/components/ui/data-table';

	type Broadcast = {
		id: string;
		title: string;
		body: string;
		sent_by: string;
		sent_at: string;
		recipient_count: number;
	};

	let activeTab = $state<'send' | 'history'>('send');
	let broadcasts = $state<Broadcast[]>([]);
	let loading = $state(true);
	let error = $state('');
	let searchQuery = $state('');

	let title = $state('');
	let message = $state('');
	let targetScope = $state('all');
	let scheduledAt = $state('');
	let sending = $state(false);
	let resultMsg = $state('');
	let resultType = $state<'success' | 'error'>('success');

	let scopes = $state([
		{ value: 'all', label: t('broadcast.audiences.all') }
	]);

	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Broadcast | null>(null);

	const columns = [
		{
			key: 'title',
			label: t('broadcast.title_label'),
			sortable: true,
			render: (_: any, row: Broadcast) => `<span class="font-medium">${row.title}</span>`
		},
		{
			key: 'sent_by',
			label: t('broadcast.sent_by'),
			sortable: true,
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
		},
		{
			key: 'recipient_count',
			label: t('broadcast.recipients'),
			sortable: true
		},
		{
			key: 'sent_at',
			label: t('broadcast.sent_at'),
			sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{
			key: 'actions',
			label: t('common.actions'),
			class: 'w-24',
			component: BroadcastActionsCell
		}
	];

	$effect(() => {
		(async () => {
			try {
				const result = await userManagementApi.listGroups();
				if (result.ok) {
					const groupScopes = result.data.groups.map((g) => ({
						value: `group:${g.name}`,
						label: g.name
					}));
					scopes = [...scopes, ...groupScopes];
				}
			} catch {
				// Keep default scopes
			}
			if (activeTab === 'history') loadBroadcasts();
		})();
	});

	async function loadBroadcasts() {
		loading = true;
		error = '';
		try {
			const result = await api<Broadcast[]>('/admin/broadcasts');
			if (result.ok) {
				broadcasts = result.data ?? [];
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function sendBroadcast() {
		if (!title || !message) return;
		sending = true;
		resultMsg = '';
		try {
			await api('/admin/broadcasts', {
				method: 'POST',
				body: {
					title,
					message,
					target_scope: targetScope,
					scheduled_at: scheduledAt || null
				}
			});
			resultType = 'success';
			resultMsg = scheduledAt ? t('broadcast.scheduled') : t('broadcast.sent');
			title = '';
			message = '';
			scheduledAt = '';
		} catch {
			resultType = 'error';
			resultMsg = t('broadcast.error');
		} finally {
			sending = false;
		}
	}

	function confirmDelete(bc: Broadcast) {
		deleteTarget = bc;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		try {
			await api(`/admin/broadcasts/${deleteTarget.id}`, { method: 'DELETE' });
			broadcasts = broadcasts.filter((b) => b.id !== deleteTarget!.id);
		} catch {
			error = t('common.error');
		} finally {
			showDeleteConfirm = false;
			deleteTarget = null;
		}
	}
</script>

{#snippet BroadcastActionsCell({ value, row }: { value: any; row: Broadcast })}
	{#if canManage}
		<Button variant="ghost" size="sm" onclick={(e: MouseEvent) => { e.stopPropagation(); confirmDelete(row); }} style="color: var(--color-danger)">
			{t('common.delete')}
		</Button>
	{/if}
{/snippet}

<svelte:head>
	<title>Broadcast — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('broadcast.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('broadcast.description')}</p>

<!-- Tabs -->
<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'send' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'send'; }}
	>
		{t('broadcast.send_tab')}
	</button>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'history' ? 'background-color: var(--color-primary); color: white' : ''}
		onclick={() => { activeTab = 'history'; loadBroadcasts(); }}
	>
		{t('broadcast.history_tab')}
	</button>
</div>

{#if error}
	<div class="mb-4 rounded-md border p-3 text-sm" style="border-color: var(--color-danger); background-color: var(--color-danger-bg)">
		{error}
	</div>
{/if}

{#if activeTab === 'send'}
	<div class="mx-auto max-w-2xl rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="space-y-4">
			<div>
				<label for="bc-title" class="mb-1 block text-sm font-medium">{t('broadcast.title_label')}</label>
				<input
					id="bc-title"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					placeholder={t('broadcast.title_placeholder')}
					bind:value={title}
				/>
			</div>

			<div>
				<label for="bc-message" class="mb-1 block text-sm font-medium">{t('broadcast.message')}</label>
				<textarea
					id="bc-message"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					rows="4"
					placeholder={t('broadcast.message_placeholder')}
					bind:value={message}
				></textarea>
			</div>

			<div>
				<label for="bc-target" class="mb-1 block text-sm font-medium">{t('broadcast.audience')}</label>
				<Select options={scopes} bind:value={targetScope} />
			</div>

			<div>
				<label for="bc-schedule" class="mb-1 block text-sm font-medium">{t('broadcast.schedule')}</label>
				<input
					id="bc-schedule"
					type="datetime-local"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={scheduledAt}
				/>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('broadcast.helper')}</p>
			</div>

			<div class="flex items-center gap-3 pt-2">
				{#if canManage}<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={sending || !title || !message}
					onclick={sendBroadcast}
				>
					{sending ? t('broadcast.sending') : scheduledAt ? t('broadcast.schedule_btn') : t('broadcast.send_now')}
				</button>{/if}
				{#if resultMsg}
					<span class="text-sm font-medium" style="color: {resultType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'}">{resultMsg}</span>
				{/if}
			</div>
		</div>
	</div>
{:else}
	{#if loading}
		<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else}
		<DataTable {columns} data={broadcasts} pageSize={25} searchable emptyMessage={t('broadcast.no_broadcasts')} />
	{/if}
{/if}

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete', 'Confirm Delete')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('broadcast.confirm_delete', 'Delete broadcast')} <strong>{deleteTarget.title}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>
