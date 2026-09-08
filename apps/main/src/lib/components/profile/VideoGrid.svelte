<script lang="ts">
	// §106 video — grid of video_sources rows for the profile "Videos" tab.
	// A video source is not a content post (it has no slug), so cards link to
	// the canonical source_url: external providers open in a new tab, uploads
	// open the same-origin media serve URL which plays the file directly.
	import { Play, ExternalLink } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { formatDuration } from '$lib/utils/duration';
	import type { VideoSource } from '$lib/types';

	interface Props {
		videos: VideoSource[];
		/** True when the profile is a synthetic (imported) video channel. */
		isChannel?: boolean;
	}

	let { videos, isChannel = false }: Props = $props();

	const isExternalUrl = (url: string | undefined | null): boolean =>
		!!url && /^https?:\/\//i.test(url);

	const sourceLabel = (sourceType: string): string =>
		t(`video.source_type.${sourceType}`, sourceType.replace(/_/g, ' '));
</script>

<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
	{#each videos as video (video.id)}
		{@const external = isExternalUrl(video.source_url)}
		{@const duration = formatDuration(video.duration_secs)}
		<a
			href={video.source_url}
			target={external ? '_blank' : undefined}
			rel={external ? 'noopener noreferrer' : undefined}
			class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<span class="relative block aspect-video w-full bg-[var(--color-surface-subtle)]">
				{#if video.thumbnail_url}
					<img
						src={video.thumbnail_url}
						alt={video.title ?? ''}
						loading="lazy"
						class="h-full w-full object-cover"
					/>
				{/if}
				<span class="pointer-events-none absolute inset-0 grid place-items-center bg-black/25">
					<span class="grid h-11 w-11 place-items-center rounded-full bg-black/60 text-white">
						<Play size={20} class="ml-0.5" />
					</span>
				</span>
				{#if duration}
					<span class="absolute bottom-1.5 right-1.5 rounded bg-black/75 px-1.5 py-0.5 text-[10.5px] font-semibold text-white [font-variant-numeric:tabular-nums]">{duration}</span>
				{/if}
			</span>
			<span class="flex flex-1 flex-col gap-1 p-3">
				<span class="line-clamp-2 text-sm font-medium text-[var(--color-text)]">
					{video.title || t('profile.videos.untitled', 'Untitled video')}
				</span>
				<span class="mt-auto flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
					<span class="inline-block rounded border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
						{sourceLabel(video.source_type)}
					</span>
					{#if external}
						<ExternalLink size={12} />
					{/if}
					{#if isChannel}
						<span>{t('profile.videos.channel_source', 'Channel import')}</span>
					{/if}
					{#if video.created_at}
						<span class="ml-auto">{relativeTime(video.created_at, new Date(), i18n.locale)}</span>
					{/if}
				</span>
			</span>
		</a>
	{/each}
</div>
