<script lang="ts">
	// FAQ accordion — listing-authored faqs[] (§114). Bits UI Accordion,
	// same pattern as the pagebuilder FaqBlock. Rendered only when the
	// listing carries faqs — callers gate on faqs.length.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { Accordion } from 'bits-ui';
	import { ChevronDown } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { ServiceFaq } from '$lib/types';

	interface Props {
		faqs: ServiceFaq[];
	}
	let { faqs }: Props = $props();
</script>

<section aria-labelledby="service-faq-heading">
	<h2
		id="service-faq-heading"
		class="text-sm font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]"
	>
		{t('services.faq_heading', 'Frequently asked questions')}
	</h2>
	<Accordion.Root type="single" class="mt-4 space-y-2">
		{#each faqs as faq, i (i)}
			<Accordion.Item
				value={`faq-${i}`}
				class="overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]"
			>
				<Accordion.Header>
					<Accordion.Trigger
						class="flex w-full items-center justify-between gap-3 px-4 py-3 text-left text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] [&[data-state=open]>svg]:rotate-180"
					>
						<span>{faq.question}</span>
						<ChevronDown
							size={16}
							class="shrink-0 text-[var(--color-muted)] transition-transform"
							aria-hidden="true"
						/>
					</Accordion.Trigger>
				</Accordion.Header>
				<Accordion.Content
					class="border-t border-[var(--color-border)] px-4 py-3 text-sm leading-relaxed text-[var(--color-text-subtle)]"
				>
					{faq.answer}
				</Accordion.Content>
			</Accordion.Item>
		{/each}
	</Accordion.Root>
</section>
