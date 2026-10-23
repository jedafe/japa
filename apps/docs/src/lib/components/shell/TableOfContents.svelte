<script lang="ts">
	import { t } from '$lib/i18n';
	import type { TocHeading } from '$lib/tiptap/serialize';

	interface Props {
		headings: TocHeading[];
	}

	let { headings }: Props = $props();

	let activeId = $state<string>('');

	// ADR-060: IntersectionObserver (never scroll listeners), with cleanup.
	// Uses $effect keyed on headings so the observer re-initializes on
	// client-side navigation (same route, different params) — onMount only
	// runs once per component instance and would leave the observer pointing
	// at stale DOM elements after a page change.
	$effect(() => {
		if (headings.length === 0) return;
		const observer = new IntersectionObserver(
			(entries) => {
				// Pick the topmost intersecting heading as active.
				const visible = entries
					.filter((e) => e.isIntersecting)
					.sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
				if (visible.length > 0) {
					activeId = visible[0].target.id;
				}
			},
			{ rootMargin: '-10% 0% -80% 0%', threshold: 0 }
		);
		for (const h of headings) {
			const el = document.getElementById(h.id);
			if (el) observer.observe(el);
		}
		return () => observer.disconnect();
	});
</script>

<nav class="toc" aria-label={t('page.table_of_contents')}>
	<h2 class="toc-title">{t('page.table_of_contents')}</h2>
	{#if headings.length === 0}
		<p class="toc-empty"></p>
	{:else}
		<ul class="toc-list">
			{#each headings as h (h.id)}
				<li>
					<a
						href="#{h.id}"
						class="toc-link"
						class:level-3={h.level === 3}
						class:active={activeId === h.id}
					>
						{h.text}
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</nav>

<style>
	.toc-title {
		font-size: 0.6875rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		margin: 0 0 0.75rem;
	}
	.toc-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}
	.toc-link {
		display: block;
		padding: 0.25rem 0 0.25rem 0.75rem;
		font-size: 0.8125rem;
		line-height: 1.4;
		color: var(--color-text-subtle);
		text-decoration: none;
		border-inline-start: 2px solid transparent;
	}
	.toc-link:hover { color: var(--color-text); }
	.toc-link.active {
		color: var(--color-primary);
		border-inline-start-color: var(--color-primary);
		font-weight: 500;
	}
	.toc-link.level-3 { padding-inline-start: 1.5rem; font-size: 0.75rem; }
	.toc-empty { height: 0; }
</style>
