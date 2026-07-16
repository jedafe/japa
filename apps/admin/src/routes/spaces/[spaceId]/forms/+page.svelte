<script lang="ts">
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type FormDef = {
		id: string;
		name: string;
		description?: string;
		submission_count?: number;
		created_at: string;
	};

	const spaceId = $derived(page.params.spaceId);

	let forms = $state<FormDef[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<{ data: FormDef[] }>(`/forms?space_id=${spaceId}`);
			if (result.ok) {
				forms = Array.isArray(result.data) ? result.data : (result.data.data ?? []);
			}
		} catch {
			errorMessage = t('space_forms.load_error');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>Space Forms — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('space_forms.title')}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('space_forms.description')}</p>
	</div>
	<a
		href="/spaces/{spaceId}/forms/new"
		class="inline-flex items-center rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
	>
		{t('space_forms.create')}
	</a>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if forms.length === 0}
	<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<p class="text-sm" style="color: var(--color-muted)">{t('space_forms.empty')}</p>
	</div>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space_forms.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space_forms.description_label')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space_forms.submissions')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space_forms.created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each forms as form}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{form.name}</td>
						<td class="px-4 py-3 text-xs max-w-xs truncate" style="color: var(--color-muted)">{form.description || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-black/5 dark:bg-white/10">{form.submission_count}</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{form.created_at}</td>
						<td class="px-4 py-3">
							<a
								href="/spaces/{spaceId}/forms/{form.id}"
								class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-info)"
							>
								View
							</a>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('space_forms.no_results')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
