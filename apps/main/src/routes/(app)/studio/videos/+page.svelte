<script lang="ts">
	import { t } from '$lib/i18n';
	import { Clapperboard, CheckCircle2, AlertCircle, Loader2, Video, Search, ListVideo, RefreshCw } from 'lucide-svelte';
	import { EmptyState, RelativeTime } from '$lib/components/ui';
	import { enhance } from '$app/forms';
	import { invalidate } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { previewBulkImport, retryImportJob, type BulkImportPreview } from '$lib/api/videos';
	import { formatDuration } from '$lib/utils/duration';
	import type { VideoImportJob } from '$lib/types';
	import type { PageData } from './$types';

	const STUDIO_CACHE_TAG = 'app:studio';

	let { data }: { data: PageData } = $props();

	const jobs = $derived<VideoImportJob[]>(data.jobs);

	// DB source_type → friendly label via the existing tab keys
	// (youtube_channel/youtube_playlist/youtube_search, §106).
	const sourceTypeLabel = (s: string): string => {
		switch (s) {
			case 'youtube_channel': return t('studio.tab.channel', 'Channel');
			case 'youtube_playlist': return t('studio.tab.playlist', 'Playlist');
			case 'youtube_search': return t('studio.tab.keyword', 'Keyword');
			default: return s.replace(/_/g, ' ');
		}
	};

	// §7 i18n — tab labels go through t() so they translate and react to
	// locale switches ($derived re-evaluates when i18n.locale changes).
	const TABS = $derived([
		{ id: 'channel', label: t('studio.tab.channel', 'Channel'), icon: Video, requiresUrl: true },
		{ id: 'playlist', label: t('studio.tab.playlist', 'Playlist'), icon: ListVideo, requiresUrl: true },
		{ id: 'keyword', label: t('studio.tab.keyword', 'Keyword'), icon: Search, requiresUrl: false }
	] as const);

	type TabID = (typeof TABS)[number]['id'];

	let tab = $state<TabID>('channel');
	let sourceUrl = $state('');
	let query = $state('');
	let targetUsername = $state('');
	let autoPublish = $state(false);
	let result = $state<{ ok?: boolean; error?: string; message?: string; job?: VideoImportJob } | null>(null);
	let submitting = $state(false);

	// §106 video — bulk import preview before enqueue.
	let preview = $state<BulkImportPreview | null>(null);
	let previewing = $state(false);
	let retryingId = $state<string | null>(null);

	const currentTab = $derived(TABS.find((t) => t.id === tab)!);

	const statusColor = (status: string): string => {
		switch (status) {
			case 'completed': return 'text-[var(--color-success)]';
			case 'failed': return 'text-[var(--color-danger)]';
			case 'partial': return 'text-[var(--color-warning)]';
			case 'running':
			case 'processing': return 'text-[var(--color-info)]';
			default: return 'text-[var(--color-muted)]';
		}
	};

	// Live status overrides for in-flight jobs — the server load captures
	// state at request time, so pending/running/processing jobs are polled
	// until they reach a terminal status (§106 import history).
	const ACTIVE_STATUSES = ['pending', 'running', 'processing'];
	const TERMINAL_STATUSES = ['completed', 'failed', 'partial'];
	let liveStatus = $state<Record<string, VideoImportJob>>({});
	const displayJobs = $derived(jobs.map((j) => liveStatus[j.id] ?? j));
	// Filter the merged view, not the raw server list — a job that reached a
	// terminal status via polling (liveStatus) must leave the poll set
	// immediately, not only after the invalidate round-trip refreshes `jobs`.
	const activeJobs = $derived(displayJobs.filter((j) => ACTIVE_STATUSES.includes(j.status)));

	async function doPreview() {
		if (previewing) return;
		previewing = true;
		preview = null;
		result = null;

		const body: Parameters<typeof previewBulkImport>[0] = {
			source_type: tab,
			auto_publish: autoPublish
		};
		if (currentTab.requiresUrl) body.source_url = sourceUrl;
		else body.query = query;
		if (targetUsername) body.target_username = targetUsername;

		try {
			const r = await previewBulkImport(body);
			if (r.error) {
				result = { error: r.error.code, message: r.error.message };
			} else if (r.data) {
				preview = r.data;
			}
		} catch (e) {
			result = { error: 'preview_failed', message: e instanceof Error ? e.message : t('studio.preview_error', 'Preview failed') };
		} finally {
			previewing = false;
		}
	}

	async function retryJob(job: VideoImportJob) {
		if (retryingId) return;
		retryingId = job.id;
		try {
			const r = await retryImportJob(job.id);
			if (r.error) {
				result = { error: r.error.code, message: r.error.message };
			} else if (r.data) {
				result = { ok: true, job: r.data };
				// Seed liveStatus with the retried job immediately — otherwise
				// the row keeps displaying the stale terminal status until the
				// next invalidate round-trip or poll tick.
				liveStatus = { ...liveStatus, [job.id]: r.data };
				await invalidate(STUDIO_CACHE_TAG);
			}
		} catch (e) {
			result = { error: 'retry_failed', message: e instanceof Error ? e.message : t('studio.retry_error', 'Retry failed') };
		} finally {
			retryingId = null;
		}
	}

	function onTabChange(next: TabID) {
		tab = next;
		sourceUrl = '';
		query = '';
		// Stale state from the previous source type must not linger under
		// the new tab's inputs.
		preview = null;
		result = null;
	}

	// Poll every in-flight job until terminal status — covers both jobs
	// submitted in this session and ones already running on page load.
	// When any job finishes, invalidate so the history re-fetches.
	$effect(() => {
		const actives = activeJobs;
		if (actives.length === 0) return;
		const iv = setInterval(async () => {
			let anyTerminal = false;
			for (const job of actives) {
				const r = await api.get<VideoImportJob>(`/me/video-imports/${job.id}`);
				if (!r.data) continue;
				liveStatus = { ...liveStatus, [job.id]: r.data };
				if (TERMINAL_STATUSES.includes(r.data.status)) anyTerminal = true;
			}
			if (anyTerminal) await invalidate(STUDIO_CACHE_TAG);
		}, 3000);
		return () => clearInterval(iv);
	});
</script>

<svelte:head>
	<title>{t('studio.tabs.videos', 'Videos')} — {t('studio.title', 'Studio — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="w-full">
	<p class="mb-4 text-sm text-[var(--color-muted)]">
		{t('studio.description', 'Bulk import YouTube channels, playlists, or keyword search results as video content.')}
	</p>

	<div>
		<!-- Tabs -->
		<div class="mb-4 flex gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1">
			{#each TABS as tdef (tdef.id)}
				<button
					type="button"
					onclick={() => onTabChange(tdef.id)}
					class="flex flex-1 items-center justify-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm font-medium transition-colors
						{tab === tdef.id ? 'bg-[var(--color-primary)] text-white' : 'text-[var(--color-muted)] hover:bg-[var(--color-surface-elevated)]'}"
				>
					<tdef.icon size={16} /> {tdef.label}
				</button>
			{/each}
		</div>

		<!-- Import form — §106: start/preview/retry all require
		     video.import.bulk; without it the card shows a notice instead
		     of a form that could only fail with 403. -->
		<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Video size={16} /> {t('studio.start', 'Start a bulk import')}
			</h2>

			{#if !data.canBulkImport}
				<div class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
					<AlertCircle size={16} />
					{t('studio.no_bulk_permission', 'Bulk video import is not available for your account.')}
				</div>
			{:else}
			{#if result?.ok}
				<div class="mb-3 flex items-center gap-2 text-sm text-[var(--color-success)]">
					<CheckCircle2 size={16} /> {t('studio.started', 'Import started. We will process it in the background.')}
				</div>
			{:else if result?.error}
				<div class="mb-3 flex items-center gap-2 text-sm text-[var(--color-danger)]">
					<AlertCircle size={16} /> {result.message || result.error}
				</div>
			{/if}

			<form
				method="POST"
				action="?/start"
				use:enhance={() => {
					submitting = true;
					return async ({ result: r }) => {
						submitting = false;
						if (r.type === 'success' || r.type === 'failure') {
							result = r.data as { ok?: boolean; error?: string; message?: string; job?: VideoImportJob };
							if (result?.ok && result?.job) {
								// The import was queued — the preview panel served its
								// purpose and would be misleading under a "started" banner.
								preview = null;
								await invalidate(STUDIO_CACHE_TAG);
							}
						} else if (r.type === 'error') {
							// Thrown action errors (network/5xx) land here — without
							// this branch the form would fail silently.
							result = { error: 'import_failed', message: t('studio.start_error', 'Import failed to start — please try again') };
						}
					};
				}}
				class="space-y-3"
			>
				<input type="hidden" name="source_type" value={tab} />

				<div>
					{#if currentTab.requiresUrl}
						<label for="source_url" class="block text-xs font-medium text-[var(--color-muted)]">
							{t('studio.source_url', 'YouTube URL')}
						</label>
						<input
							id="source_url"
							name="source_url"
							type="url"
							bind:value={sourceUrl}
							placeholder={t('studio.source_url_placeholder', 'https://www.youtube.com/...')}
							class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
						/>
					{:else}
						<label for="query" class="block text-xs font-medium text-[var(--color-muted)]">
							{t('studio.query', 'Search keyword')}
						</label>
						<input
							id="query"
							name="query"
							type="text"
							bind:value={query}
							placeholder={t('studio.query_placeholder', 'Enter a keyword or phrase')}
							class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
						/>
					{/if}
				</div>

				<!-- §106: attribution to another user additionally requires
				     admin.video_import_jobs.manage — hidden otherwise. -->
				{#if data.canAttributeImports}
					<div>
						<label for="target_username" class="block text-xs font-medium text-[var(--color-muted)]">
							{t('studio.target_username', 'Target username (optional)')}
						</label>
						<input
							id="target_username"
							name="target_username"
							type="text"
							bind:value={targetUsername}
							placeholder={t('studio.target_username_placeholder', 'Attribute imports to another user')}
							class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
						/>
					</div>
				{/if}

				<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						name="auto_publish"
						bind:checked={autoPublish}
						class="rounded border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-primary)]"
					/>
					{t('studio.auto_publish', 'Publish videos as public content automatically')}
				</label>

				<div class="flex flex-wrap items-center gap-2">
					<button
						type="submit"
						disabled={submitting || (currentTab.requiresUrl ? !sourceUrl : !query)}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
					>
						{submitting ? t('common.submitting', 'Submitting…') : t('studio.start_btn', 'Start import')}
					</button>

					<button
						type="button"
						disabled={previewing || (currentTab.requiresUrl ? !sourceUrl : !query)}
						onclick={doPreview}
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-elevated)] disabled:opacity-50"
					>
						{previewing ? t('common.loading', 'Loading…') : t('studio.preview_btn', 'Preview')}
					</button>
				</div>
			</form>

			<!-- §106 video — bulk import preview before enqueue. -->
			{#if preview}
				<div class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h3 class="mb-2 text-sm font-medium text-[var(--color-text)]">
						{t('studio.preview_title', 'Import preview')}
					</h3>
					<p class="text-sm text-[var(--color-muted)]">
						{t('studio.preview_count', 'Found {count} videos to import').replace('{count}', String(preview.count))}
					</p>

					{#if preview.sample}
						<div class="mt-3 flex gap-3">
							{#if preview.sample.thumbnail_url}
								<img
									src={preview.sample.thumbnail_url}
									alt={preview.sample.title ?? ''}
									class="h-20 w-32 shrink-0 rounded-[var(--radius)] object-cover"
									loading="lazy"
								/>
							{/if}
							<div class="min-w-0 flex-1">
								<p class="line-clamp-2 text-sm font-medium text-[var(--color-text)]">
									{preview.sample.title || t('studio.preview_untitled', 'Untitled video')}
								</p>
								<p class="mt-0.5 text-xs text-[var(--color-muted)]">
									<span>{currentTab.label}</span>
									{#if preview.sample.duration_secs}
										· {formatDuration(preview.sample.duration_secs)}
									{/if}
								</p>
							</div>
						</div>
					{/if}

					<button
						type="button"
						onclick={() => { preview = null; }}
						class="mt-3 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
					>
						{t('studio.clear_preview', 'Clear preview')}
					</button>
				</div>
			{/if}
			{/if}
		</div>

		<!-- Import history -->
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
			{t('studio.history', 'Import history')}
		</h2>

		{#if displayJobs.length === 0}
			<EmptyState icon={Clapperboard} title={t('studio.no_imports', 'No video imports yet')} />
		{:else}
			<div class="space-y-2">
				{#each displayJobs as job (job.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<span class="block text-sm font-medium text-[var(--color-text)]">
									{sourceTypeLabel(job.source_type)}
								</span>
								{#if job.source_url}
									<span class="mt-0.5 block truncate text-xs text-[var(--color-muted)]">{job.source_url}</span>
								{:else if job.query}
									<span class="mt-0.5 block truncate text-xs text-[var(--color-muted)]">{job.query}</span>
								{/if}
								<span class="mt-1 block text-xs text-[var(--color-muted)]">
									<RelativeTime iso={job.created_at} />
									{#if job.completed_at}
										· {job.status === 'completed' ? t('studio.completed', 'Completed') : t('studio.finished', 'Finished')} <RelativeTime iso={job.completed_at} />
									{/if}
								</span>
								{#if job.total_items > 0}
									<span class="mt-1 block text-xs text-[var(--color-muted)]">
										{job.imported_count}/{job.total_items} {t('studio.imported', 'imported')}
										{#if job.failed_count > 0}
											· {job.failed_count} {t('studio.failed', 'failed')}
										{/if}
									</span>
								{/if}

								{#if job.error_log && Array.isArray(job.error_log) && job.error_log.length > 0}
									{@const firstError = job.error_log[0]}
									{@const errorText = typeof firstError === 'string'
										? firstError
										: (firstError && typeof firstError === 'object' && 'message' in firstError
											? (firstError as { message?: string }).message
											: JSON.stringify(firstError))}
									<p class="mt-1 text-xs text-[var(--color-danger)]">
										{errorText ?? t('studio.unknown_error', 'Error')}
									</p>
								{/if}
							</div>
							<div class="flex shrink-0 flex-col items-end gap-1.5">
								<span class="flex items-center gap-1 text-xs font-medium capitalize {statusColor(job.status)}">
									{#if job.status === 'completed'}
										<CheckCircle2 size={12} />
									{:else if job.status === 'failed'}
										<AlertCircle size={12} />
									{:else}
										<Loader2 size={12} class={['running', 'processing'].includes(job.status) ? 'animate-spin' : ''} />
									{/if}
									{job.status}
								</span>

								{#if data.canBulkImport && !['pending', 'running', 'processing'].includes(job.status)}
									<button
										type="button"
										disabled={retryingId === job.id}
										onclick={() => retryJob(job)}
										class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline disabled:opacity-50"
									>
										<RefreshCw size={12} class={retryingId === job.id ? 'animate-spin' : ''} />
										{t('studio.retry', 'Retry')}
									</button>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
