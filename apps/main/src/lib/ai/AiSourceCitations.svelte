<script lang="ts">
	import { BookOpen } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { AiCitationSource } from './types';

	/**
	 * RAG source citation chips rendered beneath an assistant reply.
	 * §69 Citation Source Contract: the backend emits { title, slug, url }
	 * per source — url is the canonical frontend path (never derived from
	 * entity_id). Sources without a url render as non-linking labels.
	 */
	interface Props {
		sources: AiCitationSource[];
	}

	let { sources }: Props = $props();
</script>

{#if sources.length > 0}
	<div class="mt-1.5 flex flex-wrap items-center gap-1.5">
		<span class="flex items-center gap-1 text-xs" style="color: var(--color-muted)">
			<BookOpen size={12} /> {t('ai.chat.sources') ?? 'Sources'}:
		</span>
		{#each sources as s, i (i)}
			{#if s.url}
				<a
					href={s.url}
					class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors hover:opacity-80"
					style="border-color: var(--color-border); color: var(--color-text)"
				>
					{s.title ?? s.entity_type ?? 'content'}
					{#if s.similarity}
						<span style="color: var(--color-muted)">{Math.round(s.similarity * 100)}%</span>
					{/if}
				</a>
			{:else}
				<span
					class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
					style="border-color: var(--color-border); color: var(--color-muted)"
				>
					{s.title ?? s.entity_type ?? 'content'}
					{#if s.similarity}
						<span style="color: var(--color-muted)">{Math.round(s.similarity * 100)}%</span>
					{/if}
				</span>
			{/if}
		{/each}
	</div>
{/if}
