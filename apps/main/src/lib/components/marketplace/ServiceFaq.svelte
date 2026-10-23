<script lang="ts">
	// FAQ accordion — listing-authored faqs[] (§114). Bits UI Accordion,
	// same pattern as the pagebuilder FaqBlock. Rendered only when the
	// listing carries faqs — callers gate on faqs.length.
	// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.3)
	import { Accordion } from 'bits-ui';
	import { ArrowRight, ChevronDown } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import type { ServiceFaq } from '$lib/types';

	interface Props {
		faqs: ServiceFaq[];
		heading?: string;
		/** Optional "View all FAQs" link beside the heading (mockup header). */
		viewAllHref?: string;
	}
	let { faqs, heading, viewAllHref }: Props = $props();
</script>

<section aria-labelledby="service-faq-heading">
	<div class="flex flex-wrap items-baseline justify-between gap-2">
		<h2 id="service-faq-heading" class="text-xl font-bold text-[var(--color-text)]">
			{heading || t('services.faq_heading', 'Frequently asked questions')}
		</h2>
		{#if viewAllHref}
			<a
				href={viewAllHref}
				class="inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:underline"
			>
				{t('services.faq_view_all', 'View all FAQs')}
				<ArrowRight class="rtl:-scale-x-100" size={12} aria-hidden="true" />
			</a>
		{/if}
	</div>
	<Accordion.Root type="single" class="mt-4 space-y-2">
		{#each faqs as faq, i (i)}
			<Accordion.Item
				value={`faq-${i}`}
				class="overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]"
			>
				<Accordion.Header>
					<Accordion.Trigger
						class="flex w-full items-center justify-between gap-3 px-4 py-3 text-start text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] [&[data-state=open]>svg]:rotate-180"
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
