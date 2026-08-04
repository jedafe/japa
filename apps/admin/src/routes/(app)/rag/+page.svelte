<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Brain, RefreshCw, Trash2, Database, FileText } from 'lucide-svelte';
	import { api } from '$lib/api';

	type EntityTypeStats = {
		entity_type: string;
		chunk_count: number;
	};

	type IndexStats = {
		total_chunks: number;
		total_entities: number;
		last_indexed_at: string;
		by_type: EntityTypeStats[];
	};

	const canManage = $derived(hasPermission('ai.rag.manage'));

	let stats = $state<IndexStats | null>(null);
	let loading = $state(true);
	let error = $state('');
	let reindexing = $state(false);
	let message = $state('');
	let messageType = $state('');

	async function loadStats() {
		loading = true;
		error = '';
		try {
			const result = await api<IndexStats>('/admin/rag/status');
			if (result.ok) {
				stats = result.data;
			} else {
				error = result.error;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('rag.load_error', 'Failed to load RAG status');
		} finally {
			loading = false;
		}
	}

	async function triggerReindex() {
		reindexing = true;
		message = '';
		try {
			const result = await api('/admin/rag/reindex', { method: 'POST' });
			if (result.ok) {
				messageType = 'success';
				message = t('rag.reindex_queued', 'Re-index queued. This may take several minutes.');
				setTimeout(() => loadStats(), 3000);
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('rag.reindex_error', 'Failed to queue re-index');
		} finally {
			reindexing = false;
		}
	}

	async function purgeEntity(entityType: string, entityId: string) {
		try {
			const result = await api(`/admin/rag/entity/${entityType}/${entityId}`, { method: 'DELETE' });
			if (result.ok) {
				messageType = 'success';
				message = t('rag.purge_success', 'Entity embeddings purged.');
				loadStats();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('rag.purge_error', 'Failed to purge entity');
		}
	}

	loadStats();
</script>

<PageHeader
	title={t('rag.title', 'RAG / Knowledge Index')}
	description={t('rag.description', 'Manage the retrieval-augmented generation index status and trigger re-indexing.')}
/>

{#if message}
	<div
		class="mb-4 rounded-lg border p-3 text-sm"
		style={messageType === 'success'
			? 'border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)'
			: 'border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)'}
	>
		{message}
	</div>
{/if}

{#if loading}
	<div class="flex items-center gap-2 p-8" style="color: var(--color-muted)">
		<RefreshCw class="h-4 w-4 animate-spin" />
		<span>{t('rag.loading', 'Loading RAG status...')}</span>
	</div>
{:else if error}
	<div class="rounded-lg border p-4" style="border-color: var(--color-danger); color: var(--color-danger)">
		{error}
	</div>
{:else if stats}
	<div class="grid gap-4 md:grid-cols-3">
		<Card>
			<CardHeader>
				<div class="flex items-center gap-2">
					<Database class="h-5 w-5" style="color: var(--color-primary)" />
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('rag.total_chunks', 'Total Chunks')}</span>
				</div>
			</CardHeader>
			<CardContent>
				<p class="text-3xl font-bold" style="color: var(--color-foreground)">{stats.total_chunks.toLocaleString()}</p>
			</CardContent>
		</Card>
		<Card>
			<CardHeader>
				<div class="flex items-center gap-2">
					<FileText class="h-5 w-5" style="color: var(--color-primary)" />
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('rag.total_entities', 'Total Entities')}</span>
				</div>
			</CardHeader>
			<CardContent>
				<p class="text-3xl font-bold" style="color: var(--color-foreground)">{stats.total_entities.toLocaleString()}</p>
			</CardContent>
		</Card>
		<Card>
			<CardHeader>
				<div class="flex items-center gap-2">
					<Brain class="h-5 w-5" style="color: var(--color-primary)" />
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('rag.last_indexed', 'Last Indexed')}</span>
				</div>
			</CardHeader>
			<CardContent>
				<p class="text-sm font-medium" style="color: var(--color-foreground)">
					{stats.last_indexed_at ? new Date(stats.last_indexed_at).toLocaleString() : '—'}
				</p>
			</CardContent>
		</Card>
	</div>

	<div class="mt-6 flex gap-3">
		{#if canManage}
			<Button onclick={triggerReindex} disabled={reindexing}>
				<RefreshCw class="h-4 w-4 {reindexing ? 'animate-spin' : ''}" />
				{reindexing ? t('rag.reindexing', 'Re-indexing...') : t('rag.trigger_reindex', 'Trigger Re-index')}
			</Button>
		{/if}
	</div>

	{#if stats.by_type && stats.by_type.length > 0}
		<div class="mt-8">
			<h3 class="mb-3 text-sm font-semibold" style="color: var(--color-foreground)">{t('rag.by_type', 'Chunks by Entity Type')}</h3>
			<div class="overflow-hidden rounded-lg border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead style="background: var(--color-surface-alt)">
						<tr>
							<th class="px-4 py-2 text-left font-medium" style="color: var(--color-muted)">{t('rag.entity_type', 'Entity Type')}</th>
							<th class="px-4 py-2 text-right font-medium" style="color: var(--color-muted)">{t('rag.chunk_count', 'Chunk Count')}</th>
						</tr>
					</thead>
					<tbody>
						{#each stats.by_type as et}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="px-4 py-2" style="color: var(--color-foreground)">
									<Badge variant="outline">{et.entity_type}</Badge>
								</td>
								<td class="px-4 py-2 text-right font-mono" style="color: var(--color-foreground)">{et.chunk_count.toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
{/if}
