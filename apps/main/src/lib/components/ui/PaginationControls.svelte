<script lang="ts">
	import { ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		page: number;
		totalPages: number;
		onPageChange: (page: number) => void;
	}

	let { page, totalPages, onPageChange }: Props = $props();

	let canPrev = $derived(page > 1);
	let canNext = $derived(page < totalPages);
</script>

<nav class="flex items-center gap-2" aria-label={t('pagination.label', 'Pagination')}>
	<button
		type="button"
		onclick={() => canPrev && onPageChange(page - 1)}
		disabled={!canPrev}
		class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:cursor-not-allowed disabled:opacity-40"
		aria-label={t('pagination.prev', 'Previous page')}
	>
		<ChevronLeft size={16} />
	</button>

	<span class="text-sm text-[var(--color-text-subtle)]">
		{page} / {totalPages}
	</span>

	<button
		type="button"
		onclick={() => canNext && onPageChange(page + 1)}
		disabled={!canNext}
		class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:cursor-not-allowed disabled:opacity-40"
		aria-label={t('pagination.next', 'Next page')}
	>
		<ChevronRight size={16} />
	</button>
</nav>
