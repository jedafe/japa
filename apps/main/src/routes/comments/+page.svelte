<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, MessageSquare } from 'lucide-svelte';

	type Comment = {
		id: string;
		content: string;
		author_id: string;
		created_at: string;
	};

	let comments = $state<Comment[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			comments = await request<Comment[]>('/comments');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('comments.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('comments.title')}</h1>

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
	<div class="space-y-3">
		{#each comments as comment (comment.id)}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
				<div class="flex items-start gap-3">
					<MessageSquare class="mt-0.5 h-4 w-4" style="color: var(--color-muted)" />
					<div class="flex-1">
						<p class="text-sm">{comment.content}</p>
						<p class="text-xs" style="color: var(--color-muted)">by {comment.author_id} • {new Date(comment.created_at).toLocaleString()}</p>
					</div>
				</div>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('comments.empty')}</p>
		{/each}
	</div>
{/if}
