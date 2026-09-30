<!-- Ref: japaconnect-engineering/sections/39-space-types-university.md
     (university_course_offerings — public offering detail) -->
<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { GraduationCap, ArrowLeft, GitCompare, CalendarDays, Clock, Globe, Award, ListChecks, Wallet, Building2 } from 'lucide-svelte';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { formatMoney } from '$lib/utils/currency';

	let { data }: { data: PageData } = $props();

	const offering = $derived(data.offering);
	const title = $derived(`${offering.course.title} — ${offering.university.name} · JapaConnect`);
	const description = $derived(
		offering.course.description ?? t('course_offering.meta_description', 'Degree program details, tuition, duration and entry requirements.')
	);

	const facts = $derived(
		(
			[
				{ icon: Building2, label: t('course_offering.university', 'University'), value: offering.university.name, href: `/universities/${offering.university.slug}` },
				{ icon: Award, label: t('course_offering.degree_type', 'Degree'), value: offering.course.degree_type?.toUpperCase() ?? null },
				{ icon: GraduationCap, label: t('course_offering.department', 'Department'), value: offering.course.department },
				{ icon: Wallet, label: t('course_offering.tuition', 'Tuition'), value: offering.tuition != null ? formatMoney(offering.tuition, offering.currency, data.currencies) : null },
				{ icon: Clock, label: t('course_offering.duration', 'Duration'), value: offering.duration_years ? t('course_offering.years', '{n} years', { n: offering.duration_years }) : null },
				{ icon: CalendarDays, label: t('course_offering.study_mode', 'Study mode'), value: offering.study_mode },
				{ icon: CalendarDays, label: t('course_offering.intakes', 'Intakes'), value: offering.intake_months?.length ? offering.intake_months.join(', ') : null },
				{ icon: Globe, label: t('course_offering.language', 'Language'), value: offering.course.language?.toUpperCase() ?? null },
				{ icon: Award, label: t('course_offering.accreditation', 'Accreditation'), value: offering.accreditation }
			] as { icon: typeof Building2; label: string; value: string | null; href?: string }[]
		).filter((f) => f.value != null && f.value !== '')
	);

	const jsonLd = $derived({
		'@context': 'https://schema.org',
		'@type': 'Course',
		name: offering.course.title,
		description: offering.course.description ?? undefined,
		provider: {
			'@type': 'CollegeOrUniversity',
			name: offering.university.name,
			url: `${page.url.origin}/universities/${offering.university.slug}`
		},
		offers:
			offering.tuition != null
				? {
						'@type': 'Offer',
						price: offering.tuition,
						priceCurrency: offering.currency ?? undefined
					}
				: undefined
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<!-- Header -->
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-4">
		<a
			href="/universities/{offering.university.slug}"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			data-sveltekit-preload-data="hover"
		>
			<ArrowLeft size={16} /> {offering.university.name}
		</a>
		<div class="flex items-start justify-between gap-3">
			<h1 class="text-xl font-bold text-[var(--color-text)]">{offering.course.title}</h1>
			{#if offering.course.degree_type}
				<span class="shrink-0 rounded border border-[var(--color-border)] px-2 py-0.5 text-xs font-semibold uppercase text-[var(--color-muted)]">
					{offering.course.degree_type}
				</span>
			{/if}
		</div>
		{#if offering.course.field_of_study}
			<p class="mt-1 text-sm text-[var(--color-muted)]">{offering.course.field_of_study}</p>
		{/if}
	</div>

	<!-- Facts -->
	{#if facts.length > 0}
		<dl class="grid grid-cols-1 gap-x-6 gap-y-3 px-4 py-4 sm:grid-cols-2">
			{#each facts as fact (fact.label)}
				{@const Icon = fact.icon}
				<div class="flex items-start gap-2.5">
					<Icon size={16} class="mt-0.5 shrink-0 text-[var(--color-muted)]" />
					<div class="min-w-0">
						<dt class="text-xs text-[var(--color-muted)]">{fact.label}</dt>
						<dd class="text-sm font-medium text-[var(--color-text)]">
							{#if fact.href}
								<a href={fact.href} class="text-[var(--color-primary)] hover:underline" data-sveltekit-preload-data="hover">{fact.value}</a>
							{:else}
								{fact.value}
							{/if}
						</dd>
					</div>
				</div>
			{/each}
		</dl>
	{/if}

	<!-- Description + requirements -->
	<div class="space-y-4 px-4 pb-6">
		{#if offering.course.description}
			<section>
				<h2 class="mb-1 text-sm font-semibold text-[var(--color-text)]">{t('course_offering.about', 'About this program')}</h2>
				<p class="whitespace-pre-line text-sm leading-relaxed text-[var(--color-text-subtle)]">{offering.course.description}</p>
			</section>
		{/if}

		{#if offering.entry_requirements}
			<section>
				<h2 class="mb-1 flex items-center gap-1.5 text-sm font-semibold text-[var(--color-text)]">
					<ListChecks size={15} /> {t('course_offering.entry_requirements', 'Entry requirements')}
				</h2>
				<p class="whitespace-pre-line text-sm leading-relaxed text-[var(--color-text-subtle)]">{offering.entry_requirements}</p>
			</section>
		{/if}

		<a
			href={data.compareHref}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			data-sveltekit-preload-data="hover"
		>
			<GitCompare size={15} />
			{t('course_offering.compare', 'Compare with other programs')}
		</a>
	</div>
</div>
