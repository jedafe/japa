<script lang="ts">
	import { onMount } from 'svelte';
	import { api, i18nAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, RefreshCw } from 'lucide-svelte';

	type I18NString = {
		id: string;
		key: string;
		locale: string;
		value: string;
		updated_at: string;
	};

	let strings = $state<I18NString[]>([]);
	let languages = $state<Array<{ code: string; name: string }>>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let selectedLocale = $state('en');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const [stringsRes, langsRes] = await Promise.all([
				api<I18NString[]>('/admin/i18n/strings'),
				api<Array<{ code: string; name: string }>>('/admin/i18n/languages')
			]);
			if (stringsRes.ok) strings = stringsRes.data ?? [];
			if (langsRes.ok) languages = langsRes.data ?? [];
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('i18n.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('i18n.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('i18n.description')}</p>

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
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('i18n.key')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('i18n.locale')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('i18n.value')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('i18n.updated')}</th>
				</tr>
			</thead>
			<tbody>
				{#each strings as s (s.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-mono text-xs">{s.key}</td>
						<td class="px-4 py-3 text-xs">{s.locale}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{s.value.slice(0, 80)}{s.value.length > 80 ? '...' : ''}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(s.updated_at).toLocaleDateString()}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('i18n.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
