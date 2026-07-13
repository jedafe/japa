<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, GraduationCap } from 'lucide-svelte';

	type Enrollment = {
		id: string;
		track_title: string;
		progress_percent: number;
		enrolled_at: string;
	};

	let { data } = $props();
	let enrollments = $derived(data.data ?? []);
</script>

<svelte:head>
	<title>{t('me.enrollments.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('me.enrollments.title')}</h1>

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={() => invalidateAll()}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if !data.data}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="space-y-2">
		{#each enrollments as e (e.id)}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
				<div class="flex items-center justify-between">
					<div class="flex items-center gap-3">
						<GraduationCap class="h-4 w-4" style="color: var(--color-primary)" />
						<p class="font-medium">{e.track_title}</p>
					</div>
					<span class="text-xs" style="color: var(--color-muted)">{e.progress_percent}%</span>
				</div>
				<div class="mt-2 h-1.5 w-full rounded-full" style="background: var(--color-border)">
					<div class="h-1.5 rounded-full" style="width: {e.progress_percent}%; background: var(--color-primary)"></div>
				</div>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('me.enrollments.empty')}</p>
		{/each}
	</div>
{/if}
