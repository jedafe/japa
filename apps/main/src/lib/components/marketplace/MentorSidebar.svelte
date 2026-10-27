<script lang="ts">
	// Mentorship right-column card stack — the §114 2026-10-10 mentor
	// mockup: "Mentorship details" spec card (price / response time /
	// availability / sessions / languages / formats / countries), the
	// "Ready to grow?" apply card with slots-left meter, and a small
	// "Questions?" contact card. Share row lives under the stack.
	import {
		Award,
		BadgeCheck,
		Briefcase,
		Clock,
		Globe,
		GraduationCap,
		Languages,
		MapPin,
		MessageSquare,
		MonitorSmartphone,
		Target,
		TrendingUp,
		Users
	} from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import type { ServiceListing } from '$lib/types';

	interface Props {
		service: ServiceListing;
		currencies: CurrencyInfo[];
		canApply: boolean;
		onApply: () => void;
		/** Omitted when DM isn't a valid channel (own listing) — hides the
		 *  contact card. */
		onMessage?: () => void;
	}
	let { service, currencies, canApply, onApply, onMessage }: Props = $props();

	const mentor = $derived(service.mentor);
	const mentorFull = $derived(!!mentor && mentor.available_slots <= 0);
	const mentorUnavailable = $derived(!mentor || mentor.is_mentor === false);
	const price = $derived(
		service.price_amount != null
			? formatMoney(service.price_amount, service.price_currency ?? '', currencies, {
					minor: true
				})
			: null
	);
	const perLabel = $derived(
		service.price_per === 'person'
			? t('services.pkg_per_person', 'per person')
			: service.price_per === 'group'
				? t('services.pkg_per_group', 'per group')
				: service.price_per === 'package'
					? t('services.pkg_per_package', 'per package')
					: ''
	);

	const yearsLabel = $derived(
		mentor?.years_experience
			? t('services.mentor_years_value', '{count}+ years', {
					count: mentor.years_experience
				})
			: undefined
	);
	const menteesLabel = $derived(
		mentor?.mentees_helped
			? t('services.mentor_mentees_value', '{count}+', { count: mentor.mentees_helped })
			: undefined
	);

	// Mockup "Mentorship Details" spec card — type / focus / countries /
	// expertise / experience / mentees / success rate, then the general
	// logistics rows the mentor card fields add on top.
	const detailRows = $derived(
		(
			[
				{ icon: MonitorSmartphone, label: t('services.mentor_session_type', 'Mentorship type'), value: mentor?.session_type },
				{ icon: Target, label: t('services.mentor_focus_areas', 'Focus areas'), value: (mentor?.areas ?? []).join(', ') || undefined },
				{ icon: Briefcase, label: t('services.mentor_profession', 'Profession'), value: mentor?.profession ?? undefined },
				{ icon: Globe, label: t('services.mentor_website', 'Website'), value: mentor?.website_url ? mentor.website_url.replace(/^https?:\/\//, '') : undefined, href: mentor?.website_url ?? undefined },
				{ icon: Globe, label: t('services.mentor_countries_covered', 'Countries covered'), value: (mentor?.countries ?? []).join(', ') || undefined },
				{ icon: Award, label: t('services.mentor_expertise', 'Expertise'), value: (mentor?.credentials ?? []).join(', ') || undefined },
				{ icon: GraduationCap, label: t('services.mentor_experience', 'Experience'), value: yearsLabel },
				{ icon: Users, label: t('services.mentor_mentees_helped', 'Mentees helped'), value: menteesLabel },
				{ icon: TrendingUp, label: t('services.mentor_success_rate', 'Success rate'), value: mentor?.success_label },
				{ icon: Clock, label: t('services.mentor_response_time', 'Response time'), value: mentor?.response_time },
				{ icon: BadgeCheck, label: t('services.mentor_availability', 'Availability'), value: mentor?.availability },
				{ icon: Languages, label: t('services.mentor_languages', 'Languages'), value: (mentor?.languages ?? []).join(', ') || undefined },
				{ icon: Briefcase, label: t('services.mentor_formats', 'Formats'), value: (mentor?.session_formats ?? []).join(', ') || undefined },
				{ icon: MapPin, label: t('services.mentor_location', 'Location'), value: mentor?.location }
			] as const
		).filter((r) => !!r.value)
	);
</script>

{#if mentor}
	<aside class="flex flex-col gap-4 lg:sticky lg:top-24">
		<!-- Mentorship details -->
		{#if detailRows.length > 0 || price}
			<div
				class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5"
			>
				<div class="flex items-baseline justify-between gap-2">
					<h3 class="text-sm font-bold text-[var(--color-text)]">
						{t('services.mentorship_details', 'Mentorship details')}
					</h3>
					{#if price}
						<p class="text-end">
							<span class="text-lg font-bold text-[var(--color-text)]">{price}</span>
							<span class="text-xs text-[var(--color-muted)]">{perLabel}</span>
						</p>
					{/if}
				</div>
				<dl class="mt-4 divide-y divide-[var(--color-border)]">
					{#each detailRows as row, i (i)}
						{@const Icon = row.icon}
						<div class="flex items-baseline justify-between gap-3 py-2.5">
							<dt
								class="flex min-w-0 shrink-0 items-center gap-2 text-xs text-[var(--color-text-subtle)]"
							>
								<Icon size={13} class="shrink-0 text-[var(--color-muted)]" aria-hidden="true" />
								<span class="truncate">{row.label}</span>
							</dt>
							<dd
								class="min-w-0 text-end text-xs font-semibold break-words text-[var(--color-text)]"
							>
								{#if 'href' in row && row.href}
									<a
										class="text-[var(--color-primary)] hover:underline"
										href={row.href}
										target="_blank"
										rel="noopener noreferrer nofollow">{row.value}</a>
								{:else}
									{row.value}
								{/if}
							</dd>
						</div>
					{/each}
				</dl>
			</div>
		{/if}

		<!-- Ready to grow? apply card -->
		<div
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-center"
		>
			<h3 class="text-sm font-bold text-[var(--color-text)]">
				{t('services.mentor_apply_heading', 'Ready to grow?')}
			</h3>
			<p class="mt-1 text-xs leading-relaxed text-[var(--color-muted)]">
				{t(
					'services.mentor_apply_body',
					'Apply for a mentorship spot and start your journey.'
				)}
			</p>
			<button
				type="button"
				disabled={!canApply}
				onclick={onApply}
				class="mt-4 inline-flex w-full items-center justify-center rounded-lg bg-[var(--color-primary)] px-4 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:cursor-not-allowed disabled:opacity-50"
			>
				{t('services.apply_mentorship', 'Apply for mentorship')}
			</button>
			{#if mentorUnavailable}
				<p class="mt-2.5 text-[11px] font-medium text-[var(--color-muted)]">
					{t('services.mentor_not_mentoring', 'This mentor is not currently accepting mentees.')}
				</p>
			{:else if mentorFull}
				<p class="mt-2.5 inline-flex items-center gap-1.5 text-[11px] font-medium text-[var(--color-warning,#b45309)]">
					<Users size={12} aria-hidden="true" />
					{t('services.mentor_at_capacity', 'At capacity — not accepting new mentees right now')}
				</p>
			{:else if mentor.capacity > 0}
				<p
					class="mt-2.5 inline-flex items-center gap-1.5 text-[11px] font-medium text-[var(--color-success)]"
				>
					<Users size={12} aria-hidden="true" />
					{t('services.mentor_slots_left', '{count} slots left', {
						count: Math.max(mentor.available_slots, 0)
					})}
				</p>
			{/if}
		</div>

		<!-- Questions? contact card -->
		{#if onMessage}
		<div
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 text-center"
		>
			<span
				class="mx-auto grid h-10 w-10 place-items-center rounded-full bg-[var(--color-primary)]/10"
			>
				<MessageSquare size={16} class="text-[var(--color-primary)]" aria-hidden="true" />
			</span>
			<h3 class="mt-2 text-sm font-bold text-[var(--color-text)]">
				{t('services.mentor_questions', 'Questions?')}
			</h3>
			<p class="mt-1 text-xs leading-relaxed text-[var(--color-muted)]">
				{t('services.mentor_questions_body', 'Message the mentor before you apply.')}
			</p>
			<button
				type="button"
				onclick={() => onMessage?.()}
				class="mt-3.5 inline-flex w-full items-center justify-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-semibold text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				<MessageSquare size={14} aria-hidden="true" />
				{t('services.mentor_contact', 'Contact mentor')}
			</button>
		</div>
		{/if}
	</aside>
{/if}
