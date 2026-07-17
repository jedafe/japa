<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

	interface Props {
		url: string;
		title?: string;
		description?: string;
		image?: string;
		siteName?: string;
	}

	let { url, title = $bindable(), description = $bindable(), image = $bindable(), siteName = $bindable() }: Props = $props();

	let hostname = $derived(getHostname(url));
	let loading = $state(false);
	let fetchError = $state(false);

	function getHostname(url: string): string {
		try {
			return new URL(url).hostname;
		} catch {
			return url;
		}
	}

	function getCacheKey(url: string): string {
		return `link-preview:${url}`;
	}

	onMount(async () => {
		// Skip if props already provided
		if (title || image) return;

		// Check sessionStorage cache
		const cacheKey = getCacheKey(url);
		try {
			const cached = sessionStorage.getItem(cacheKey);
			if (cached) {
				const data = JSON.parse(cached);
				title = data.title;
				description = data.description;
				image = data.image_url;
				siteName = data.site_name;
				return;
			}
		} catch {
			// ignore cache errors
		}

		loading = true;
		try {
			const controller = new AbortController();
			const timeout = setTimeout(() => controller.abort(), 3000);

			const data = await api.getLinkPreview(url);
			clearTimeout(timeout);

			title = data.title;
			description = data.description;
			image = data.image_url;
			siteName = data.site_name;

			// Cache the result
			try {
				sessionStorage.setItem(cacheKey, JSON.stringify(data));
			} catch {
				// ignore cache errors
			}
		} catch {
			fetchError = true;
		} finally {
			loading = false;
		}
	});
</script>

{#if loading}
	<div class="link-preview loading">
		<div class="skeleton-image"></div>
		<div class="content">
			<div class="skeleton-line short"></div>
			<div class="skeleton-line"></div>
			<div class="skeleton-line medium"></div>
		</div>
	</div>
{:else if fetchError && !title && !image}
	<a href={url} target="_blank" rel="noopener noreferrer" class="link-preview fallback">
		<div class="content">
			<div class="hostname">{hostname}</div>
		</div>
	</a>
{:else}
	<a
		href={url}
		target="_blank"
		rel="noopener noreferrer"
		class="link-preview"
	>
		{#if image}
			<div class="image-wrapper">
				<img src={image} alt={title || ''} class="preview-image" />
			</div>
		{/if}

		<div class="content">
			{#if siteName}
				<div class="site-name">{siteName}</div>
			{/if}

			{#if title}
				<div class="title">{title}</div>
			{/if}

			{#if description}
				<div class="description">{description}</div>
			{/if}

			<div class="hostname">{hostname}</div>
		</div>
	</a>
{/if}

<style>
	.link-preview {
		display: flex;
		flex-direction: column;
		border: 1px solid var(--color-border, #e5e7eb);
		border-radius: 0.5rem;
		overflow: hidden;
		text-decoration: none;
		color: inherit;
		transition: box-shadow 0.2s;
	}

	.link-preview:hover {
		box-shadow: 0 4px 6px -1px rgb(0 0 0 / 0.1);
	}

	.link-preview.loading {
		pointer-events: none;
	}

	.image-wrapper {
		width: 100%;
		aspect-ratio: 16 / 9;
		overflow: hidden;
		background: var(--color-bg-muted, #f3f4f6);
	}

	.preview-image {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.content {
		padding: 0.75rem 1rem;
	}

	.site-name {
		font-size: 0.75rem;
		color: var(--color-text-muted, #6b7280);
		margin-bottom: 0.25rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	.title {
		font-weight: 600;
		font-size: 1rem;
		color: var(--color-text, #111827);
		margin-bottom: 0.25rem;
		line-height: 1.4;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.description {
		font-size: 0.875rem;
		color: var(--color-text-muted, #6b7280);
		line-height: 1.5;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		margin-bottom: 0.5rem;
	}

	.hostname {
		font-size: 0.75rem;
		color: var(--color-text-muted, #9ca3af);
	}

	.skeleton-image {
		width: 100%;
		aspect-ratio: 16 / 9;
		background: linear-gradient(90deg, var(--color-bg-muted, #f3f4f6) 25%, var(--color-bg-muted-dark, #e5e7eb) 50%, var(--color-bg-muted, #f3f4f6) 75%);
		background-size: 200% 100%;
		animation: shimmer 1.5s infinite;
	}

	.skeleton-line {
		height: 0.75rem;
		border-radius: 0.25rem;
		background: linear-gradient(90deg, var(--color-bg-muted, #f3f4f6) 25%, var(--color-bg-muted-dark, #e5e7eb) 50%, var(--color-bg-muted, #f3f4f6) 75%);
		background-size: 200% 100%;
		animation: shimmer 1.5s infinite;
		margin-bottom: 0.5rem;
	}

	.skeleton-line.short {
		width: 30%;
	}

	.skeleton-line.medium {
		width: 60%;
	}

	@keyframes shimmer {
		0% { background-position: 200% 0; }
		100% { background-position: -200% 0; }
	}
</style>
