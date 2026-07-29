<script lang="ts">
	import type { ComparisonEmbedProps } from '../types';
	import { ArrowRight } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	let { props }: { props: ComparisonEmbedProps } = $props();
</script>

<div class="mx-auto w-full max-w-4xl px-4 py-6">
	<a
		href={`/compare/${props.compare_slug}`}
		class="flex items-center justify-between rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-colors hover:bg-[var(--color-surface-subtle)]"
	>
		<span class="text-sm font-medium text-[var(--color-text)]">
			{t('pagebuilder.view_comparison', 'View comparison')}
		</span>
		<ArrowRight size={18} class="text-[var(--color-text-subtle)]" />
	</a>
</div>
