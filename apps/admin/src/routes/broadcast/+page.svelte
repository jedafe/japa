<script lang="ts">
	import { onMount } from 'svelte';
	import { api, userManagementApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';

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

	// Send form
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

	onMount(async () => {
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
	});

	async function loadBroadcasts() {
		loading = true;
		error = '';
		try {
			const result = await api<{ data: Broadcast[] }>('/admin/broadcasts');
			if (result.ok) {
				broadcasts = result.data.data ?? [];
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

	async function deleteBroadcast(id: string) {
		if (!confirm(t('broadcast.confirm_delete'))) return;
		try {
			await api(`/admin/broadcasts/${id}`, { method: 'DELETE' });
			broadcasts = broadcasts.filter((b) => b.id !== id);
		} catch {
			error = t('common.error');
		}
	}
</script>

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
				<select
					id="bc-target"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-background)"
					bind:value={targetScope}
				>
					{#each scopes as scope}
						<option value={scope.value}>{scope.label}</option>
					{/each}
				</select>
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
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={sending || !title || !message}
					onclick={sendBroadcast}
				>
					{sending ? t('broadcast.sending') : scheduledAt ? t('broadcast.schedule_btn') : t('broadcast.send_now')}
				</button>
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
		<div class="rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-bottom: 1px solid var(--color-border)">
						<th class="px-4 py-3 text-left font-medium">{t('broadcast.title_label')}</th>
						<th class="px-4 py-3 text-left font-medium">{t('broadcast.sent_by')}</th>
						<th class="px-4 py-3 text-left font-medium">{t('broadcast.recipients')}</th>
						<th class="px-4 py-3 text-left font-medium">{t('broadcast.sent_at')}</th>
						<th class="px-4 py-3 text-right font-medium">{t('common.actions')}</th>
					</tr>
				</thead>
				<tbody>
					{#each broadcasts as bc (bc.id)}
						<tr style="border-bottom: 1px solid var(--color-border)">
							<td class="px-4 py-3 font-medium">{bc.title}</td>
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{bc.sent_by}</td>
							<td class="px-4 py-3">{bc.recipient_count}</td>
							<td class="px-4 py-3">{new Date(bc.sent_at).toLocaleDateString()}</td>
							<td class="px-4 py-3 text-right">
								<button
									class="text-xs text-red-500 hover:text-red-700"
									onclick={() => deleteBroadcast(bc.id)}
								>
									{t('common.delete')}
								</button>
							</td>
						</tr>
					{:else}
						<tr>
							<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
								{t('broadcast.no_broadcasts')}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
{/if}
