<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Tabs } from '$lib/components/ui/tabs';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Wand, Plus, RefreshCw, BarChart3, RotateCcw, X, Eye } from 'lucide-svelte';
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
		rejected: number;
		failed: number;
		publish_rate: number;
		avg_review_hours: number;
	};

	type ContentTypeOption = {
		type: string;
		label: string;
		guide_categories?: string[];
		can_auto_publish: boolean;
	};

	const canManage = $derived(hasPermission('studio.manage'));
	const canCreate = $derived(hasPermission('studio.create') || hasPermission('studio.manage'));

	let activeTab = $state('jobs');
	let jobs = $state<StudioJob[]>([]);
	let batches = $state<StudioBatch[]>([]);
	let stats = $state<StudioStats | null>(null);
	let loading = $state(true);
	let error = $state('');
	let createJobSlideOverOpen = $state(false);
	let saving = $state(false);
	let message = $state('');
	let messageType = $state('');
	let personas = $state<{ id: string; label: string }[]>([]);
	let contentTypes = $state<ContentTypeOption[]>([]);

	let newJob = $state({
		persona_id: '',
		content_type: '',
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
				api<ContentTypeOption[]>('/admin/studio/content-types')
			]);
			if (personasRes.ok) {
				personas = personasRes.data ?? [];
			}
			if (typesRes.ok) {
				contentTypes = typesRes.data ?? [];
				// Auto-select the first content type if none selected yet.
				if (contentTypes.length > 0 && !newJob.content_type) {
					newJob.content_type = contentTypes[0].type;
				}
			}
		} catch {
			// Non-critical — form will show loading state
		}
	}

	// Derive the guide categories for the currently selected content type.
	const selectedContentType = $derived(
		contentTypes.find((ct) => ct.type === newJob.content_type)
	);
	const guideCategories = $derived(selectedContentType?.guide_categories ?? []);

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

	let viewingJob = $state<StudioJob | null>(null);
	let viewJobSlideOverOpen = $state(false);

	let createBatchSlideOverOpen = $state(false);
	let savingBatch = $state(false);
	let newBatch = $state({
		name: '',
		description: '',
		jobs: [] as Array<{ persona_id: string; content_type: string; guide_category: string; topic_prompt: string }>
	});

	type BatchDetail = {
		batch: StudioBatch;
		stats: {
			total: number;
			queued: number;
			generating: number;
			draft_ready: number;
			in_review: number;
			approved: number;
			published: number;
			rejected: number;
			failed: number;
		};
	};
	let viewingBatch = $state<BatchDetail | null>(null);
	let viewBatchSlideOverOpen = $state(false);

	async function viewJob(id: string) {
		try {
			const result = await api<StudioJob>(`/admin/studio/jobs/${id}`);
			if (result.ok && result.data) {
				viewingJob = result.data;
				viewJobSlideOverOpen = true;
			}
		} catch {
			// silent
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
				createJobSlideOverOpen = false;
				newJob = { persona_id: '', content_type: '', topic_prompt: '', guide_category: '', target_space_id: '', target_tags: '' };
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

	function addBatchJobRow() {
		newBatch.jobs = [...newBatch.jobs, { persona_id: '', content_type: '', guide_category: '', topic_prompt: '' }];
	}

	function removeBatchJobRow(idx: number) {
		newBatch.jobs = newBatch.jobs.filter((_, i) => i !== idx);
	}

	async function createBatch() {
		savingBatch = true;
		message = '';
		try {
			const body = {
				name: newBatch.name,
				description: newBatch.description || undefined,
				jobs: newBatch.jobs
					.filter((j) => j.topic_prompt && j.content_type)
					.map((j) => ({
						persona_id: j.persona_id,
						content_type: j.content_type,
						guide_category: j.guide_category || undefined,
						topic_prompt: j.topic_prompt
					}))
			};
			if (!body.name || body.jobs.length === 0) {
				messageType = 'error';
				message = t('studio.batch_validation', 'Batch name and at least one job are required.');
				savingBatch = false;
				return;
			}
			const result = await api('/admin/studio/batches', { method: 'POST', body });
			if (result.ok) {
				messageType = 'success';
				message = t('studio.batch_create_success', 'Batch created.');
				createBatchSlideOverOpen = false;
				newBatch = { name: '', description: '', jobs: [] };
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('studio.batch_create_error', 'Failed to create batch');
		} finally {
			savingBatch = false;
		}
	}

	async function viewBatch(id: string) {
		try {
			const result = await api<BatchDetail>(`/admin/studio/batches/${id}`);
			if (result.ok && result.data) {
				viewingBatch = result.data;
				viewBatchSlideOverOpen = true;
			}
		} catch {
			// silent
		}
	}

	async function retryBatch(id: string) {
		try {
			const result = await api(`/admin/studio/batches/${id}/retry`, { method: 'POST' });
			if (result.ok) {
				messageType = 'success';
				message = t('studio.batch_retry_success', 'Failed jobs retried.');
				load();
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('studio.batch_retry_error', 'Failed to retry batch');
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

	const jobRowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: StudioJob) => viewJob(row.id) },
		{
			label: t('studio.retry', 'Retry'),
			icon: RotateCcw,
			onclick: (row: StudioJob) => retryJob(row.id),
			show: (row: StudioJob) => canManage && row.status === 'failed'
		},
		{
			label: t('common.cancel', 'Cancel'),
			icon: X,
			onclick: (row: StudioJob) => cancelJob(row.id),
			show: (row: StudioJob) => canManage && (row.status === 'queued' || row.status === 'generating')
		}
	];

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

	const batchRowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: StudioBatch) => viewBatch(row.id) },
		{
			label: t('studio.retry_failed', 'Retry Failed'),
			icon: RotateCcw,
			onclick: (row: StudioBatch) => retryBatch(row.id),
			show: () => canManage
		}
	];

	load();
</script>

<PageHeader
	title={t('studio.title', 'AI Content Studio')}
	description={t('studio.description', 'Manage AI-generated content jobs and batches.')}
	actions={canCreate ? [
		{ label: t('studio.create_job', 'Create Job'), icon: Plus, onclick: () => { loadCreateOptions(); createJobSlideOverOpen = true; } },
		{ label: t('studio.create_batch', 'Create Batch'), icon: BarChart3, onclick: () => { loadCreateOptions(); createBatchSlideOverOpen = true; } }
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
			actions={jobRowActions}
			emptyMessage={t('studio.no_jobs', 'No studio jobs found.')}
		/>
	{:else if activeTab === 'batches'}
		<DataTable
			data={batches}
			columns={batchColumns}
			actions={batchRowActions}
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

{#if createJobSlideOverOpen}
	<SlideOver
		bind:open={createJobSlideOverOpen}
		title={t('studio.create_job_title', 'Create Studio Job')}
	>
		<div class="space-y-3">
			<div>
				<label for="studio-persona" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.persona', 'Persona')}</label>
				<select id="studio-persona" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newJob.persona_id}>
					<option value="">{t('studio.select_persona', 'Select a persona...')}</option>
					{#each personas as p (p)}
						<option value={p.id}>{p.label}</option>
					{/each}
				</select>
			</div>
			<div>
				<label for="studio-content-type" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.content_type', 'Content Type')}</label>
				<select id="studio-content-type" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newJob.content_type}>
					{#each contentTypes as ct (ct)}
						<option value={ct.type}>{ct.label}</option>
					{/each}
					{#if contentTypes.length === 0}
						<option value="" disabled>{t('studio.loading_types', 'Loading content types...')}</option>
					{/if}
				</select>
			</div>
			{#if guideCategories.length > 0}
				<div>
					<label for="studio-guide-category" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.guide_category', 'Guide Category')}</label>
					<select id="studio-guide-category" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newJob.guide_category}>
						<option value="">{t('studio.select_category', 'Select category...')}</option>
						{#each guideCategories as cat (cat)}
							<option value={cat}>{cat}</option>
						{/each}
					</select>
				</div>
			{/if}
			<div>
				<label for="studio-topic" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.topic_prompt', 'Topic Prompt')}</label>
				<textarea id="studio-topic" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="3" bind:value={newJob.topic_prompt} placeholder={t('studio.topic_placeholder', 'Describe what the AI should write about...')}></textarea>
			</div>
			<div>
				<label for="studio-tags" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.target_tags', 'Target Tags (comma-separated)')}</label>
				<input id="studio-tags" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newJob.target_tags} placeholder={t('app.studio.ph_tech_ai_tutorial', 'tech, ai, tutorial')} />
			</div>
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				<Button variant="outline" onclick={() => (createJobSlideOverOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={createJob} disabled={saving || !newJob.persona_id || !newJob.topic_prompt}>
					{saving ? t('common.saving', 'Saving...') : t('studio.create', 'Create Job')}
				</Button>
			</div>
		{/snippet}
	</SlideOver>
{/if}

{#if viewJobSlideOverOpen && viewingJob}
	<SlideOver
		bind:open={viewJobSlideOverOpen}
		title={t('studio.job_detail', 'Job Detail')}
	>
		<div class="space-y-3 text-sm">
			<div class="grid gap-2 sm:grid-cols-2">
				<div>
					<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.job_id', 'Job ID')}</span>
					<p class="font-mono text-xs" style="color: var(--color-foreground)">{viewingJob.id}</p>
				</div>
				<div>
					<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.status', 'Status')}</span>
					<p><Badge variant="outline">{viewingJob.status}</Badge></p>
				</div>
				<div>
					<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.content_type', 'Content Type')}</span>
					<p style="color: var(--color-foreground)">{viewingJob.content_type}</p>
				</div>
				<div>
					<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.created', 'Created')}</span>
					<p style="color: var(--color-foreground)">{viewingJob.created_at ? new Date(viewingJob.created_at).toLocaleString() : '—'}</p>
				</div>
			</div>
			<div>
				<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.topic', 'Topic')}</span>
				<p class="mt-1 rounded-md border p-2 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">{viewingJob.topic_prompt || '—'}</p>
			</div>
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				{#if viewingJob && canManage && viewingJob.status === 'failed'}
					<Button variant="outline" onclick={() => { retryJob(viewingJob!.id); viewJobSlideOverOpen = false; }}>
						<RotateCcw class="h-4 w-4" />
						{t('studio.retry', 'Retry')}
					</Button>
				{/if}
				<Button onclick={() => { viewJobSlideOverOpen = false; viewingJob = null; }}>{t('common.close', 'Close')}</Button>
			</div>
		{/snippet}
	</SlideOver>
{/if}

{#if createBatchSlideOverOpen}
	<SlideOver
		bind:open={createBatchSlideOverOpen}
		title={t('studio.create_batch_title', 'Create Studio Batch')}
	>
		<div class="space-y-3">
			<div>
				<label for="batch-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.batch_name', 'Batch Name')}</label>
				<input id="batch-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBatch.name} placeholder={t('studio.batch_name_ph', 'e.g. UK visa guides — Oct 2026')} />
			</div>
			<div>
				<label for="batch-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('studio.description', 'Description')}</label>
				<input id="batch-desc" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={newBatch.description} placeholder={t('studio.batch_desc_ph', 'Optional description')} />
			</div>
			<div class="border-t pt-3" style="border-color: var(--color-border)">
				<div class="mb-2 flex items-center justify-between">
					<span class="text-sm font-medium" style="color: var(--color-foreground)">{t('studio.batch_jobs', 'Jobs in batch')}</span>
					<Button variant="outline" onclick={addBatchJobRow}>
						<Plus class="h-4 w-4" />
						{t('studio.add_job', 'Add Job')}
					</Button>
				</div>
				{#if newBatch.jobs.length === 0}
					<p class="text-xs" style="color: var(--color-muted)">{t('studio.no_batch_jobs', 'No jobs added yet. Click "Add Job" to start.')}</p>
				{/if}
				{#each newBatch.jobs as job, idx (idx)}
					<div class="mb-3 rounded-md border p-3 space-y-2" style="border-color: var(--color-border)">
						<div class="flex items-center justify-between">
							<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.job_n', 'Job')} {idx + 1}</span>
							<button onclick={() => removeBatchJobRow(idx)} class="text-xs" style="color: var(--color-danger)">
								<X class="h-3 w-3" />
							</button>
						</div>
						<select class="w-full rounded-md border px-2 py-1 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={job.persona_id}>
							<option value="">{t('studio.select_persona', 'Select a persona...')}</option>
							{#each personas as p (p)}
								<option value={p.id}>{p.label}</option>
							{/each}
						</select>
						<select class="w-full rounded-md border px-2 py-1 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={job.content_type}>
							<option value="">{t('studio.select_type', 'Select type...')}</option>
							{#each contentTypes as ct (ct)}
								<option value={ct.type}>{ct.label}</option>
							{/each}
						</select>
						<input type="text" class="w-full rounded-md border px-2 py-1 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={job.topic_prompt} placeholder={t('studio.topic_placeholder', 'Describe what the AI should write about...')} />
					</div>
				{/each}
			</div>
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				<Button variant="outline" onclick={() => (createBatchSlideOverOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
				<Button onclick={createBatch} disabled={savingBatch || !newBatch.name}>
					{savingBatch ? t('common.saving', 'Saving...') : t('studio.create_batch', 'Create Batch')}
				</Button>
			</div>
		{/snippet}
	</SlideOver>
{/if}

{#if viewBatchSlideOverOpen && viewingBatch}
	<SlideOver
		bind:open={viewBatchSlideOverOpen}
		title={t('studio.batch_detail', 'Batch Detail')}
	>
		<div class="space-y-3 text-sm">
			<div class="grid gap-2 sm:grid-cols-2">
				<div>
					<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.batch_name', 'Name')}</span>
					<p style="color: var(--color-foreground)">{viewingBatch.batch.name}</p>
				</div>
				<div>
					<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.created', 'Created')}</span>
					<p style="color: var(--color-foreground)">{viewingBatch.batch.created_at ? new Date(viewingBatch.batch.created_at).toLocaleString() : '—'}</p>
				</div>
			</div>
			{#if viewingBatch.batch.description}
				<div>
					<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.description', 'Description')}</span>
					<p style="color: var(--color-foreground)">{viewingBatch.batch.description}</p>
				</div>
			{/if}
			<div class="border-t pt-2" style="border-color: var(--color-border)">
				<span class="text-xs font-medium" style="color: var(--color-muted)">{t('studio.progress', 'Progress')}</span>
				<div class="mt-2 grid grid-cols-2 gap-2 text-xs">
					<div>Total: <span class="font-bold" style="color: var(--color-foreground)">{viewingBatch.stats.total}</span></div>
					<div>Queued: <span style="color: var(--color-muted)">{viewingBatch.stats.queued}</span></div>
					<div>Generating: <span style="color: var(--color-warning)">{viewingBatch.stats.generating}</span></div>
					<div>Draft Ready: <span style="color: var(--color-primary)">{viewingBatch.stats.draft_ready}</span></div>
					<div>In Review: <span style="color: var(--color-warning)">{viewingBatch.stats.in_review}</span></div>
					<div>Published: <span style="color: var(--color-success)">{viewingBatch.stats.published}</span></div>
					<div>Rejected: <span style="color: var(--color-danger)">{viewingBatch.stats.rejected}</span></div>
					<div>Failed: <span style="color: var(--color-danger)">{viewingBatch.stats.failed}</span></div>
				</div>
			</div>
		</div>
		{#snippet footer()}
			<div class="flex justify-end gap-2">
				{#if canManage && viewingBatch && viewingBatch.stats.failed > 0}
					<Button variant="outline" onclick={() => { retryBatch(viewingBatch!.batch.id); viewBatchSlideOverOpen = false; }}>
						<RotateCcw class="h-4 w-4" />
						{t('studio.retry_failed', 'Retry Failed')}
					</Button>
				{/if}
				<Button onclick={() => { viewBatchSlideOverOpen = false; viewingBatch = null; }}>{t('common.close', 'Close')}</Button>
			</div>
		{/snippet}
	</SlideOver>
{/if}
