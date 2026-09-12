<script lang="ts">
	import { t } from '$lib/i18n';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import { page } from '$app/state';
	import { Briefcase, MapPin, Clock, CheckCircle2, AlertCircle, ExternalLink } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { enhance } from '$app/forms';
	import type { JobDetail } from '$lib/types';
	import type { PageData } from './$types';
	import { sanitizeContentHtml, safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const job = $derived<JobDetail | null>(data.job);
	const authed = $derived(data.authed);

	let applyResult = $state<{ ok?: boolean; error?: string; status?: string } | null>(null);
	let applying = $state(false);

	const title = $derived(job ? `${job.title} — JapaConnect` : 'Job — JapaConnect');

	function field(key: string): string {
		return job?.field_values?.[key] ?? '';
	}

	// Build a human-readable location string from city + country custom fields.
	const locationStr = $derived(() => {
		const city = field('city');
		const country = field('country');
		return [city, country].filter(Boolean).join(', ');
	});

	// Build a salary range string from salary_min/salary_max/salary_currency.
	const salaryStr = $derived(() => {
		const min = field('salary_min');
		const max = field('salary_max');
		const currency = field('salary_currency');
		if (!min && !max) return '';
		const range = [min, max].filter(Boolean).join('–');
		return currency ? `${currency} ${range}` : range;
	});

	// JSON-LD JobPosting for SEO (Ref: Section 63, ROADMAP-MAIN 36.03).
	const jsonLd = $derived(
		job
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'JobPosting',
					title: job.title,
					description: job.summary || job.body,
					datePublished: job.published_at,
					url: page.url.href,
					hiringOrganization: {
						'@type': 'Organization',
						name: field('company_name') || 'JapaConnect'
					},
					jobLocation: locationStr()
						? { '@type': 'Place', address: locationStr() }
						: undefined,
					employmentType: field('job_type') || undefined,
					estimatedSalary: salaryStr() || undefined
				})
			: ''
	);
</script>

<svelte:head>
	<title>{title}</title>
	{#if job?.summary}
		<meta name="description" content={job.summary} />
	{/if}
	<meta property="og:title" content={job?.title ?? 'Job'} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	{#if !job}
		<div class="p-4">
			<EmptyState icon={Briefcase} title={t('jobs.not_found', 'Job not found')} />
		</div>
	{:else}
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<a href="/jobs" class="text-xs text-[var(--color-muted)] hover:underline">
				← {t('jobs.back', 'Back to jobs')}
			</a>
			<h1 class="mt-2 text-lg font-semibold text-[var(--color-text)]">{job.title}</h1>
			{#if job.source}
				{@const source = job.source}
				<div class="mt-1.5 flex items-center gap-1 text-sm">
					<EntityHoverChip
						type="source"
						id={source.id}
						slug={source.slug}
						name={source.name}
						avatar={source.logo_url}
						class="inline-flex items-center gap-0.5 font-medium text-[var(--color-primary)] hover:underline"
					>
						{#snippet trigger()}
							{t('jobs.source', 'Source')}: {source.name}
						{/snippet}
					</EntityHoverChip>
					{#if source.source_url}
						<a
							href={source.source_url}
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-0.5 text-[var(--color-muted)] hover:text-[var(--color-primary)]"
							aria-label={t('jobs.original_source', 'View original source')}
						>
							<ExternalLink size={14} />
						</a>
					{/if}
				</div>
			{/if}
			<div class="mt-2 flex flex-wrap gap-3 text-xs text-[var(--color-muted)]">
				{#if field('company_name')}
					<span class="flex items-center gap-1"><Briefcase size={12} /> {field('company_name')}</span>
				{/if}
				{#if locationStr()}
					<span class="flex items-center gap-1"><MapPin size={12} /> {locationStr()}</span>
				{/if}
				{#if field('job_type')}
					<span class="flex items-center gap-1"><Clock size={12} /> {field('job_type')}</span>
				{/if}
				{#if field('location_type')}
					<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field('location_type')}</span>
				{/if}
				{#if salaryStr()}
					<span>{salaryStr()}</span>
				{/if}
			</div>
		</div>

		<div class="p-4">
			{#if job.summary}
				<p class="mb-3 text-sm text-[var(--color-muted)]">{job.summary}</p>
			{/if}
			{#if job.body}
				<div class="prose prose-sm max-w-none text-[var(--color-text)]">
					{@html sanitizeContentHtml(job.body)}
				</div>
			{/if}

			<!-- Apply form -->
			<div class="mt-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
					{t('jobs.apply', 'Apply for this job')}
				</h2>

				{#if !authed}
					<p class="text-sm text-[var(--color-muted)]">
						<a href="/login?redirect={encodeURIComponent(page.url.pathname)}" class="text-[var(--color-primary)] hover:underline">
							{t('common.sign_in', 'Sign in')}
						</a>
						{t('jobs.apply_auth_required', ' to apply for this job.')}
					</p>
				{:else if applyResult?.ok}
					<div class="flex items-center gap-2 text-sm text-[var(--color-success)]">
						<CheckCircle2 size={16} />
						{t('jobs.applied', 'Application submitted! Status')}: {applyResult.status}
					</div>
				{:else}
					<form
						method="POST"
						action="?/apply"
						use:enhance={() => {
							applying = true;
							return async ({ result }) => {
								applying = false;
								if (result.type === 'success') {
									applyResult = result.data as { ok?: boolean; error?: string; status?: string };
								}
							};
						}}
						class="space-y-3"
					>
						{#if applyResult?.error}
							<div class="flex items-center gap-2 text-sm text-[var(--color-danger)]">
								<AlertCircle size={16} /> {applyResult.error}
							</div>
						{/if}
						<input type="hidden" name="job_id" value={job.id} />
						<button
							type="submit"
							disabled={applying}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90 disabled:opacity-50"
						>
							{applying ? t('common.submitting', 'Submitting…') : t('jobs.submit_application', 'Submit application')}
						</button>
					</form>
				{/if}
			</div>
		</div>
	{/if}
</div>
