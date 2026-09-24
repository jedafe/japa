<script lang="ts">
	import { t } from '$lib/i18n';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import { page } from '$app/state';
	import { Briefcase, MapPin, Clock, CheckCircle2, AlertCircle, ExternalLink, Pencil, Star, Calendar, Globe, Truck, FileText, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { initiateUpload, uploadToPresigned, confirmMedia } from '$lib/api/create';
	import type { JobDetail, MediaAsset } from '$lib/types';
	import type { PageData } from './$types';
	import { sanitizeContentHtml, safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const job = $derived<JobDetail | null>(data.job);
	const authed = $derived(data.authed);
	const canEdit = $derived(data.canEdit ?? false);

	let applyResult = $state<{ ok?: boolean; error?: string; status?: string } | null>(null);
	let applying = $state(false);
	let cvFile = $state<File | null>(null);
	let coverLetter = $state('');

	// The viewer's existing application — from SSR (my_application) or set
	// after a successful apply below. Drives the disabled "Applied" state.
	const myApplication = $derived(
		applyResult?.ok
			? { id: '', status: applyResult.status ?? 'applied' }
			: (job?.my_application ?? null)
	);

	const applyErrorMessage = $derived.by(() => {
		const code = applyResult?.error;
		if (!code) return null;
		const map: Record<string, string> = {
			already_applied: t('jobs.already_applied', 'You have already applied to this job.'),
			missing_resume_url: t('jobs.missing_resume', 'Please provide a resume link.'),
			job_not_open: t('jobs.job_not_open', 'This job is not accepting applications.'),
			jobs_disabled: t('jobs.job_not_open', 'This job is not accepting applications.'),
			auth_required: t('jobs.apply_auth_required', 'Sign in to apply for this job.'),
			validation_failed: t('jobs.apply_failed', 'Could not submit your application.'),
			cv_upload_failed: t('jobs.cv_upload_failed', 'Could not upload your CV — please try again or apply without it.'),
			apply_failed: t('jobs.apply_failed', 'Could not submit your application.')
		};
		return map[code] ?? t('jobs.apply_failed', 'Could not submit your application.');
	});

	const title = $derived(job ? `${job.title} — JapaConnect` : 'Job — JapaConnect');

	function field(key: string): string {
		return job?.field_values?.[key] ?? '';
	}

	// On-platform apply (§63): optional CV upload goes through the two-step
	// media upload (initiate → presigned PUT → confirm), then the CV URL +
	// cover letter are stored as a form_submissions row linked to the
	// job_application via submission_id.
	async function submitApplication() {
		if (!job || applying) return;
		applyResult = null;
		applying = true;
		try {
			const fv: Record<string, string> = {};
			if (cvFile) {
				const init = await initiateUpload({
					filename: cvFile.name,
					mime_type: cvFile.type || 'application/pdf',
					file_size: cvFile.size
				});
				if (init.error || !init.data) {
					applyResult = { error: 'cv_upload_failed' };
					return;
				}
				const up = await uploadToPresigned(init.data.presigned_url, cvFile);
				if (!up.ok) {
					applyResult = { error: 'cv_upload_failed' };
					return;
				}
				await confirmMedia(init.data.media_id);
				const media = await api.get<MediaAsset>(`/media/${init.data.media_id}`);
				if (media.data?.url) {
					fv.resume_url = media.data.url;
					fv.resume_filename = cvFile.name;
				}
			}
			if (coverLetter.trim()) fv.cover_letter = coverLetter.trim();

			let submissionId = '';
			if (Object.keys(fv).length > 0 && data.applicationFormId) {
				const sub = await api.post<{ id: string }>(
					`/forms/${data.applicationFormId}/submissions`,
					{ field_values: fv }
				);
				submissionId = sub.data?.id ?? '';
			}

			const res = await api.post<{ id: string; status: string }>(
				`/jobs/${job.id}/apply`,
				submissionId ? { submission_id: submissionId } : {}
			);
			if (res.error) {
				applyResult = { error: res.error.code ?? 'apply_failed' };
				return;
			}
			applyResult = { ok: true, status: res.data?.status ?? 'applied' };
		} finally {
			applying = false;
		}
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
			<div class="flex items-center gap-3">
				<a href="/jobs" class="text-xs text-[var(--color-muted)] hover:underline">
					← {t('jobs.back', 'Back to jobs')}
				</a>
				{#if canEdit}
					<a href="/jobs/{job.slug}/edit" class="flex items-center gap-1 text-xs text-[var(--color-primary)] hover:underline">
						<Pencil size={11} /> {t('common.edit', 'Edit')}
					</a>
				{/if}
			</div>
			<h1 class="mt-2 text-lg font-semibold text-[var(--color-text)]">
				{job.title}
				{#if field('is_featured') === 'true'}
					<span class="ml-2 inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/10 px-2.5 py-0.5 align-middle text-[11px] font-medium text-[var(--color-primary)]">
						<Star size={11} /> {t('jobs.featured', 'Featured')}
					</span>
				{/if}
			</h1>
			{#if job.source?.id}
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
				{#if field('experience_level')}
					<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{field('experience_level')}</span>
				{/if}
				{#if field('application_deadline')}
					<span class="flex items-center gap-1"><Calendar size={12} /> {t('jobs.deadline', 'Deadline')}: {field('application_deadline')}</span>
				{/if}
				{#if field('visa_sponsorship') === 'true'}
					<span class="flex items-center gap-1"><Globe size={12} /> {t('jobs.visa_sponsorship', 'Visa sponsorship')}</span>
				{/if}
				{#if field('relocation_assistance') === 'true'}
					<span class="flex items-center gap-1"><Truck size={12} /> {t('jobs.relocation', 'Relocation assistance')}</span>
				{/if}
			</div>
			{#if field('skills_required')}
				<div class="mt-2 flex flex-wrap gap-1.5">
					{#each field('skills_required').split(',').map((s) => s.trim()).filter(Boolean) as skill (skill)}
						<span class="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs text-[var(--color-muted)]">{skill}</span>
					{/each}
				</div>
			{/if}
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

				{#if field('application_url')}
					<p class="mb-3 text-sm text-[var(--color-muted)]">
						{t('jobs.apply_external_hint', 'This job is listed externally. Apply on the employer site.')}
					</p>
					<a
						href={field('application_url')}
						target="_blank"
						rel="noopener noreferrer"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90"
					>
						<ExternalLink size={14} /> {t('jobs.apply_external', 'Apply externally')}
					</a>
				{:else if !authed}
					<p class="text-sm text-[var(--color-muted)]">
						<a href="/login?redirect={encodeURIComponent(page.url.pathname)}" class="text-[var(--color-primary)] hover:underline">
							{t('common.sign_in', 'Sign in')}
						</a>
						{t('jobs.apply_auth_required', ' to apply for this job.')}
					</p>
				{:else if myApplication}
					<div class="flex flex-wrap items-center gap-3">
						<div class="flex items-center gap-2 text-sm text-[var(--color-success)]">
							<CheckCircle2 size={16} />
							{t('jobs.applied_state', 'You have applied to this job.')}
							<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs capitalize">{myApplication.status}</span>
						</div>
						<a href="/me/applications" class="text-xs text-[var(--color-primary)] hover:underline">
							{t('jobs.my_applications_link', 'My applications')} →
						</a>
					</div>
				{:else}
					<form
						onsubmit={(e) => { e.preventDefault(); void submitApplication(); }}
						class="space-y-3"
					>
						{#if applyErrorMessage}
							<div class="flex items-center gap-2 text-sm text-[var(--color-danger)]">
								<AlertCircle size={16} /> {applyErrorMessage}
							</div>
						{/if}

						<div class="space-y-1.5">
							<label for="job-cv" class="text-xs font-medium text-[var(--color-muted)]">
								{t('jobs.cv_label', 'CV / Resume')} <span class="font-normal">({t('common.optional', 'optional')})</span>
							</label>
							<div class="flex items-center gap-3">
								<label
									class="inline-flex cursor-pointer items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] transition-colors hover:border-[var(--color-primary)]"
								>
									<FileText size={14} />
									{cvFile ? cvFile.name : t('jobs.cv_choose', 'Choose file')}
									<input
										id="job-cv"
										type="file"
										accept=".pdf,.doc,.docx,.txt,.md"
										class="sr-only"
										onchange={(e) => { cvFile = e.currentTarget.files?.[0] ?? null; }}
									/>
								</label>
								{#if cvFile}
									<button
										type="button"
										onclick={() => (cvFile = null)}
										class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
									>
										{t('common.remove', 'Remove')}
									</button>
								{/if}
							</div>
							<p class="text-xs text-[var(--color-muted)]">
								{t('jobs.cv_formatter_hint', 'Need a polished CV?')}
								<a href="/tools/cv-formatter" target="_blank" rel="noopener" class="text-[var(--color-primary)] hover:underline">
									{t('jobs.cv_formatter_link', 'Open the CV formatter')}
								</a>
							</p>
						</div>

						<div class="space-y-1.5">
							<label for="job-cover" class="text-xs font-medium text-[var(--color-muted)]">
								{t('jobs.cover_letter', 'Cover letter')} <span class="font-normal">({t('common.optional', 'optional')})</span>
							</label>
							<textarea
								id="job-cover"
								rows="3"
								bind:value={coverLetter}
								placeholder={t('jobs.cover_letter_placeholder', 'Briefly introduce yourself…')}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)]"
							></textarea>
						</div>

						<button
							type="submit"
							disabled={applying}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:opacity-90 disabled:opacity-50"
						>
							{#if applying}<Loader2 size={14} class="animate-spin" />{/if}
							{applying ? t('common.submitting', 'Submitting…') : t('jobs.submit_application', 'Submit application')}
						</button>
					</form>
				{/if}
			</div>
		</div>
	{/if}
</div>
