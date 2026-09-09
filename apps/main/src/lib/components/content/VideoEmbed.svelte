<script lang="ts">
	// §106 video — renders the player for a video_sources row on the content
	// single page. The stored body_html only carries an empty .video-embed
	// placeholder (the server sanitizer strips the data-* attributes), so the
	// page resolves the source via GET /videos/{id} and renders the player
	// here instead of relying on sanitized markup.
	import { t } from '$lib/i18n';
	import { formatDuration } from '$lib/utils/duration';
	import type { VideoSource } from '$lib/types';

	interface Props {
		source: VideoSource;
	}

	let { source }: Props = $props();

	/** Best-effort external ID recovery when external_id is missing —
	 *  handles the canonical YouTube watch/youtu.be/embed/shorts URLs the
	 *  resolver stores in source_url. */
	function extractExternalId(src: VideoSource): string {
		if (src.external_id) return src.external_id;
		try {
			const u = new URL(src.source_url, 'http://localhost');
			const host = u.hostname.replace(/^www\./, '');
			if (host === 'youtu.be') return u.pathname.replace(/^\//, '');
			if (host === 'youtube.com') {
				const v = u.searchParams.get('v');
				if (v) return v;
				const seg = u.pathname.split('/').filter(Boolean);
				if (['embed', 'shorts'].includes(seg[0]) && seg[1]) return seg[1];
			}
			if (host === 'vimeo.com') {
				const seg = u.pathname.split('/').filter(Boolean);
				if (seg[0]) return seg[0];
			}
			if (host === 'tiktok.com') {
				const seg = u.pathname.split('/').filter(Boolean);
				for (let i = seg.length - 1; i >= 0; i--) {
					if (/^\d+$/.test(seg[i])) return seg[i];
				}
			}
			if (host === 'instagram.com') {
				const seg = u.pathname.split('/').filter(Boolean);
				for (let i = 0; i < seg.length - 1; i++) {
					if (['p', 'reel', 'reels'].includes(seg[i]) && seg[i + 1]) return seg[i + 1];
				}
			}
		} catch {
			// fall through — return empty
		}
		return '';
	}

	const isUpload = $derived(source.source_type === 'upload');
	const unavailable = $derived(['removed', 'takedown_requested'].includes(source.status ?? ''));
	const externalId = $derived(extractExternalId(source));
	const iframeSrc = $derived.by(() => {
		if (!externalId) return null;
		switch (source.source_type) {
			case 'youtube':
				return `https://www.youtube-nocookie.com/embed/${encodeURIComponent(externalId)}`;
			case 'vimeo':
				return `https://player.vimeo.com/video/${encodeURIComponent(externalId)}`;
			case 'tiktok':
				return `https://www.tiktok.com/player/v1/${encodeURIComponent(externalId)}`;
			case 'instagram_reel':
				return `https://www.instagram.com/p/${encodeURIComponent(externalId)}/embed`;
			default:
				return null;
		}
	});
	const duration = $derived(formatDuration(source.duration_secs));
</script>

<div class="video-embed-player" class:video-embed-unavailable={unavailable}>
	{#if unavailable}
		<!-- §106 video — removed/takedown notice replaces the player. -->
		<div class="video-embed-unavailable-panel">
			<p class="video-embed-unavailable-title">{t('content.video.unavailable', 'Video unavailable')}</p>
			<p class="video-embed-unavailable-body">
				{t('content.video.unavailable_body', 'This video has been removed or is unavailable, possibly due to a takedown request.')}
			</p>
		</div>
	{:else if isUpload}
		<!-- Uploaded videos are served same-origin via /api/v1/media/serve.
		     No captions track exists for user uploads. -->
		<!-- svelte-ignore a11y_media_has_caption -->
		<video controls preload="metadata" src={source.source_url} poster={source.thumbnail_url ?? undefined}></video>
	{:else if iframeSrc}
		<iframe
			src={iframeSrc}
			title={source.title || t('content.video.player_title', 'Video player')}
			allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
			allowfullscreen
			loading="lazy"
		></iframe>
	{:else}
		<!-- Unknown provider — link out to the canonical source. -->
		<a href={source.source_url} target="_blank" rel="noopener noreferrer" class="video-embed-fallback">
			{source.title || source.source_url}
		</a>
	{/if}
	{#if duration && !unavailable}
		<span class="video-embed-duration">{duration}</span>
	{/if}
</div>

<style>
	.video-embed-player {
		position: relative;
		width: 100%;
		aspect-ratio: 16 / 9;
		border-radius: var(--radius-card, 12px);
		overflow: hidden;
		background: #000;
	}
	.video-embed-player video,
	.video-embed-player iframe {
		width: 100%;
		height: 100%;
		border: 0;
		display: block;
	}
	.video-embed-fallback {
		display: grid;
		place-items: center;
		width: 100%;
		height: 100%;
		padding: 1rem;
		color: #fff;
		text-align: center;
	}
	.video-embed-duration {
		position: absolute;
		bottom: 0.5rem;
		right: 0.5rem;
		border-radius: var(--radius, 6px);
		background: rgba(0, 0, 0, 0.75);
		padding: 2px 6px;
		font-size: 0.6875rem;
		font-weight: 600;
		color: #fff;
		font-variant-numeric: tabular-nums;
		pointer-events: none;
	}
	.video-embed-unavailable {
		background: var(--color-surface-elevated, #1a1a1a);
	}
	.video-embed-unavailable-panel {
		display: grid;
		place-items: center;
		place-content: center;
		width: 100%;
		height: 100%;
		padding: 1rem;
		text-align: center;
		gap: 0.5rem;
	}
	.video-embed-unavailable-title {
		font-weight: 600;
		font-size: 0.875rem;
		color: var(--color-text, #fff);
	}
	.video-embed-unavailable-body {
		font-size: 0.75rem;
		color: var(--color-muted, #aaa);
		max-width: 24rem;
	}
</style>
