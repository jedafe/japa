<script lang="ts">
	// Embed card for entity threads at /thread/{slug} — the "what this thread
	// is about" block that links back to the entity page. Three sources:
	//   - tool/wiki anchor (icon tile + entity descriptor)
	//   - §49 linked-discussion parent content (badge + cover + excerpt)
	//   - any other contents row (badge + cover + excerpt)
	// Visual language mirrors ContentEmbedCard (border, badge, title, desc).
	// Ref: docs/superpowers/specs/2026-11-08-thread-rename-unified-discussions-design.md
	import { t } from '#lib/i18n/index.js';
	import { Wrench, BookOpen } from 'lucide-svelte';

	interface Props {
		title: string;
		href: string;
		excerpt?: string | null;
		/** Type label chip (content_type or entity label). */
		badge?: string | null;
		imageUrl?: string | null;
		/** Entity kind for anchor threads — renders an icon tile. */
		entityType?: 'tool' | 'wiki_page' | string;
	}

	let { title, href, excerpt = null, badge = null, imageUrl = null, entityType = '' }: Props = $props();

	const badgeLabel = $derived(
		badge
			? t(`content_type.${badge}`, badge.replace(/_/g, ' '))
			: entityType === 'tool'
				? t('thread.embed.tool', 'Tool')
				: entityType === 'wiki_page'
					? t('thread.embed.wiki_page', 'Wiki page')
					: null
	);
</script>

<a {href} class="te-live">
	{#if imageUrl}
		<div class="te-image">
			<img src={imageUrl} alt={title} loading="lazy" />
		</div>
	{:else if entityType}
		<div class="te-image te-icon">
			{#if entityType === 'wiki_page'}
				<BookOpen size={26} strokeWidth={1.6} />
			{:else}
				<Wrench size={26} strokeWidth={1.6} />
			{/if}
		</div>
	{/if}
	<div class="te-text">
		{#if badgeLabel}
			<span class="te-badge">{badgeLabel}</span>
		{/if}
		<div class="te-title">{title}</div>
		{#if excerpt}
			<p class="te-desc">{excerpt}</p>
		{/if}
	</div>
</a>

<style>
	.te-live {
		display: flex;
		gap: 0.75rem;
		padding: 0.75rem;
		text-decoration: none;
		color: inherit;
		border: 1px solid var(--color-border, #e4e4e7);
		border-radius: 0.625rem;
		background: var(--color-surface, #fff);
		transition: border-color 0.15s;
	}

	.te-live:hover {
		border-color: var(--color-primary, #3b49df);
	}

	.te-image {
		flex-shrink: 0;
		width: 4.5rem;
		height: 4.5rem;
		border-radius: 0.5rem;
		overflow: hidden;
		background: var(--color-surface-subtle, #f4f4f5);
	}

	.te-icon {
		display: grid;
		place-items: center;
		color: var(--color-muted, #71717a);
	}

	.te-image img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.te-text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.te-badge {
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

	.te-title {
		font-weight: 600;
		font-size: 0.875rem;
		color: var(--color-text, #18181b);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.te-desc {
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

	@media (max-width: 640px) {
		.te-image {
			width: 3.5rem;
			height: 3.5rem;
		}

		.te-desc {
			line-clamp: 1;
			-webkit-line-clamp: 1;
		}
	}
</style>
