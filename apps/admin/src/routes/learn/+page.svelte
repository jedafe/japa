<script lang="ts">
	import { api, learnAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Eye, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';

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

	let searchQuery = $state('');

	const columns = [
		{ key: 'title', label: t('learn.title'), sortable: true },
		{ key: 'slug', label: t('learn.slug'), sortable: true, class: 'text-xs font-mono', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` },
		{ key: 'enrollment_count', label: t('learn.enrollments'), sortable: true, class: 'text-xs' },
		{ key: 'is_published', label: t('learn.status'), sortable: true, render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-warning/15 text-warning dark:text-warning'}">${v ? t('learn.published') : t('learn.draft')}</span>` },
		{ key: 'actions', label: t('common.actions'), class: 'w-32 text-right', component: TrackActionsCell }
	];

	function filteredTracks(): Track[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return tracks;
		return tracks.filter(tr =>
			tr.title.toLowerCase().includes(q) ||
			tr.slug.toLowerCase().includes(q)
		);
	}

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

	$effect(() => {
		load();
	});
</script>

{#snippet TrackActionsCell(row: Track)}
	<div class="flex items-center justify-end gap-1">
		{#if !row.is_published}
			<Button variant="ghost" size="sm" onclick={() => publishTrack(row)}>
				{t('learn.publish')}
			</Button>
		{/if}
		<Button variant="ghost" size="sm" onclick={() => viewEnrollments(row)}>
			<Eye class="h-3 w-3" style="color: var(--color-info)" />
		</Button>
		<Button variant="ghost" size="sm" onclick={() => deleteTrack(row.id)}>
			<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
		</Button>
	</div>
{/snippet}

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

<DataTable
	columns={columns}
	data={filteredTracks()}
	{loading}
	pageSize={25}
	emptyMessage={t('learn.empty')}
	searchable
	searchPlaceholder={t('common.search')}
	onsearch={(q) => { searchQuery = q; }}
/>

<!-- Enrollments Modal -->
<Modal bind:open={showEnrollments} title={t('learn.enrollments')} onConfirm={() => { showEnrollments = false; }} confirmLabel={t('common.close')}>
	{#if selectedTrack}
		<p class="text-sm mb-3" style="color: var(--color-muted)">{selectedTrack.title}</p>
		{#if enrollments.length === 0}
			<p class="text-sm" style="color: var(--color-muted)">{t('learn.no_enrollments')}</p>
		{:else}
			<div class="max-h-96 overflow-y-auto rounded-md border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr style="border-bottom: 1px solid var(--color-border)">
							<th class="px-4 py-2 text-left font-medium text-xs" style="color: var(--color-muted)">{t('learn.user_id')}</th>
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
