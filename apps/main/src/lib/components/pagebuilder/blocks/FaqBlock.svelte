<script lang="ts">
	import type { FaqProps } from '../types';
	import { Accordion } from 'bits-ui';
	import { ChevronDown } from 'lucide-svelte';

	let { props }: { props: FaqProps } = $props();
	const items = $derived(props.items ?? []);
</script>

{#if items.length > 0}
	<div class="mx-auto w-full max-w-2xl px-4 py-6">
		<Accordion.Root type="single" class="space-y-2">
			{#each items as item, i (i)}
				<Accordion.Item value={`faq-${i}`} class="overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					<Accordion.Header>
						<Accordion.Trigger
							class="flex w-full items-center justify-between gap-3 px-4 py-3 text-left text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] [&[data-state=open]>svg]:rotate-180"
						>
							<span>{item.question}</span>
							<ChevronDown size={16} class="shrink-0 text-[var(--color-muted)] transition-transform" />
						</Accordion.Trigger>
					</Accordion.Header>
					<Accordion.Content class="border-t border-[var(--color-border)] px-4 py-3 text-sm leading-relaxed text-[var(--color-text-subtle)]">
						{item.answer}
					</Accordion.Content>
				</Accordion.Item>
			{/each}
		</Accordion.Root>
	</div>
{/if}
