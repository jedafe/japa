<script lang="ts">
	// Canonical video single page (§106). sourceOnly=false renders the full
	// article page for a published video post; sourceOnly=true renders a lean
	// player page for a bare video_sources row reached from /sources/{channel}.
	import ContentArticlePage from '$lib/components/content/ContentArticlePage.svelte';
	import VideoEmbed from '$lib/components/content/VideoEmbed.svelte';
	import { formatDuration } from '$lib/utils/duration';
	import { api } from '$lib/api/client';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import { Sparkles, CheckCircle2, AlertCircle, Loader2 } from 'lucide-svelte';
	import { t, siteName} from '$lib/i18n';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const source = $derived(data.videoSource);
	const platformLabel = $derived(
		source ? t(`video.source_type.${source.source_type}`, source.source_type.replace(/_/g, ' ')) : ''
	);

	// §106: POST /videos/{id}/summarize enqueues a video:generate_summary
	// task that drafts title/body as an in_review contents row (HITL).
	let summarizing = $state(false);
	let summarizeResult = $state<'queued' | 'error' | null>(null);

	async function generateSummary() {
		if (!source || summarizing) return;
		summarizing = true;
		summarizeResult = null;
		const r = await api.post(`/videos/${source.id}/summarize`);
		summarizing = false;
		summarizeResult = r.error ? 'error' : 'queued';
	}
</script>

<svelte:head>
	{#if data.sourceOnly && source}
		<title>{source.title ?? t('video.single.title', 'Video')} — {siteName()}</title>
		{#if source.description}
			<meta name="description" content={source.description.slice(0, 160)} />
		{/if}
		{@const videoCanonical = `https://japa.to/video/${source.slug ?? source.id}`}
		<link rel="canonical" href={videoCanonical} />
		<meta property="og:url" content={videoCanonical} />
		<meta property="og:type" content="video.other" />
	{/if}
	{#if data.sourceOnly}
		<meta name="robots" content="noindex,follow" />
	{/if}
</svelte:head>

{#if data.sourceOnly}
	{#if source}
	<div class="mx-auto w-full max-w-3xl px-4 py-6">
		<VideoEmbed {source} />

		<h1 class="mt-6 text-balance text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] md:text-3xl">
			{source.title ?? t('video.single.title', 'Video')}
		</h1>

		<div class="mt-3 flex flex-wrap items-center gap-3 text-sm text-[var(--color-text-muted)]">
			{#if platformLabel}
				<span
					class="rounded-full bg-[var(--color-primary)]/10 px-3 py-1 text-xs font-medium text-[var(--color-primary)]"
					>{platformLabel}</span
				>
			{/if}
			{#if source.duration_secs}
				<span>{formatDuration(source.duration_secs)}</span>
			{/if}
			<a
				href={source.source_url}
				target={source.source_url.startsWith('/api/') ? undefined : '_blank'}
				rel="noopener noreferrer"
				class="font-medium text-[var(--color-primary)] hover:underline"
			>
				{t('video.watch_on', 'Watch on {platform}').replace('{platform}', platformLabel)}
			</a>
			{#if source.source_id}
				<a
					href="/sources/{source.source_id}"
					class="font-medium text-[var(--color-text-muted)] hover:text-[var(--color-text)] hover:underline"
				>
					{t('video.view_channel', 'View channel')}
				</a>
			{/if}
			{#if source.status === 'removed' && source.author_user_id === data.currentUserId}
				<AppealButton entityType="video_source" entityId={source.id} />
			{/if}
			{#if data.canSummarize}
				<button
					type="button"
					onclick={generateSummary}
					disabled={summarizing || summarizeResult === 'queued'}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-elevated)] disabled:opacity-50"
				>
					{#if summarizing}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						<Sparkles size={14} />
					{/if}
					{t('video.summarize_btn', 'Generate AI draft')}
				</button>
			{/if}
		</div>

		{#if summarizeResult === 'queued'}
			<p class="mt-3 flex items-center gap-2 text-sm text-[var(--color-success)]">
				<CheckCircle2 size={16} />
				{t('video.summarize_queued', 'AI draft queued — it will appear in review shortly.')}
			</p>
		{:else if summarizeResult === 'error'}
			<p class="mt-3 flex items-center gap-2 text-sm text-[var(--color-danger)]">
				<AlertCircle size={16} />
				{t('video.summarize_error', 'Could not queue the AI draft.')}
			</p>
		{/if}

		{#if source.description}
			<p class="mt-6 whitespace-pre-line text-[15px] leading-relaxed text-[var(--color-text)]">
				{source.description}
			</p>
		{/if}
	</div>
	{/if}
{:else}
	<ContentArticlePage {data} />
{/if}
