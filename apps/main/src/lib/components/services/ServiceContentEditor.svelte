<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// §114/000829 structured detail-page content editor — one collapsed
	// <details> section shared by the create and edit forms. Covers:
	//   selling_points[] — hero trust chips (icon + title + description)
	//   facts[]          — hero facts card (icon + label + value)
	//   highlights[]     — checklist lines
	//   audience[]       — "who this is for" checklist lines
	//   included[]       — "what's included" icon cards
	//   faqs[] / steps[] — accordion / step rail (steps carry an icon)
	// The client caps mirror the seeded system_settings
	// (service_marketplace.content.max_*); the backend re-validates
	// against the live settings, so the caps only hide the "add" buttons.
	// Icons pick from the marketplaceIcon whitelist — server-provided
	// names resolve through it at render time.
	import { ChevronDown, Plus, Trash2 } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import {
		marketplaceIcon,
		MARKETPLACE_ICON_NAMES
	} from '$lib/components/marketplace/marketplaceIcon';

	export interface IconEntry {
		icon: string;
		title: string;
		description: string;
	}
	export interface FactEntry {
		icon: string;
		label: string;
		value: string;
	}
	export interface FaqEntry {
		question: string;
		answer: string;
	}
	export interface StepEntry {
		icon: string;
		title: string;
		description: string;
	}

	const MAX_FAQS = 12;
	const MAX_STEPS = 8;
	const MAX_SELLING_POINTS = 6;
	const MAX_FACTS = 8;
	const MAX_INCLUDED = 12;
	const MAX_HIGHLIGHTS = 12;
	const MAX_AUDIENCE = 12;

	interface Props {
		inputClass: string;
		/** Open the section by default (edit form: when content exists). */
		open?: boolean;
		/** Render the fields without the collapsible "Content details (optional)"
		 *  wrapper — for hosts that already provide the section title (e.g. the
		 *  mentorship settings "Content details" tab). */
		bare?: boolean;
		faqs: FaqEntry[];
		steps: StepEntry[];
		sellingPoints: IconEntry[];
		facts: FactEntry[];
		included: IconEntry[];
		highlights: string[];
		audience: string[];
	}
	let {
		inputClass,
		open = $bindable(false),
		bare = false,
		faqs = $bindable([]),
		steps = $bindable([]),
		sellingPoints = $bindable([]),
		facts = $bindable([]),
		included = $bindable([]),
		highlights = $bindable([]),
		audience = $bindable([])
	}: Props = $props();
</script>

{#snippet iconSelect(value: string, onpick: (name: string) => void, label: string)}
	{@const Preview = marketplaceIcon(value)}
	<div class="flex items-center gap-2">
		<span
			class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]"
			aria-hidden="true"
		>
			<Preview size={14} class="text-[var(--color-primary)]" />
		</span>
		<select
			value={value}
			onchange={(e) => onpick(e.currentTarget.value)}
			class="{inputClass} w-auto min-w-0 flex-1 sm:flex-none"
			aria-label={label}
		>
			<option value="">{t('services.icon_none', 'No icon')}</option>
			{#each MARKETPLACE_ICON_NAMES as name (name)}
				<option value={name}>{name}</option>
			{/each}
		</select>
	</div>
{/snippet}

<!-- §114 structured content — all listing kinds; collapsed by default
	so the form stays lean. -->
{#snippet fields()}
		<!-- Selling points — hero trust chips -->
		<div>
			<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('services.selling_points_items', 'Selling points')}
			</span>
			<p class="mb-2 text-xs text-[var(--color-muted)]">
				{t(
					'services.selling_points_hint',
					'Short trust signals shown under the title (e.g. "High Success Rate").'
				)}
			</p>
			<div class="space-y-2">
				{#each sellingPoints as point, idx (idx)}
					<div
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<div class="flex items-start gap-2">
							<div class="flex-1 space-y-2">
								{@render iconSelect(point.icon, (v) => (point.icon = v), t('services.icon_pick', 'Icon'))}
								<input
									type="text"
									bind:value={point.title}
									placeholder={t('services.point_title_ph', 'Title (e.g. High Success Rate)')}
									class={inputClass}
									aria-label={t('services.point_title_ph', 'Title')}
								/>
								<input
									type="text"
									bind:value={point.description}
									placeholder={t('services.point_desc_ph', 'Short description (optional)')}
									class={inputClass}
									aria-label={t('services.point_desc_ph', 'Short description')}
								/>
							</div>
							<button
								type="button"
								onclick={() => (sellingPoints = sellingPoints.filter((_, i) => i !== idx))}
								class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
								aria-label={t('common.remove', 'Remove')}
							>
								<Trash2 size={16} />
							</button>
						</div>
					</div>
				{/each}
			</div>
			{#if sellingPoints.length < MAX_SELLING_POINTS}
				<button
					type="button"
					onclick={() =>
						(sellingPoints = [...sellingPoints, { icon: '', title: '', description: '' }])}
					class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Plus size={14} /> {t('services.selling_points_add', 'Add selling point')}
				</button>
			{/if}
		</div>

		<!-- Facts — the card beside/under the hero image -->
		<div>
			<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('services.facts_items', 'Hero facts')}
			</span>
			<p class="mb-2 text-xs text-[var(--color-muted)]">
				{t(
					'services.facts_hint',
					'Key facts shown next to the hero image (e.g. "Processing time — 3–10 days").'
				)}
			</p>
			<div class="space-y-2">
				{#each facts as fact, idx (idx)}
					<div
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<div class="flex items-start gap-2">
							<div class="flex-1 space-y-2">
								{@render iconSelect(fact.icon, (v) => (fact.icon = v), t('services.icon_pick', 'Icon'))}
								<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
									<input
										type="text"
										bind:value={fact.label}
										placeholder={t('services.fact_label_ph', 'Label (e.g. Processing time)')}
										class={inputClass}
										aria-label={t('services.fact_label_ph', 'Label')}
									/>
									<input
										type="text"
										bind:value={fact.value}
										placeholder={t('services.fact_value_ph', 'Value (e.g. 3–10 days)')}
										class={inputClass}
										aria-label={t('services.fact_value_ph', 'Value')}
									/>
								</div>
							</div>
							<button
								type="button"
								onclick={() => (facts = facts.filter((_, i) => i !== idx))}
								class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
								aria-label={t('common.remove', 'Remove')}
							>
								<Trash2 size={16} />
							</button>
						</div>
					</div>
				{/each}
			</div>
			{#if facts.length < MAX_FACTS}
				<button
					type="button"
					onclick={() => (facts = [...facts, { icon: '', label: '', value: '' }])}
					class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Plus size={14} /> {t('services.facts_add', 'Add fact')}
				</button>
			{/if}
		</div>

		<!-- Included — "what's included" icon cards -->
		<div>
			<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('services.included_items', "What's included")}
			</span>
			<p class="mb-2 text-xs text-[var(--color-muted)]">
				{t(
					'services.included_hint',
					'Feature cards shown in the included grid (packages fall back to the items list when empty).'
				)}
			</p>
			<div class="space-y-2">
				{#each included as entry, idx (idx)}
					<div
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<div class="flex items-start gap-2">
							<div class="flex-1 space-y-2">
								{@render iconSelect(entry.icon, (v) => (entry.icon = v), t('services.icon_pick', 'Icon'))}
								<input
									type="text"
									bind:value={entry.title}
									placeholder={t('services.included_title_ph', 'Title (e.g. Document review)')}
									class={inputClass}
									aria-label={t('services.included_title_ph', 'Title')}
								/>
								<input
									type="text"
									bind:value={entry.description}
									placeholder={t('services.point_desc_ph', 'Short description (optional)')}
									class={inputClass}
									aria-label={t('services.point_desc_ph', 'Short description')}
								/>
							</div>
							<button
								type="button"
								onclick={() => (included = included.filter((_, i) => i !== idx))}
								class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
								aria-label={t('common.remove', 'Remove')}
							>
								<Trash2 size={16} />
							</button>
						</div>
					</div>
				{/each}
			</div>
			{#if included.length < MAX_INCLUDED}
				<button
					type="button"
					onclick={() => (included = [...included, { icon: '', title: '', description: '' }])}
					class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Plus size={14} /> {t('services.included_add', 'Add included item')}
				</button>
			{/if}
		</div>

		<!-- Highlights — one checklist line per row -->
		<div>
			<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('services.highlights_items', 'Highlights')}
			</span>
			<HintIcon text={t('services.highlights_hint', 'Checklist lines shown in the highlights card.')} />
			<div class="space-y-2">
				{#each highlights as line, idx (idx)}
					<div class="flex items-center gap-2">
						<input
							type="text"
							value={line}
							oninput={(e) => (highlights[idx] = e.currentTarget.value)}
							placeholder={t('services.highlight_line_ph', 'e.g. Airport pickup included')}
							class={inputClass}
							aria-label={t('services.highlight_line_ph', 'Highlight')}
						/>
						<button
							type="button"
							onclick={() => (highlights = highlights.filter((_, i) => i !== idx))}
							class="text-[var(--color-muted)] hover:text-[var(--color-danger)]"
							aria-label={t('common.remove', 'Remove')}
						>
							<Trash2 size={16} />
						</button>
					</div>
				{/each}
			</div>
			{#if highlights.length < MAX_HIGHLIGHTS}
				<button
					type="button"
					onclick={() => (highlights = [...highlights, ''])}
					class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Plus size={14} /> {t('services.highlights_add', 'Add highlight')}
				</button>
			{/if}
		</div>

		<!-- Audience — "who this is for" checklist lines -->
		<div>
			<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('services.audience_items', 'Who this is for')}
			</span>
			<HintIcon text={t('services.audience_hint', 'Checklist lines describing the ideal customer.')} />
			<div class="space-y-2">
				{#each audience as line, idx (idx)}
					<div class="flex items-center gap-2">
						<input
							type="text"
							value={line}
							oninput={(e) => (audience[idx] = e.currentTarget.value)}
							placeholder={t('services.audience_line_ph', 'e.g. Students planning to study abroad')}
							class={inputClass}
							aria-label={t('services.audience_line_ph', 'Audience line')}
						/>
						<button
							type="button"
							onclick={() => (audience = audience.filter((_, i) => i !== idx))}
							class="text-[var(--color-muted)] hover:text-[var(--color-danger)]"
							aria-label={t('common.remove', 'Remove')}
						>
							<Trash2 size={16} />
						</button>
					</div>
				{/each}
			</div>
			{#if audience.length < MAX_AUDIENCE}
				<button
					type="button"
					onclick={() => (audience = [...audience, ''])}
					class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Plus size={14} /> {t('services.audience_add', 'Add audience line')}
				</button>
			{/if}
		</div>

		<!-- Steps — how-it-works rail with optional icons -->
		<div>
			<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('services.steps_items', 'How it works')}
			</span>
			<p class="mb-2 text-xs text-[var(--color-muted)]">
				{t(
					'services.steps_hint',
					'Walk travellers through the process — rendered as numbered steps on your listing.'
				)}
			</p>
			<div class="space-y-2">
				{#each steps as step, idx (idx)}
					<div
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<div class="flex items-start gap-2">
							<div class="flex-1 space-y-2">
								{@render iconSelect(step.icon, (v) => (step.icon = v), t('services.icon_pick', 'Icon'))}
								<input
									type="text"
									bind:value={step.title}
									placeholder={t('services.step_title_ph', 'Step title (e.g. Initial consultation)')}
									class={inputClass}
									aria-label={t('services.step_title_ph', 'Step title')}
								/>
								<input
									type="text"
									bind:value={step.description}
									placeholder={t('services.step_desc_ph', 'Short description (optional)')}
									class={inputClass}
									aria-label={t('services.step_desc_ph', 'Short description')}
								/>
							</div>
							<button
								type="button"
								onclick={() => (steps = steps.filter((_, i) => i !== idx))}
								class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
								aria-label={t('common.remove', 'Remove')}
							>
								<Trash2 size={16} />
							</button>
						</div>
					</div>
				{/each}
			</div>
			{#if steps.length < MAX_STEPS}
				<button
					type="button"
					onclick={() => (steps = [...steps, { icon: '', title: '', description: '' }])}
					class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Plus size={14} /> {t('services.steps_add', 'Add step')}
				</button>
			{/if}
		</div>

		<!-- FAQs -->
		<div>
			<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('services.faq_items', 'FAQs')}
			</span>
			<p class="mb-2 text-xs text-[var(--color-muted)]">
				{t(
					'services.faq_hint',
					'Answer the questions travellers ask most — shown as an accordion on your listing.'
				)}
			</p>
			<div class="space-y-2">
				{#each faqs as faq, idx (idx)}
					<div
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<div class="flex items-start gap-2">
							<div class="flex-1 space-y-2">
								<input
									type="text"
									bind:value={faq.question}
									placeholder={t(
										'services.faq_question_ph',
										'Question (e.g. How long does processing take?)'
									)}
									class={inputClass}
									aria-label={t('services.faq_question_ph', 'Question')}
								/>
								<textarea
									bind:value={faq.answer}
									rows={2}
									placeholder={t('services.faq_answer_ph', 'Answer')}
									class="{inputClass} resize-y"
									aria-label={t('services.faq_answer_ph', 'Answer')}
								></textarea>
							</div>
							<button
								type="button"
								onclick={() => (faqs = faqs.filter((_, i) => i !== idx))}
								class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
								aria-label={t('common.remove', 'Remove')}
							>
								<Trash2 size={16} />
							</button>
						</div>
					</div>
				{/each}
			</div>
			{#if faqs.length < MAX_FAQS}
				<button
					type="button"
					onclick={() => (faqs = [...faqs, { question: '', answer: '' }])}
					class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					<Plus size={14} /> {t('services.faq_add', 'Add question')}
				</button>
			{/if}
		</div>
{/snippet}

{#if bare}
	<!-- No <details> here: a summary-less <details> still renders the
	     browser's default "Details" disclosure triangle. -->
	<div class="space-y-5">
		{@render fields()}
	</div>
{:else}
	<details
		class="group rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]/40"
		bind:open
	>
		<summary
			class="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-semibold text-[var(--color-text)] [&::-webkit-details-marker]:hidden"
		>
			{t('services.content_details_section', 'Content details (optional)')}
			<ChevronDown
				size={16}
				class="ms-auto shrink-0 text-[var(--color-muted)] transition-transform group-open:rotate-180"
			/>
		</summary>
		<div class="space-y-5 border-t border-[var(--color-border)] px-4 py-4">
			{@render fields()}
		</div>
	</details>
{/if}
