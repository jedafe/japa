<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { ExternalLink, Loader2, X } from 'lucide-svelte';

	interface LinkPreview {
		url: string;
		title: string;
		description: string;
		image: string;
		site_name: string;
		favicon: string;
	}

	interface Props {
		url: string;
		onclose?: () => void;
	}

	let { url, onclose }: Props = $props();

	let preview = $state<LinkPreview | null>(null);
	let loading = $state(true);
	let error = $state(false);

	$effect(() => {
		fetchPreview(url);
	});

	async function fetchPreview(targetUrl: string) {
		loading = true;
		error = false;
		preview = null;

		const encoded = encodeURIComponent(targetUrl);
		const result = await api.get<LinkPreview>(`/link-preview?url=${encoded}`);

		if (result.error || !result.data) {
			error = true;
			loading = false;
			return;
		}

		preview = result.data;
		loading = false;
	}

	function displayUrl(u: string): string {
		try {
			const parsed = new URL(u);
			return parsed.hostname;
		} catch {
			return u;
		}
	}

	/** Defense-in-depth: only allow http(s) and relative URLs as href.
	 *  The server sanitizer also validates data-url, but the frontend
	 *  must never render javascript:/data: URIs as clickable links. */
	function safeHref(u: string): string {
		const lower = u.trim().toLowerCase();
		if (lower.startsWith('http://') || lower.startsWith('https://') || lower.startsWith('/') || lower.startsWith('#') || lower.startsWith('?')) {
			return u;
		}
		return '#';
	}
</script>

<div class="link-preview-card">
	{#if loading}
		<div class="lp-loading">
			<Loader2 size={16} style="animation: lp-spin 0.8s linear infinite;" />
			<span>{t('create.editor.link_preview.loading')}</span>
		</div>
	{:else if error}
		<div class="lp-error">
			<span>{t('create.editor.link_preview.error')}</span>
			<a href={safeHref(url)} target="_blank" rel="noopener noreferrer" class="lp-open-external">
				{displayUrl(url)}
				<ExternalLink size={12} />
			</a>
		</div>
	{:else if preview}
		<a href={safeHref(url)} target="_blank" rel="noopener noreferrer" class="lp-content">
			{#if preview.image}
				<div class="lp-image">
					<img src={preview.image} alt={preview.title} loading="lazy" />
				</div>
			{/if}
			<div class="lp-text">
				<div class="lp-title">{preview.title || displayUrl(url)}</div>
				{#if preview.description}
					<p class="lp-desc">{preview.description}</p>
				{/if}
				<div class="lp-meta">
					{#if preview.favicon}
						<img src={preview.favicon} alt="" class="lp-favicon" loading="lazy" />
					{/if}
					<span class="lp-site">{preview.site_name || displayUrl(url)}</span>
				</div>
			</div>
			<ExternalLink size={14} style="flex-shrink:0;color:var(--text-muted,#cbd5e1);align-self:center;" />
		</a>
	{/if}

	{#if onclose}
		<button type="button" class="lp-close" onclick={onclose} aria-label={t('create.editor.link_preview.dismiss')}>
			<X size={14} />
		</button>
	{/if}
</div>

<style>
	.link-preview-card {
		position: relative;
		border: 1px solid var(--color-border, #e4e4e7);
		border-radius: 0.625rem;
		overflow: hidden;
		background: var(--color-surface, #fff);
		transition: border-color 0.15s;
	}

	.link-preview-card:hover {
		border-color: var(--color-primary, #3b49df);
	}

	.lp-loading,
	.lp-error {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		font-size: 0.8125rem;
		color: var(--color-muted, #71717a);
	}

	.lp-error {
		flex-direction: column;
		align-items: flex-start;
		gap: 0.25rem;
	}

	@keyframes lp-spin {
		to {
			transform: rotate(360deg);
		}
	}

	.lp-open-external {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		color: var(--color-primary, #3b49df);
		text-decoration: none;
		font-size: 0.8125rem;
	}

	.lp-content {
		display: flex;
		gap: 0.75rem;
		padding: 0.75rem;
		text-decoration: none;
		color: inherit;
	}

	.lp-image {
		flex-shrink: 0;
		width: 4.5rem;
		height: 4.5rem;
		border-radius: 0.5rem;
		overflow: hidden;
		background: var(--color-surface-subtle, #f4f4f5);
	}

	.lp-image img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.lp-text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.lp-title {
		font-weight: 600;
		font-size: 0.875rem;
		color: var(--color-text, #18181b);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.lp-desc {
		font-size: 0.8125rem;
		color: var(--color-text-subtle, #52525b);
		line-height: 1.4;
		display: -webkit-box;
		line-clamp: 2;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		margin: 0;
	}

	.lp-meta {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.75rem;
		color: var(--color-muted, #71717a);
	}

	.lp-favicon {
		width: 1rem;
		height: 1rem;
		border-radius: 0.125rem;
	}

	.lp-close {
		position: absolute;
		top: 0.5rem;
		right: 0.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 1.5rem;
		height: 1.5rem;
		border: none;
		border-radius: 0.25rem;
		background: var(--color-surface-subtle, #f4f4f5);
		color: var(--color-muted, #71717a);
		cursor: pointer;
		opacity: 0;
		transition: opacity 0.15s;
	}

	.link-preview-card:hover .lp-close {
		opacity: 1;
	}

	.lp-close:hover {
		background: var(--color-border, #e4e4e7);
	}

	@media (max-width: 640px) {
		.lp-image {
			width: 3.5rem;
			height: 3.5rem;
		}

		.lp-desc {
			line-clamp: 1;
			-webkit-line-clamp: 1;
		}
	}
</style>
