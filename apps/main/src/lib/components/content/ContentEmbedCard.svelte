<script lang="ts">
	// Internal-content embed card — mounted into
	// `div[data-type="content-embed"][data-content-slug]` placeholders by
	// hydrateContentEmbeds on view pages. Fetches the live article via
	// GET /content/{slug}; on failure it re-renders the serialized static
	// snapshot (`initial`) or an unavailable notice.
	// Ref: docs/superpowers/specs/2026-09-28-list-content-type-design.md
	//      .jeos/sections/14-content-type-system.md (List amendment)
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { contentHref } from '$lib/utils/contentLink';
	import { Loader2 } from 'lucide-svelte';
	import type { ListEmbedSnapshot } from '$lib/types';

	// GET /content/{slug} detail DTO — the subset this card renders.
	interface EmbedContentDTO {
		slug: string;
		title: string;
		content_type: string;
		featured_image_url?: string | null;
		header_image_url?: string | null;
		meta_description?: string | null;
		excerpt?: string | null;
		author_name?: string | null;
		author_username?: string | null;
		author_avatar_url?: string | null;
	}

	interface Props {
		slug: string;
		/** Static snapshot serialized into body_html — shown while loading
		 *  and as the fallback when the content can no longer be fetched. */
		initial?: ListEmbedSnapshot;
	}

	let { slug, initial }: Props = $props();

	let content = $state<EmbedContentDTO | null>(null);
	let failed = $state(false);

	$effect(() => {
		load(slug);
	});

	async function load(targetSlug: string) {
		const res = await api.get<EmbedContentDTO>(`/content/${targetSlug}`);
		if (res.error || !res.data) {
			failed = true;
			return;
		}
		content = res.data;
		failed = false;
	}

	const typeLabel = (ct: string) => t(`content_type.${ct}`, ct.replace(/_/g, ' '));

	const snapshotHref = $derived(contentHref({ slug, content_type: initial?.content_type }));
	const excerpt = $derived(content?.meta_description ?? content?.excerpt ?? null);
	const image = $derived(content?.featured_image_url ?? content?.header_image_url ?? null);
	const authorName = $derived(content?.author_name ?? content?.author_username ?? null);
</script>

<div class="content-embed-card">
	{#if content}
		<a href={contentHref(content)} class="ce-live">
			{#if image}
				<div class="ce-image">
					<img src={image} alt={content.title} loading="lazy" />
				</div>
			{/if}
			<div class="ce-text">
				<span class="ce-badge">{typeLabel(content.content_type)}</span>
				<div class="ce-title">{content.title}</div>
				{#if excerpt}
					<p class="ce-desc">{excerpt}</p>
				{/if}
				{#if authorName}
					<div class="ce-meta">
						{#if content.author_avatar_url}
							<img src={content.author_avatar_url} alt="" class="ce-avatar" loading="lazy" />
						{/if}
						<span class="ce-author">{authorName}</span>
					</div>
				{/if}
			</div>
		</a>
	{:else if initial && (initial.title || initial.image_url)}
		<!-- Static snapshot serialized into body_html — shown while the live
		     fetch is in flight and kept when the content is gone. -->
		<a href={snapshotHref} class="ce-live">
			{#if initial.image_url}
				<div class="ce-image">
					<img src={initial.image_url} alt={initial.title ?? ''} loading="lazy" />
				</div>
			{/if}
			<div class="ce-text">
				{#if initial.content_type}
					<span class="ce-badge">{typeLabel(initial.content_type)}</span>
				{/if}
				<div class="ce-title">{initial.title ?? slug}</div>
				{#if initial.excerpt}
					<p class="ce-desc">{initial.excerpt}</p>
				{/if}
			</div>
		</a>
	{:else if failed}
		<div class="ce-unavailable">
			<span>{t('content.embed.unavailable', 'This content is no longer available')}</span>
		</div>
	{:else}
		<div class="ce-loading">
			<Loader2 size={16} style="animation: ce-spin 0.8s linear infinite;" />
			<span>{t('common.loading', 'Loading')}</span>
		</div>
	{/if}
</div>

<style>
	.content-embed-card {
		position: relative;
		margin: 0.75rem 0;
		border: 1px solid var(--color-border, #e4e4e7);
		border-radius: 0.625rem;
		overflow: hidden;
		background: var(--color-surface, #fff);
		transition: border-color 0.15s;
	}

	.content-embed-card:hover {
		border-color: var(--color-primary, #3b49df);
	}

	.ce-loading,
	.ce-unavailable {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		font-size: 0.8125rem;
		color: var(--color-muted, #71717a);
	}

	@keyframes ce-spin {
		to {
			transform: rotate(360deg);
		}
	}

	.ce-live {
		display: flex;
		gap: 0.75rem;
		padding: 0.75rem;
		text-decoration: none;
		color: inherit;
	}

	.ce-image {
		flex-shrink: 0;
		width: 4.5rem;
		height: 4.5rem;
		border-radius: 0.5rem;
		overflow: hidden;
		background: var(--color-surface-subtle, #f4f4f5);
	}

	.ce-image img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.ce-text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.ce-badge {
		align-self: flex-start;
		border-radius: 0.25rem;
		background: var(--color-surface-subtle, #f4f4f5);
		border: 1px solid var(--color-border, #e4e4e7);
		padding: 0.0625rem 0.375rem;
		font-size: 0.625rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-text-subtle, #52525b);
	}

	.ce-title {
		font-weight: 600;
		font-size: 0.875rem;
		color: var(--color-text, #18181b);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.ce-desc {
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

	.ce-meta {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.75rem;
		color: var(--color-muted, #71717a);
	}

	.ce-avatar {
		width: 1rem;
		height: 1rem;
		border-radius: 9999px;
		object-fit: cover;
	}

	@media (max-width: 640px) {
		.ce-image {
			width: 3.5rem;
			height: 3.5rem;
		}

		.ce-desc {
			line-clamp: 1;
			-webkit-line-clamp: 1;
		}
	}
</style>
