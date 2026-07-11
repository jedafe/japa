<script lang="ts">
	import { onMount } from 'svelte';
	import { api, userManagementApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

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
	});

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
</script>

<svelte:head>
	<title>Broadcast — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('broadcast.title')}</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('broadcast.description')}</p>

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
