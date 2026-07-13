<script lang="ts">
	import { onMount } from 'svelte';
	import { api, learnAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Eye, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Track = {
		id: string;
		title: string;
		slug: string;
		description?: string;
		is_published: boolean;
		enrollment_count: number;
		created_at: string;
	};

	let tracks = $state<Track[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let selectedTrack = $state<Track | null>(null);
	let enrollments = $state<{ user_id: string; enrolled_at: string }[]>([]);
	let showEnrollments = $state(false);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Track[]>('/admin/learn/tracks');
			if (result.ok) {
				tracks = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function viewEnrollments(track: Track) {
		errorMessage = '';
		try {
			const result = await api<{ user_id: string; enrolled_at: string }[]>(`/admin/learn/tracks/${track.id}/enrollments`);
			if (result.ok) {
				selectedTrack = track;
				enrollments = result.data ?? [];
				showEnrollments = true;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function publishTrack(track: Track) {
		errorMessage = '';
		try {
			await api(`/admin/learn/tracks/${track.id}/publish`, { method: 'POST', body: {} });
			load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function deleteTrack(id: string) {
		errorMessage = '';
		try {
			await api(`/admin/learn/tracks/${id}`, { method: 'DELETE' });
			tracks = tracks.filter((t) => t.id !== id);
		} catch {
			errorMessage = t('common.error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('learn.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('learn.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('learn.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('learn.title')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('learn.slug')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('learn.enrollments')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('learn.status')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each tracks as track (track.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{track.title}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{track.slug}</td>
						<td class="px-4 py-3 text-xs">{track.enrollment_count}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {track.is_published ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-amber-500/15 text-amber-700 dark:text-amber-400'}">
								{track.is_published ? t('learn.published') : t('learn.draft')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								{#if !track.is_published}
									<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => publishTrack(track)}>
										{t('learn.publish')}
									</button>
								{/if}
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => viewEnrollments(track)}>
									<Eye class="h-3 w-3" style="color: var(--color-info)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => deleteTrack(track.id)}>
									<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('learn.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Enrollments Modal -->
<Modal bind:open={showEnrollments} title={t('learn.enrollments')} onConfirm={() => showEnrollments = false} confirmLabel={t('common.close')}>
	{#if selectedTrack}
		<p class="text-sm mb-3" style="color: var(--color-muted)">{selectedTrack.title}</p>
		{#if enrollments.length === 0}
			<p class="text-sm" style="color: var(--color-muted)">{t('learn.no_enrollments')}</p>
		{:else}
			<div class="max-h-96 overflow-y-auto rounded-md border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr style="border-bottom: 1px solid var(--color-border)">
							<th class="px-4 py-2 text-left font-medium text-xs" style="color: var(--color-muted)">User ID</th>
							<th class="px-4 py-2 text-left font-medium text-xs" style="color: var(--color-muted)">{t('learn.enrolled_at')}</th>
						</tr>
					</thead>
					<tbody>
						{#each enrollments as e (e.user_id)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="px-4 py-2 text-xs font-mono">{e.user_id}</td>
								<td class="px-4 py-2 text-xs" style="color: var(--color-muted)">{new Date(e.enrolled_at).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</Modal>
