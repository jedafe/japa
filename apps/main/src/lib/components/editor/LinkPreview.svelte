<script lang="ts">
	interface Props {
		url: string;
		title?: string;
		description?: string;
		image?: string;
		siteName?: string;
	}

	let { url, title, description, image, siteName }: Props = $props();

	let hostname = $derived(getHostname(url));

	function getHostname(url: string): string {
		try {
			return new URL(url).hostname;
		} catch {
			return url;
		}
	}
</script>

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
</style>
