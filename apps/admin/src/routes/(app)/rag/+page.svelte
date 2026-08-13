<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Brain, RefreshCw, Trash2, Database, FileText, Settings } from 'lucide-svelte';
	import { Modal } from '$lib/components/ui/modal';
	import { api } from '$lib/api';

	type EntityTypeStats = {
		entity_type: string;
		chunk_count: number;
		entity_count: number;
	};

	type IndexStats = {
		total_chunks: number;
		total_entities: number;
		last_indexed_at: string;
		by_type: EntityTypeStats[];
	};

	type RagSettings = {
		'rag.enabled': boolean;
		'rag.embedding_model': string;
		'rag.retrieval_top_k': number;
		'rag.chunk_max_tokens': number;
	};

	const canManage = $derived(hasPermission('ai.rag.manage'));

	let stats = $state<IndexStats | null>(null);
	let loading = $state(true);
	let error = $state('');
	let reindexing = $state(false);
	let message = $state('');
	let messageType = $state('');

	// Purge confirmation state — the by_type table is an aggregate view, so
	// the purge button purges ALL embeddings for the entity type. A
	// confirmation modal is shown before the destructive DELETE.
	let purgeModalOpen = $state(false);
	let pendingPurgeType = $state('');

	// Settings state
	let settings = $state<RagSettings | null>(null);
	let settingsLoading = $state(false);
	let settingsSaving = $state(false);
	let settingsForm = $state({
		enabled: true,
		embedding_model: 'text-embedding-3-small',
		retrieval_top_k: 5,
		chunk_max_tokens: 400
	});

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
			error = e instanceof Error ? e.message : t('rag.load_error');
		} finally {
			loading = false;
		}
	}

	async function loadSettings() {
		settingsLoading = true;
		try {
			const result = await api<RagSettings>('/admin/settings/rag');
			if (result.ok) {
				settings = result.data;
				settingsForm = {
					enabled: result.data['rag.enabled'],
					embedding_model: result.data['rag.embedding_model'],
					retrieval_top_k: result.data['rag.retrieval_top_k'],
					chunk_max_tokens: result.data['rag.chunk_max_tokens']
				};
			}
		} catch (e) {
			// Settings load is best-effort — the status dashboard still works.
			console.error('Failed to load RAG settings', e);
		} finally {
			settingsLoading = false;
		}
	}

	async function saveSettings() {
		settingsSaving = true;
		message = '';
		try {
			const result = await api('/admin/settings/rag', {
				method: 'PUT',
				body: JSON.stringify({
					enabled: settingsForm.enabled,
					embedding_model: settingsForm.embedding_model,
					retrieval_top_k: settingsForm.retrieval_top_k,
					chunk_max_tokens: settingsForm.chunk_max_tokens
				})
			});
			if (result.ok) {
				messageType = 'success';
				message = t('rag.settings_saved');
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('rag.settings_error');
		} finally {
			settingsSaving = false;
		}
	}

	async function triggerReindex() {
		reindexing = true;
		message = '';
		try {
			const result = await api('/admin/rag/reindex', { method: 'POST' });
			if (result.ok) {
				messageType = 'success';
				message = t('rag.reindex_queued');
				setTimeout(() => loadStats(), 3000);
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('rag.reindex_error');
		} finally {
			reindexing = false;
		}
	}

	async function confirmPurgeType(entityType: string) {
		pendingPurgeType = entityType;
		purgeModalOpen = true;
	}

	async function executePurge() {
		const entityType = pendingPurgeType;
		purgeModalOpen = false;
		pendingPurgeType = '';
		try {
			// BUG-004 fix: the by_type table is an aggregate view. The DELETE
			// /admin/rag/entity/{type}/{id} route expects a specific entity
			// UUID, not a type. We use a sentinel "all" id to signal a
			// type-level purge; the backend handler treats id="all" as
			// "purge all embeddings for this entity_type".
			const result = await api(`/admin/rag/entity/${entityType}/all`, { method: 'DELETE' });
			if (result.ok) {
				messageType = 'success';
				message = t('rag.purge_success');
				loadStats();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('rag.purge_error');
		}
	}

	loadStats();
	loadSettings();
</script>

<PageHeader
	title={t('rag.title')}
	description={t('rag.description')}
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
		<span>{t('rag.loading')}</span>
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
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('rag.total_chunks')}</span>
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
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('rag.total_entities')}</span>
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
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('rag.last_indexed')}</span>
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
				{reindexing ? t('rag.reindexing') : t('rag.trigger_reindex')}
			</Button>
		{/if}
	</div>

	{#if stats.by_type && stats.by_type.length > 0}
		<div class="mt-8">
			<h3 class="mb-3 text-sm font-semibold" style="color: var(--color-foreground)">{t('rag.by_type')}</h3>
			<div class="overflow-hidden rounded-lg border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead style="background: var(--color-surface-alt)">
						<tr>
							<th class="px-4 py-2 text-left font-medium" style="color: var(--color-muted)">{t('rag.entity_type')}</th>
							<th class="px-4 py-2 text-right font-medium" style="color: var(--color-muted)">{t('rag.total_entities')}</th>
							<th class="px-4 py-2 text-right font-medium" style="color: var(--color-muted)">{t('rag.chunk_count')}</th>
							{#if canManage}
								<th class="px-4 py-2 text-right font-medium" style="color: var(--color-muted)">{t('rag.purge')}</th>
							{/if}
						</tr>
					</thead>
					<tbody>
						{#each stats.by_type as et (et.entity_type)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="px-4 py-2" style="color: var(--color-foreground)">
									<Badge variant="outline">{et.entity_type}</Badge>
								</td>
								<td class="px-4 py-2 text-right font-mono" style="color: var(--color-foreground)">{et.entity_count.toLocaleString()}</td>
								<td class="px-4 py-2 text-right font-mono" style="color: var(--color-foreground)">{et.chunk_count.toLocaleString()}</td>
								{#if canManage}
									<td class="px-4 py-2 text-right">
										<Button
											variant="destructive"
											size="sm"
											onclick={() => confirmPurgeType(et.entity_type)}
										>
											<Trash2 class="h-3 w-3" />
											{t('rag.purge')}
										</Button>
									</td>
								{/if}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
{/if}

{#if purgeModalOpen}
	<Modal bind:open={purgeModalOpen} title={t('rag.purge')} description={t('rag.confirm_purge')} size="sm">
		<div class="flex justify-end gap-2 pt-4">
			<Button variant="outline" onclick={() => { purgeModalOpen = false; pendingPurgeType = ''; }}>
				{t('rag.cancel_purge', 'Cancel')}
			</Button>
			<Button variant="destructive" onclick={executePurge}>
				<Trash2 class="h-4 w-4" />
				{t('rag.purge')}
			</Button>
		</div>
	</Modal>
{/if}

{#if canManage && !settingsLoading && settings}
	<div class="mt-8">
		<div class="mb-3 flex items-center gap-2">
			<Settings class="h-4 w-4" style="color: var(--color-primary)" />
			<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">{t('rag.settings')}</h3>
		</div>
		<Card>
			<CardContent>
				<form
					class="grid gap-4 pt-4"
					onsubmit={(e) => {
						e.preventDefault();
						saveSettings();
					}}
				>
					<div class="flex items-center justify-between">
						<div>
							<label class="text-sm font-medium" style="color: var(--color-foreground)" for="rag-enabled">
								{t('rag.setting_enabled')}
							</label>
							<p class="text-xs" style="color: var(--color-muted)">{t('rag.setting_enabled_help')}</p>
						</div>
						<input
							id="rag-enabled"
							type="checkbox"
							bind:checked={settingsForm.enabled}
							class="h-4 w-4"
						/>
					</div>

					<div class="grid gap-1">
						<label class="text-sm font-medium" style="color: var(--color-foreground)" for="rag-model">
							{t('rag.setting_embedding_model')}
						</label>
						<p class="text-xs" style="color: var(--color-muted)">{t('rag.setting_embedding_model_help')}</p>
						<input
							id="rag-model"
							type="text"
							bind:value={settingsForm.embedding_model}
							placeholder="text-embedding-3-small"
							class="rounded-md border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>

					<div class="grid gap-1">
						<label class="text-sm font-medium" style="color: var(--color-foreground)" for="rag-topk">
							{t('rag.setting_retrieval_top_k')}
						</label>
						<p class="text-xs" style="color: var(--color-muted)">{t('rag.setting_retrieval_top_k_help')}</p>
						<input
							id="rag-topk"
							type="number"
							min="1"
							max="50"
							bind:value={settingsForm.retrieval_top_k}
							class="rounded-md border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>

					<div class="grid gap-1">
						<label class="text-sm font-medium" style="color: var(--color-foreground)" for="rag-chunk">
							{t('rag.setting_chunk_max_tokens')}
						</label>
						<p class="text-xs" style="color: var(--color-muted)">{t('rag.setting_chunk_max_tokens_help')}</p>
						<input
							id="rag-chunk"
							type="number"
							min="50"
							max="2000"
							bind:value={settingsForm.chunk_max_tokens}
							class="rounded-md border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						/>
					</div>

					<div>
						<Button type="submit" disabled={settingsSaving}>
							{settingsSaving ? t('rag.saving') : t('rag.save_settings')}
						</Button>
					</div>
				</form>
			</CardContent>
		</Card>
	</div>
{/if}
