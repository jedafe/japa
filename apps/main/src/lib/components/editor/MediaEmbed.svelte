<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';

	interface Props {
		mediaId?: string;
		url?: string;
		platform?: string;
		alt?: string;
	}

	let { mediaId, url, platform, alt = '' }: Props = $props();

	// TODO: When mediaId is provided, fetch media URL from API
	// For now, resolve directly from the url prop
	let resolvedUrl = $derived(url || '');

	function getEmbedUrl(url: string, platform?: string): string {
		if (!url) return '';

		// YouTube
		if (platform === 'youtube' || url.includes('youtube.com') || url.includes('youtu.be')) {
			const videoId = extractYouTubeId(url);
			if (videoId) {
				return `https://www.youtube.com/embed/${videoId}`;
			}
		}

		// Vimeo
		if (platform === 'vimeo' || url.includes('vimeo.com')) {
			const match = url.match(/vimeo\.com\/(\d+)/);
			if (match) {
				return `https://player.vimeo.com/video/${match[1]}`;
			}
		}

		// Twitter/X
		if (platform === 'twitter' || url.includes('twitter.com') || url.includes('x.com')) {
			// Twitter embeds require their widget.js — return the URL as-is for now
			return url;
		}

		// Default: return URL as-is
		return url;
	}

	function extractYouTubeId(url: string): string | null {
		const match = url.match(/(?:youtube\.com\/(?:[^\/]+\/.+\/|(?:v|e(?:mbed)?)\/|.*[?&]v=)|youtu\.be\/)([^"&?\/\s]{11})/);
		return match ? match[1] : null;
	}

	function isImage(url: string): boolean {
		return /\.(jpg|jpeg|png|gif|webp|svg)(\?.*)?$/i.test(url);
	}

	function isVideo(url: string): boolean {
		return /\.(mp4|webm|ogg)(\?.*)?$/i.test(url);
	}

	let embedUrl = $derived(getEmbedUrl(resolvedUrl, platform));
	let isImageFile = $derived(isImage(resolvedUrl));
	let isVideoFile = $derived(isVideo(resolvedUrl));
</script>

<div class="media-embed">
	{#if isImageFile}
		<img src={resolvedUrl} {alt} class="image" />
	{:else if isVideoFile}
		<!-- svelte-ignore a11y_media_has_caption -->
		<video src={resolvedUrl} controls class="video"></video>
	{:else if embedUrl && (platform === 'youtube' || platform === 'vimeo')}
		<div class="embed-wrapper">
			<iframe
				src={embedUrl}
				title={alt || `Embedded ${platform || 'media'}`}
				frameborder="0"
				allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
				allowfullscreen
				class="embed-iframe"
			></iframe>
		</div>
	{:else if resolvedUrl}
		<a href={resolvedUrl} target="_blank" rel="noopener noreferrer" class="link">
			{resolvedUrl}
		</a>
	{:else}
		<div class="placeholder">
			<span>{t('editor.no_media')}</span>
		</div>
	{/if}
</div>

<style>
	.media-embed {
		margin: 1rem 0;
		border-radius: 0.5rem;
		overflow: hidden;
	}

	.image {
		max-width: 100%;
		height: auto;
		display: block;
		border-radius: 0.5rem;
	}

	.video {
		max-width: 100%;
		border-radius: 0.5rem;
	}

	.embed-wrapper {
		position: relative;
		padding-bottom: 56.25%; /* 16:9 aspect ratio */
		height: 0;
		overflow: hidden;
		border-radius: 0.5rem;
	}

	.embed-iframe {
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
		height: 100%;
		border: none;
	}

	.link {
		display: inline-block;
		padding: 0.5rem 1rem;
		background: var(--color-bg-muted, #f3f4f6);
		border-radius: 0.25rem;
		color: var(--color-primary, #2563eb);
		text-decoration: none;
		word-break: break-all;
	}

	.link:hover {
		text-decoration: underline;
	}

	.placeholder {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 1rem;
		background: var(--color-bg-muted, #f3f4f6);
		border-radius: 0.5rem;
		color: var(--color-text-muted, #6b7280);
	}
</style>
