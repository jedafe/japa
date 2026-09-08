<script lang="ts">
	import { t } from '$lib/i18n';
	import { Clapperboard, CheckCircle2, AlertCircle, Loader2, Video, Search, ListVideo } from 'lucide-svelte';
	import { EmptyState, RelativeTime } from '$lib/components/ui';
	import { enhance } from '$app/forms';
	import { invalidate } from '$app/navigation';
	import { api } from '$lib/api/client';
	import type { VideoImportJob } from '$lib/types';
	import type { PageData } from './$types';

	const STUDIO_CACHE_TAG = 'app:studio';

	let { data }: { data: PageData } = $props();

	const jobs = $derived<VideoImportJob[]>(data.jobs);

	const TABS = [
		{ id: 'channel', label: 'Channel', icon: Video, requiresUrl: true },
		{ id: 'playlist', label: 'Playlist', icon: ListVideo, requiresUrl: true },
		{ id: 'keyword', label: 'Keyword', icon: Search, requiresUrl: false }
	] as const;

	type TabID = (typeof TABS)[number]['id'];

	let tab = $state<TabID>('channel');
	let sourceUrl = $state('');
	let query = $state('');
	let targetUsername = $state('');
	let autoPublish = $state(false);
	let result = $state<{ ok?: boolean; error?: string; message?: string; job?: VideoImportJob } | null>(null);
	let submitting = $state(false);

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

	let pollInterval: ReturnType<typeof setInterval> | null = null;

	function startPolling(job: VideoImportJob) {
		if (pollInterval) clearInterval(pollInterval);

		pollInterval = setInterval(async () => {
			const r = await api.get<VideoImportJob>(`/me/video-imports/${job.id}`);
			if (r.error || !r.data) {
				if (pollInterval) clearInterval(pollInterval);
				return;
			}

			const updated = r.data;
			result = { ok: true, job: updated };

			if (['completed', 'failed', 'partial'].includes(updated.status)) {
				if (pollInterval) clearInterval(pollInterval);
				await invalidate(STUDIO_CACHE_TAG);
			}
		}, 3000);
	}

	function onTabChange(next: TabID) {
		tab = next;
		sourceUrl = '';
		query = '';
	}

	$effect(() => {
		if (result?.ok && result?.job && ['running', 'processing', 'pending'].includes(result.job.status)) {
			startPolling(result.job);
		}
		return () => {
			if (pollInterval) clearInterval(pollInterval);
		};
	});
</script>

<svelte:head>
	<title>{t('studio.title', 'Studio — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Clapperboard size={20} /> {t('studio.heading', 'Studio')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('studio.description', 'Bulk import YouTube channels, playlists, or keyword search results as video content.')}
		</p>
	</div>

	<div class="p-4">
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

		<!-- Import form -->
		<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Video size={16} /> {t('studio.start', 'Start a bulk import')}
			</h2>

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
						if (r.type === 'success') {
							result = r.data as { ok?: boolean; error?: string; message?: string; job?: VideoImportJob };
							if (result?.ok && result?.job) {
								await invalidate(STUDIO_CACHE_TAG);
								startPolling(result.job);
							}
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

				<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						name="auto_publish"
						bind:checked={autoPublish}
						class="rounded border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-primary)]"
					/>
					{t('studio.auto_publish', 'Publish videos as public content automatically')}
				</label>

				<button
					type="submit"
					disabled={submitting || (currentTab.requiresUrl ? !sourceUrl : !query)}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{submitting ? t('common.submitting', 'Submitting…') : t('studio.start_btn', 'Start import')}
				</button>
			</form>
		</div>

		<!-- Import history -->
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
			{t('studio.history', 'Import history')}
		</h2>

		{#if jobs.length === 0}
			<EmptyState icon={Clapperboard} title={t('studio.no_imports', 'No video imports yet')} />
		{:else}
			<div class="space-y-2">
				{#each jobs as job (job.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<span class="block text-sm font-medium capitalize text-[var(--color-text)]">
									{job.source_type}
								</span>
								{#if job.source_url}
									<span class="mt-0.5 block truncate text-xs text-[var(--color-muted)]">{job.source_url}</span>
								{:else if job.query}
									<span class="mt-0.5 block truncate text-xs text-[var(--color-muted)]">{job.query}</span>
								{/if}
								<span class="mt-1 block text-xs text-[var(--color-muted)]">
									<RelativeTime iso={job.created_at} />
									{#if job.completed_at}
										· {t('studio.completed', 'Completed')} <RelativeTime iso={job.completed_at} />
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
							</div>
							<span class="flex shrink-0 items-center gap-1 text-xs font-medium capitalize {statusColor(job.status)}">
								{#if job.status === 'completed'}
									<CheckCircle2 size={12} />
								{:else if job.status === 'failed'}
									<AlertCircle size={12} />
								{:else}
									<Loader2 size={12} class={['running', 'processing'].includes(job.status) ? 'animate-spin' : ''} />
								{/if}
								{job.status}
							</span>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
