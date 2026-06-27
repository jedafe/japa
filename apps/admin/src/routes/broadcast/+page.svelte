<script lang="ts">
	import { api } from '$lib/api';

	let title = $state('');
	let message = $state('');
	let targetScope = $state('all');
	let scheduledAt = $state('');
	let sending = $state(false);
	let resultMsg = $state('');
	let resultType = $state<'success' | 'error'>('success');

	const scopes = [
		{ value: 'all', label: 'All Users' },
		{ value: 'group:moderator', label: 'Moderators' },
		{ value: 'group:admin', label: 'Admins' },
		{ value: 'geo:NG', label: 'Nigeria' },
		{ value: 'geo:US', label: 'United States' },
		{ value: 'geo:GB', label: 'United Kingdom' }
	];

	async function sendBroadcast() {
		if (!title || !message) return;
		sending = true;
		resultMsg = '';
		try {
			await api('/admin/broadcast', {
				method: 'POST',
				body: {
					title,
					message,
					target_scope: targetScope,
					scheduled_at: scheduledAt || null
				}
			});
			resultType = 'success';
			resultMsg = scheduledAt ? 'Broadcast scheduled' : 'Broadcast sent';
			title = '';
			message = '';
			scheduledAt = '';
		} catch {
			resultType = 'error';
			resultMsg = 'Failed to send broadcast';
		} finally {
			sending = false;
		}
	}
</script>

<svelte:head>
	<title>Broadcast — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Notification Broadcast</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">Send targeted notifications with optional scheduling</p>

<div class="mx-auto max-w-2xl rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<div class="space-y-4">
		<div>
			<label for="bc-title" class="mb-1 block text-sm font-medium">Title</label>
			<input
				id="bc-title"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				placeholder="Notification title"
				bind:value={title}
			/>
		</div>

		<div>
			<label for="bc-message" class="mb-1 block text-sm font-medium">Message</label>
			<textarea
				id="bc-message"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				rows="4"
				placeholder="Notification body"
				bind:value={message}
			></textarea>
		</div>

		<div>
			<label for="bc-target" class="mb-1 block text-sm font-medium">Target Audience</label>
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
			<label for="bc-schedule" class="mb-1 block text-sm font-medium">Schedule (optional)</label>
			<input
				id="bc-schedule"
				type="datetime-local"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				bind:value={scheduledAt}
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">Leave empty to send immediately</p>
		</div>

		<div class="flex items-center gap-3 pt-2">
			<button
				class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				disabled={sending || !title || !message}
				onclick={sendBroadcast}
			>
				{sending ? 'Sending...' : scheduledAt ? 'Schedule Broadcast' : 'Send Now'}
			</button>
			{#if resultMsg}
				<span class="text-sm font-medium" style="color: {resultType === 'success' ? 'var(--color-success)' : 'var(--color-danger)'}">{resultMsg}</span>
			{/if}
		</div>
	</div>
</div>
