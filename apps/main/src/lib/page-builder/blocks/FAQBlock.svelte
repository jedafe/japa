<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	interface FAQItem {
		question: string;
		answer: string;
	}

	interface Props {
		questions?: FAQItem[];
		title?: string;
		readOnly?: boolean;
	}

	let {
		questions = [],
		title = '',
		readOnly = false,
	}: Props = $props();

	let openIndices = $state<Set<number>>(new Set());

	function toggle(index: number) {
		const next = new Set(openIndices);
		if (next.has(index)) {
			next.delete(index);
		} else {
			next.add(index);
		}
		openIndices = next;
	}

	function isOpen(index: number): boolean {
		return openIndices.has(index);
	}
	let wrapperClass = $derived(
		`block-faq${!readOnly ? ' ring-2 ring-blue-400/40 hover:ring-blue-400/60' : ''}`
	);
</script>

<div class={wrapperClass}>
	{#if title}
		<h2 class="mb-6 text-2xl font-bold text-gray-900 dark:text-white">
			{title}
		</h2>
	{/if}

	{#if questions.length === 0}
		<div class="flex h-32 items-center justify-center rounded-lg border-2 border-dashed border-gray-300 bg-gray-50 text-sm text-gray-400 dark:border-gray-600 dark:bg-gray-800 dark:text-gray-500">
			No FAQ items added
		</div>
	{:else}
		<div class="divide-y divide-gray-200 dark:divide-gray-700">
			{#each questions as item, i}
				<div class="py-1">
					<button
						type="button"
						class="flex w-full items-center justify-between gap-4 py-4 text-left transition-colors hover:text-blue-600 dark:hover:text-blue-400"
						aria-expanded={isOpen(i)}
						onclick={() => toggle(i)}
					>
						<span class="text-base font-medium text-gray-900 dark:text-white">
							{item.question}
						</span>
						<svg
							class="h-5 w-5 shrink-0 text-gray-400 transition-transform {isOpen(i) ? 'rotate-180' : ''}"
							fill="none"
							viewBox="0 0 24 24"
							stroke="currentColor"
							stroke-width="2"
						>
							<path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" />
						</svg>
					</button>

					{#if isOpen(i)}
						<div class="pb-4 text-sm leading-relaxed text-gray-600 dark:text-gray-300">
							{item.answer}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
