<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Wand, Plus, RefreshCw, BarChart3 } from 'lucide-svelte';
	import { api } from '$lib/api';

	type StudioJob = {
		id: string;
		batch_id: string;
		persona_id: string;
		content_type: string;
		topic_prompt: string;
		status: string;
		generated_at: string | null;
		reviewed_at: string | null;
		published_at: string | null;
		created_at: string;
	};

	type StudioBatch = {
		id: string;
		name: string;
		description: string | null;
		created_by: string;
		created_at: string;
	};

	type StudioStats = {
		total_jobs: number;
		queued: number;
		generating: number;
		draft_ready: number;
		in_review: number;
		published: number;
	};

	const canManage = $derived(hasPermission('studio.manage'));

	let activeTab = $state('jobs');
	let jobs = $state<StudioJob[]>([]);
	let batches = $state<StudioBatch[]>([]);
	let stats = $state<StudioStats | null>(null);
	let loading = $state(true);
	let error = $state('');
	let createJobModalOpen = $state(false);
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');
	let personas = $state<{ id: string; label: string }[]>([]);
	let contentTypes = $state<string[]>([]);

	let newJob = $state({
		persona_id: '',
		content_type: 'blog',
		topic_prompt: '',
		guide_category: '',
		target_space_id: '',
		target_tags: ''
	});

	const tabs = $derived([
		{ value: 'jobs', label: t('studio.jobs', 'Jobs'), icon: Wand },
		{ value: 'batches', label: t('studio.batches', 'Batches'), icon: BarChart3 },
		{ value: 'stats', label: t('studio.stats', 'Statistics'), icon: BarChart3 }
	]);

	async function load() {
		loading = true;
		error = '';
		try {
			const [jobsRes, batchesRes, statsRes] = await Promise.all([
				api<StudioJob[]>('/admin/studio/jobs'),
				api<StudioBatch[]>('/admin/studio/batches'),
				api<StudioStats>('/admin/studio/stats')
			]);
			if (jobsRes.ok) {
				jobs = jobsRes.data ?? [];
			}
			if (batchesRes.ok) {
				batches = batchesRes.data ?? [];
			}
			if (statsRes.ok) {
				stats = statsRes.data;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('studio.load_error', 'Failed to load studio data');
		} finally {
			loading = false;
		}
	}

	async function loadCreateOptions() {
		try {
			const [personasRes, typesRes] = await Promise.all([
				api<{ id: string; label: string }[]>('/admin/studio/personas'),
				api<string[]>('/admin/studio/content-types')
			]);
			if (personasRes.ok) {
				personas = personasRes.data ?? [];
			}
			if (typesRes.ok) {
				contentTypes = typesRes.data ?? [];
			}
		} catch {
			// Non-critical — form will use defaults
		}
	}

	async function retryJob(id: string) {
		try {
			const result = await api(`/admin/studio/jobs/${id}/retry`, { method: 'POST' });
			if (result.ok) {
				messageType = 'success';
				message = t('studio.retry_success', 'Job retried.');
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('studio.retry_error', 'Failed to retry job');
		}
	}

	async function cancelJob(id: string) {
		try {
			const result = await api(`/admin/studio/jobs/${id}`, { method: 'DELETE' });
			if (result.ok) {
				messageType = 'success';
				message = t('studio.cancel_success', 'Job cancelled.');
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('studio.cancel_error', 'Failed to cancel job');
		}
	}

	async function createJob() {
		saving = true;
		message = '';
		try {
			const body = {
				persona_id: newJob.persona_id,
				content_type: newJob.content_type,
				topic_prompt: newJob.topic_prompt,
				guide_category: newJob.guide_category || undefined,
				target_space_id: newJob.target_space_id || undefined,
				target_tags: newJob.target_tags ? newJob.target_tags.split(',').map((s: string) => s.trim()).filter(Boolean) : []
			};
			const result = await api('/admin/studio/jobs', { method: 'POST', body });
			if (result.ok) {
				messageType = 'success';
				message = t('studio.create_success', 'Studio job created.');
				createJobModalOpen = false;
				newJob = { persona_id: '', content_type: 'blog', topic_prompt: '', guide_category: '', target_space_id: '', target_tags: '' };
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('studio.create_error', 'Failed to create job');
		} finally {
			saving = false;
		}
	}

	const jobColumns = $derived([
		{
			key: 'id',
			label: t('studio.job_id', 'Job ID'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs text-foreground">${value?.slice(0, 8) ?? '—'}…</span>`
		},
		{
			key: 'content_type',
			label: t('studio.content_type', 'Content Type'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground">${value}</span>`
		},
		{
			key: 'topic_prompt',
			label: t('studio.topic', 'Topic'),
			render: (value: string) => `<span class="text-muted-foreground text-xs truncate max-w-xs block">${value ?? '—'}</span>`
		},
		{
			key: 'status',
			label: t('studio.status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const clsMap: Record<string, string> = {
					queued: 'bg-muted text-muted-foreground border-border',
					generating: 'bg-warning/15 text-warning border-warning',
					draft_ready: 'bg-primary/15 text-primary border-primary',
					in_review: 'bg-warning/15 text-warning border-warning',
					published: 'bg-success/15 text-success border-success',
					failed: 'bg-danger/15 text-danger border-danger',
					cancelled: 'bg-muted text-muted-foreground border-border'
				};
				const cls = clsMap[value] ?? 'bg-muted text-muted-foreground border-border';
				return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${value}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('studio.created', 'Created'),
			sortable: true,
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString() : '—'}</span>`
		}
	]);

	const batchColumns = $derived([
		{
			key: 'id',
			label: t('studio.batch_id', 'Batch ID'),
			sortable: true,
			render: (value: string) => `<span class="font-mono text-xs text-foreground">${value?.slice(0, 8) ?? '—'}…</span>`
		},
		{
			key: 'name',
			label: t('studio.batch_name', 'Name'),
			sortable: true,
			render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
		},
		{
			key: 'description',
			label: t('studio.description', 'Description'),
			render: (value: string | null) => `<span class="text-muted-foreground text-xs">${value ?? '—'}</span>`
		},
		{
			key: 'created_at',
			label: t('studio.created', 'Created'),
			sortable: true,
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString() : '—'}</span>`
		}
	]);

	load();
</script>

<PageHeader
	title={t('studio.title', 'AI Content Studio')}
	description={t('studio.description', 'Manage AI-generated content jobs and batches.')}
	actions={canManage ? [
		{ label: t('studio.create_job', 'Create Job'), icon: Plus, onclick: () => { loadCreateOptions(); createJobModalOpen = true; } }
	] : []}
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
		<span>{t('studio.loading', 'Loading studio data...')}</span>
	</div>
{:else if error}
	<div class="rounded-lg border p-4" style="border-color: var(--color-danger); color: var(--color-danger)">
		{error}
	</div>
{:else}
	<Tabs {tabs} bind:value={activeTab} />

	{#if activeTab === 'jobs'}
		<DataTable
			data={jobs}
			columns={jobColumns}
			emptyMessage={t('studio.no_jobs', 'No studio jobs found.')}
		/>
	{:else if activeTab === 'batches'}
		<DataTable
			data={batches}
			columns={batchColumns}
			emptyMessage={t('studio.no_batches', 'No batches found.')}
		/>
	{:else if activeTab === 'stats' && stats}
		<div class="grid gap-4 md:grid-cols-3">
			<Card>
				<CardHeader>
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('studio.total_jobs', 'Total Jobs')}</span>
				</CardHeader>
				<CardContent>
					<p class="text-3xl font-bold" style="color: var(--color-foreground)">{stats.total_jobs}</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader>
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('studio.queued', 'Queued')}</span>
				</CardHeader>
				<CardContent>
					<p class="text-3xl font-bold" style="color: var(--color-foreground)">{stats.queued}</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader>
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('studio.published', 'Published')}</span>
				</CardHeader>
				<CardContent>
					<p class="text-3xl font-bold" style="color: var(--color-foreground)">{stats.published}</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader>
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('studio.generating', 'Generating')}</span>
				</CardHeader>
				<CardContent>
					<p class="text-3xl font-bold" style="color: var(--color-foreground)">{stats.generating}</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader>
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('studio.draft_ready', 'Draft Ready')}</span>
				</CardHeader>
				<CardContent>
					<p class="text-3xl font-bold" style="color: var(--color-foreground)">{stats.draft_ready}</p>
				</CardContent>
			</Card>
			<Card>
				<CardHeader>
					<span class="text-sm font-medium" style="color: var(--color-muted)">{t('studio.in_review', 'In Review')}</span>
				</CardHeader>
				<CardContent>
					<p class="text-3xl font-bold" style="color: var(--color-foreground)">{stats.in_review}</p>
				</CardContent>
			</Card>
		</div>
	{/if}
{/if}

{#if createJobModalOpen}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		onclick={() => (createJobModalOpen = false)}
		onkeydown={(e) => e.key === 'Escape' && (createJobModalOpen = false)}>
		<div role="presentation" class="w-full max-w-lg rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
			<h2 class="mb-4 text-lg font-semibold" style="color: var(--color-foreground)">{t('studio.create_job_title', 'Create Studio Job')}</h2>
			<div class="space-y-3">
				<div>
					<label for="studio-persona" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.persona', 'Persona')}</label>
					<select id="studio-persona" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newJob.persona_id}>
						<option value="">{t('studio.select_persona', 'Select a persona...')}</option>
						{#each personas as p}
							<option value={p.id}>{p.label}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="studio-content-type" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.content_type', 'Content Type')}</label>
					<select id="studio-content-type" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newJob.content_type}>
						{#each contentTypes as ct}
							<option value={ct}>{ct}</option>
						{/each}
						{#if contentTypes.length === 0}
							<option value="blog">blog</option>
							<option value="guide">guide</option>
						{/if}
					</select>
				</div>
				<div>
					<label for="studio-topic" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.topic_prompt', 'Topic Prompt')}</label>
					<textarea id="studio-topic" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="3" bind:value={newJob.topic_prompt} placeholder={t('studio.topic_placeholder', 'Describe what the AI should write about...')}></textarea>
				</div>
				<div>
					<label for="studio-tags" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.target_tags', 'Target Tags (comma-separated)')}</label>
					<input id="studio-tags" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newJob.target_tags} placeholder="tech, ai, tutorial" />
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<Button variant="outline" onclick={() => (createJobModalOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={createJob} disabled={saving || !newJob.persona_id || !newJob.topic_prompt}>
					{saving ? t('common.saving', 'Saving...') : t('studio.create', 'Create Job')}
				</Button>
			</div>
		</div>
	</div>
{/if}
