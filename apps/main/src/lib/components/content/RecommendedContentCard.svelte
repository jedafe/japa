<script lang="ts">
	// "Recommended {type}s" sidebar card for content single pages
	// (ADR-058 Exception #1 route families). Items are the same content_type
	// as the current article, ranked by the backend's embedding-similarity
	// related endpoint (GET /content/{id}/related?content_type=…). Renders
	// nothing when the list is empty.
	import { t } from '$lib/i18n';
	import { contentHref } from '$lib/utils/contentLink';
	import type { ContentArticle } from '$lib/types/content';

	interface Props {
		items: ContentArticle[];
		contentType: string;
	}

	let { items, contentType }: Props = $props();

	// Plural display label for the card title — content_type_plural.* keys
	// exist for the known types; unknown types fall back to the singular
	// content_type.* label, then the humanized raw key.
	const typeLabel = $derived(
		t(`content_type_plural.${contentType}`, t(`content_type.${contentType}`, contentType.replace(/_/g, ' ')))
	);
	const title = $derived(t('content.recommended', 'Recommended {type}', { type: typeLabel }));
</script>

{#if items.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title}</h2>
		<ul class="space-y-2">
			{#each items as item (item.id)}
				<li>
					<a href={contentHref(item)} class="block truncate text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">{item.title}</a>
				</li>
			{/each}
		</ul>
	</section>
{/if}
