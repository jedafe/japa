<script lang="ts">
	// Canonical video single page (§106). sourceOnly=false renders the full
	// article page for a published video post; sourceOnly=true renders a lean
	// player page for a bare video_sources row reached from /sources/{channel}.
	import ContentArticlePage from '$lib/components/content/ContentArticlePage.svelte';
	import VideoEmbed from '$lib/components/content/VideoEmbed.svelte';
	import { formatDuration } from '$lib/utils/duration';
	import { t } from '$lib/i18n';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const source = $derived(data.videoSource);
	const platformLabel = $derived(
		source ? t(`video.source_type.${source.source_type}`, source.source_type.replace(/_/g, ' ')) : ''
	);
</script>

<svelte:head>
	{#if data.sourceOnly && source}
		<title>{source.title ?? t('video.single.title', 'Video')} — JapaConnect</title>
		{#if source.description}
			<meta name="description" content={source.description.slice(0, 160)} />
		{/if}
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
		</div>

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
