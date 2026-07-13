<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Image, Trash2 } from 'lucide-svelte';

	type Media = {
		id: string;
		url: string;
		content_type: string;
		size: number;
		created_at: string;
	};

	let media = $state<Media[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			media = (await api.getMyMedia()) as unknown as Media[];
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function deleteMedia(id: string) {
		errorMessage = '';
		try {
			await api.deleteMedia(id);
			media = media.filter((m) => m.id !== id);
		} catch {
			errorMessage = t('common.error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('me.media.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.media.title')}</h1>

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
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each media as m (m.id)}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
				{#if m.content_type.startsWith('image/')}
					<img src={m.url} alt="" class="mb-2 h-32 w-full rounded object-cover" />
				{:else}
					<div class="mb-2 flex h-32 items-center justify-center rounded" style="background: var(--color-surface)">
						<Image class="h-8 w-8" style="color: var(--color-muted)" />
					</div>
				{/if}
				<div class="flex items-center justify-between">
					<p class="text-xs" style="color: var(--color-muted)">{(m.size / 1024).toFixed(1)} KB</p>
					<button class="rounded p-1 hover:bg-black/5" onclick={() => deleteMedia(m.id)}>
						<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
					</button>
				</div>
			</div>
		{:else}
			<p class="col-span-full py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.media.empty')}</p>
		{/each}
	</div>
{/if}
