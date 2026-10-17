<script lang="ts">
	import { t, i18n, siteName} from '$lib/i18n';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import { page } from '$app/state';
	import {
		Briefcase, MapPin, Clock, CheckCircle2, AlertCircle, ExternalLink, Pencil,
		Calendar, Globe, Truck, FileText, Loader2, BadgeCheck, Bookmark, Share2,
		Flag, ArrowRight, ChevronRight, Building2, Wallet, GraduationCap, X,
		UserRound, Layers
	} from 'lucide-svelte';
	import { Modal } from '$lib/components/ui';
	import { EmptyState, AutocompleteSelect } from '$lib/components/ui';
	import RecommendedContentCard from '$lib/components/content/RecommendedContentCard.svelte';
	import { api } from '$lib/api/client';
	import { initiateUpload, uploadToPresigned, confirmMedia } from '$lib/api/create';
	import { invalidateAll } from '$app/navigation';
	import { getViewerState, updateViewerState } from '$lib/api/viewerState';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { relativeTime } from '$lib/feed/time';
	import type { JobDetail, MediaAsset } from '$lib/types';
	import type { PageData } from './$types';
	import { sanitizeContentHtml, safeJsonLdScript } from '$lib/utils/safeHtml';
	import { safeHttpUrl } from '$lib/utils/safeUrl';
	import { salaryPeriodLabel } from '$lib/utils/jobs';
	import { currencySymbol } from '$lib/utils/currency';

	let { data }: { data: PageData } = $props();

	const job = $derived<JobDetail | null>(data.job);
	const authed = $derived(data.authed);
	const canEdit = $derived(data.canEdit ?? false);

	let applyResult = $state<{ ok?: boolean; error?: string; status?: string } | null>(null);
	let applying = $state(false);
	let cvFile = $state<File | null>(null);
	let coverLetter = $state('');

	// Browsers sometimes report an empty MIME for .doc/.md/.rtf — infer the
	// declared type from the extension so /media/upload validation succeeds.
	const CV_EXT_MIME: Record<string, string> = {
		pdf: 'application/pdf',
		doc: 'application/msword',
		docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
		odt: 'application/vnd.oasis.opendocument.text',
		rtf: 'application/rtf',
		txt: 'text/plain',
		md: 'text/markdown'
	};

	function cvMimeType(file: File): string {
		if (file.type) return file.type;
		const ext = file.name.split('.').pop()?.toLowerCase() ?? '';
		return CV_EXT_MIME[ext] ?? 'application/pdf';
	}

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

	const title = $derived(job ? `${job.title} — ${siteName()}` : 'Job — ${siteName()}');
	// Section 26 — self-referencing canonical on the clean slug URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(job ? `${page.url.origin}/jobs/${job.slug}` : `${page.url.origin}/jobs`);
	// Fallback excerpt when job.summary is null — strip HTML from the body
	// so the meta description is never empty (ResolveMetadata fallback).
	const fallbackDescription = $derived(
		job
			? (job.summary?.trim() ||
					(job.body ?? '').replace(/<[^>]*>/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 160) ||
					job.title)
			: ''
	);

	function field(key: string): string {
		return job?.field_values?.[key] ?? '';
	}

	// Select fields store their machine value (full_time, remote, …) — the
	// detail endpoint doesn't return option labels, so prettify the token.
	function prettify(value: string): string {
		return value
			.split(/[_-]+/)
			.filter(Boolean)
			.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
			.join(' ');
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
					mime_type: cvMimeType(cvFile),
					file_size: cvFile.size,
					context: 'document'
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
			// Refresh SSR-derived state (my_application) after a successful apply.
			await invalidateAll().catch(() => {});
		} catch {
			applyResult = { error: 'apply_failed' };
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

	// Build a salary range string from salary_min/salary_max/salary_currency,
	// suffixed with the pay period ("USD 5,000 – 8,000 per month").
	const salaryStr = $derived(() => {
		const min = field('salary_min');
		const max = field('salary_max');
		const currency = field('salary_currency');
		if (!min && !max) return '';
		const range = [min, max].filter(Boolean).join(' – ');
		const period = salaryPeriodLabel(field('salary_period'));
		const sym = currency ? currencySymbol(currency, data.currencies) : '';
		// Symbol prefix sits tight ("₦5,000"); a bare-code fallback keeps a
		// space ("USD 5,000").
		const amount = sym ? (sym === currency.trim().toUpperCase() ? `${sym} ${range}` : `${sym}${range}`) : range;
		return period ? `${amount} ${period}` : amount;
	});

	// job_function values are seeded select tokens (engineering, hr, …) with
	// i18n labels; fall back to the prettified token for unseeded values.
	const jobFunctionLabel = $derived(
		field('job_function') ? t(`jobs.func_${field('job_function')}`, prettify(field('job_function'))) : ''
	);

	const companyName = $derived(field('company_name') || job?.poster?.name || '');
	const companyLogo = $derived(field('company_logo') || job?.poster?.logo_url || '');
	const companyVerified = $derived(data.org?.verified ?? job?.poster?.verified ?? false);
	const orgHref = $derived(
		job?.poster?.kind === 'organization' && job.poster.slug ? `/org/${job.poster.slug}` : ''
	);
	const externalApplyUrl = $derived(safeHttpUrl(field('application_url')));
	const postedAgo = $derived(job ? relativeTime(job.published_at, new Date(), i18n.locale) : '');
	// job_type / location_type have seeded i18n labels (jobs.type_*, jobs.remote…)
	// — fall back to the prettified token for unseeded values.
	const jobTypeLabel = $derived(
		field('job_type') ? t(`jobs.type_${field('job_type')}`, prettify(field('job_type'))) : ''
	);
	const locationTypeLabel = $derived(
		field('location_type') ? t(`jobs.${field('location_type')}`, prettify(field('location_type'))) : ''
	);

	const deadlineStr = $derived(() => {
		const raw = field('application_deadline');
		if (!raw) return '';
		const d = new Date(raw);
		if (Number.isNaN(d.getTime())) return raw;
		return d.toLocaleDateString(i18n.locale, { month: 'short', day: 'numeric', year: 'numeric' });
	});

	const skills = $derived(
		field('skills_required')
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean)
	);

	const benefits = $derived(() => {
		const out: { icon: typeof Globe; label: string }[] = [];
		if (field('visa_sponsorship') === 'true')
			out.push({ icon: Globe, label: t('jobs.visa_sponsorship', 'Visa sponsorship') });
		if (field('relocation_assistance') === 'true')
			out.push({ icon: Truck, label: t('jobs.relocation', 'Relocation assistance') });
		return out;
	});

	// ---- Save (bookmark) — POST/DELETE /content/{id}/bookmark. ----
	let isBookmarked = $state(false);
	let bookmarkPending = $state(false);

	$effect(() => {
		if (!authed || !job) return;
		getViewerState(job.id).then((s) => (isBookmarked = s.bookmarked));
	});

	async function toggleBookmark() {
		if (!job || bookmarkPending) return;
		if (!(await requireAuthOrRedirect())) return;
		bookmarkPending = true;
		const was = isBookmarked;
		isBookmarked = !was;
		updateViewerState(job.id, { bookmarked: !was });
		const res = was
			? await api.delete(`/content/${job.id}/bookmark`)
			: await api.post(`/content/${job.id}/bookmark`, {});
		if (res.error) {
			isBookmarked = was;
			updateViewerState(job.id, { bookmarked: was });
		}
		bookmarkPending = false;
	}

	// ---- Share — Web Share API with clipboard fallback (same as forum). ----
	let shared = $state(false);

	async function shareJob() {
		if (!job) return;
		const url = page.url.href;
		try {
			if (navigator.share) {
				await navigator.share({ title: job.title, url });
			} else {
				await navigator.clipboard.writeText(url);
				shared = true;
				setTimeout(() => (shared = false), 2000);
			}
		} catch {
			// user dismissed the share sheet — no action needed
		}
	}

	// ---- Report — POST /content/{id}/report (Section 51). ----
	let reportOpen = $state(false);
	let reportReason = $state('');
	let reportNotes = $state('');
	let reportSubmitting = $state(false);
	let reportError = $state('');
	let reportSubmitted = $state(false);

	function openReport() {
		reportOpen = true;
		reportSubmitted = false;
		reportError = '';
	}

	function closeReport() {
		reportOpen = false;
		reportSubmitted = false;
		reportError = '';
		reportReason = '';
		reportNotes = '';
	}

	async function submitReport() {
		if (!(await requireAuthOrRedirect())) return;
		if (!job || !reportReason.trim() || reportSubmitting) return;
		reportSubmitting = true;
		reportError = '';
		const res = await api.post<{ id: string }>(`/content/${job.id}/report`, {
			reason: reportReason.trim(),
			notes: reportNotes.trim() || null,
			evidence_urls: []
		});
		if (res.error) {
			reportError = res.error.message;
		} else {
			reportSubmitted = true;
			reportReason = '';
			reportNotes = '';
		}
		reportSubmitting = false;
	}

	// ---- View tracking (Section 14 — POST /content/{id}/view). Fire-and-forget
	// on mount; the backend deduplicates per user/session per day. ----
	$effect(() => {
		if (!job?.id) return;
		api.post(`/content/${job.id}/view`, {});
	});

	// ---- Tabs — anchor scroll with an IntersectionObserver scroll-spy. ----
	const tabs = $derived(() => {
		const list: { id: string; label: string }[] = [
			{ id: 'overview', label: t('jobs.tab_overview', 'Overview') }
		];
		if (benefits().length > 0 || skills.length > 0)
			list.push({ id: 'details', label: t('jobs.tab_details', 'Requirements') });
		if (companyName)
			list.push({ id: 'company', label: t('jobs.tab_company', 'About Company') });
		if ((data.recommendedContent ?? []).length > 0)
			list.push({ id: 'similar', label: t('jobs.tab_similar', 'Similar Jobs') });
		return list;
	});
	let activeTab = $state('overview');

	$effect(() => {
		const observer = new IntersectionObserver(
			(entries) => {
				for (const e of entries) {
					if (e.isIntersecting) activeTab = e.target.id;
				}
			},
			{ rootMargin: '-15% 0px -75% 0px' }
		);
		for (const tab of tabs()) {
			const el = document.getElementById(tab.id);
			if (el) observer.observe(el);
		}
		return () => observer.disconnect();
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
					url: canonical,
					hiringOrganization: {
						'@type': 'Organization',
						name: field('company_name') || siteName()
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
	{#if fallbackDescription}
		<meta name="description" content={fallbackDescription} />
	{/if}
	<meta property="og:title" content={job?.title ?? 'Job'} />
	{#if fallbackDescription}<meta property="og:description" content={fallbackDescription} />{/if}
	<meta property="og:type" content="article" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-6xl px-4 py-4">
	{#if !job}
		<EmptyState icon={Briefcase} title={t('jobs.not_found', 'Job not found')} />
	{:else}
		<!-- Breadcrumb + actions -->
		<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
			<nav class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]" aria-label="Breadcrumb">
				<a href="/" class="hover:text-[var(--color-text)]">{t('nav.home', 'Home')}</a>
				<ChevronRight size={12} />
				<a href="/jobs" class="hover:text-[var(--color-text)]">{t('jobs.heading', 'Jobs')}</a>
				<ChevronRight size={12} />
				<span class="max-w-48 truncate text-[var(--color-text)]">{job.title}</span>
			</nav>
			<div class="flex items-center gap-2">
				{#if canEdit}
					<a
						href="/jobs/{job.slug}/edit"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<Pencil size={13} /> {t('common.edit', 'Edit')}
					</a>
				{/if}
				<button
					type="button"
					onclick={toggleBookmark}
					disabled={bookmarkPending}
					aria-pressed={isBookmarked}
					title={isBookmarked ? t('jobs.unsave', 'Remove from saved jobs') : t('jobs.save', 'Save this job')}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50 {isBookmarked ? 'border-[var(--color-primary)]/40 text-[var(--color-primary)]' : 'text-[var(--color-text)]'}"
				>
					<Bookmark size={13} fill={isBookmarked ? 'currentColor' : 'none'} />
					{isBookmarked ? t('jobs.saved', 'Saved') : t('common.save', 'Save')}
				</button>
				<button
					type="button"
					onclick={shareJob}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<Share2 size={13} />
					{shared ? t('jobs.shared', 'Copied!') : t('common.share', 'Share')}
				</button>
			</div>
		</div>

		<!-- Header card -->
		<header class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<div class="flex flex-wrap items-start gap-4">
				<div class="grid h-16 w-16 shrink-0 place-items-center overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-subtle)]">
					{#if companyLogo}
						<img src={companyLogo} alt={companyName} class="h-full w-full object-cover" />
					{:else}
						<Building2 size={26} class="text-[var(--color-muted)]" />
					{/if}
				</div>
				<div class="min-w-0 flex-1">
					<div class="flex flex-wrap items-center gap-2">
						{#if jobTypeLabel}
							<span class="rounded-full bg-[var(--color-success)]/10 px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-success)]">
								{jobTypeLabel}
							</span>
						{/if}
						{#if field('is_featured') === 'true'}
							<span class="rounded-full bg-[var(--color-primary)]/10 px-2.5 py-0.5 text-[11px] font-medium text-[var(--color-primary)]">
								{t('jobs.featured', 'Featured')}
							</span>
						{/if}
					</div>
					<h1 class="mt-1.5 text-xl font-semibold text-[var(--color-text)]">{job.title}</h1>
					{#if companyName}
						<div class="mt-0.5 flex items-center gap-1 text-sm">
							{#if orgHref}
								<a href={orgHref} class="font-medium text-[var(--color-primary)] hover:underline">{companyName}</a>
							{:else}
								<span class="font-medium text-[var(--color-text)]">{companyName}</span>
							{/if}
							{#if companyVerified}
								<BadgeCheck size={15} class="text-[var(--color-primary)]" />
							{/if}
						</div>
					{/if}
					<div class="mt-2 flex flex-wrap gap-x-4 gap-y-1.5 text-xs text-[var(--color-muted)]">
						{#if locationTypeLabel}
							<span class="flex items-center gap-1"><MapPin size={13} /> {locationTypeLabel}</span>
						{/if}
						{#if locationStr()}
							<span class="flex items-center gap-1"><Globe size={13} /> {locationStr()}</span>
						{/if}
						{#if field('experience_level')}
							<span class="flex items-center gap-1"><Clock size={13} /> {prettify(field('experience_level'))}</span>
						{/if}
						{#if postedAgo}
							<span class="flex items-center gap-1"><Calendar size={13} /> {t('jobs.posted_ago', 'Posted {time} ago').replace('{time}', postedAgo)}</span>
						{/if}
					</div>
					{#if job.source?.id}
						{@const source = job.source}
						<div class="mt-2 flex items-center gap-1 text-xs text-[var(--color-muted)]">
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
							{#if safeHttpUrl(source.source_url)}
								<a
									href={safeHttpUrl(source.source_url)}
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center gap-0.5 text-[var(--color-muted)] hover:text-[var(--color-primary)]"
									aria-label={t('jobs.original_source', 'View original source')}
								>
									<ExternalLink size={13} />
								</a>
							{/if}
						</div>
					{/if}
				</div>
				<div class="flex w-full flex-col gap-2 sm:w-auto sm:min-w-44">
					{#if myApplication}
						<div class="inline-flex items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-success)]/10 px-4 py-2.5 text-sm font-medium text-[var(--color-success)]">
							<CheckCircle2 size={16} /> {t('jobs.status_applied', 'Applied')}
						</div>
						<a href="/me/applications" class="text-center text-xs text-[var(--color-primary)] hover:underline">
							{t('jobs.my_applications_link', 'My applications')} →
						</a>
					{:else if externalApplyUrl}
						<a
							href={externalApplyUrl}
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							{t('jobs.apply_now', 'Apply Now')} <ArrowRight size={15} />
						</a>
					{:else}
						<a
							href="#apply"
							class="inline-flex items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							{t('jobs.apply_now', 'Apply Now')} <ArrowRight size={15} />
						</a>
					{/if}
				</div>
			</div>
		</header>

		<!-- Section tabs -->
		<nav class="sticky top-0 z-10 mt-4 flex gap-5 overflow-x-auto border-b border-[var(--color-border)] bg-[var(--color-bg)]" aria-label={t('jobs.sections', 'Job sections')}>
			{#each tabs() as tab (tab.id)}
				<a
					href="#{tab.id}"
					onclick={() => (activeTab = tab.id)}
					class="border-b-2 pb-2.5 pt-2 text-sm font-medium whitespace-nowrap transition-colors {activeTab === tab.id ? 'border-[var(--color-primary)] text-[var(--color-primary)]' : 'border-transparent text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				>
					{tab.label}
				</a>
			{/each}
		</nav>

		<div class="mt-5 grid gap-6 lg:grid-cols-[1fr_300px]">
			<!-- Main column -->
			<div class="min-w-0">
				<section id="overview" class="scroll-mt-24">
					<h2 class="mb-2 text-base font-semibold text-[var(--color-text)]">{t('jobs.overview', 'Job Overview')}</h2>
					{#if job.summary}
						<p class="mb-3 text-sm leading-relaxed text-[var(--color-muted)]">{job.summary}</p>
					{/if}
					{#if job.body}
						<div class="prose prose-sm max-w-none text-[var(--color-text)]">
							{@html sanitizeContentHtml(job.body)}
						</div>
					{/if}
				</section>

				{#if skills.length > 0 || benefits().length > 0}
					<section id="details" class="mt-6 scroll-mt-24">
						{#if skills.length > 0}
							<h2 class="mb-3 text-base font-semibold text-[var(--color-text)]">{t('jobs.requirements_skills', 'Requirements & Skills')}</h2>
							<ul class="space-y-2.5">
								{#each skills as skill (skill)}
									<li class="flex items-start gap-2.5 text-sm text-[var(--color-text)]">
										<CheckCircle2 size={17} class="mt-0.5 shrink-0 text-[var(--color-success)]" />
										<span>{skill}</span>
									</li>
								{/each}
							</ul>
						{/if}
						{#if benefits().length > 0}
							<h2 class="mb-3 mt-6 text-base font-semibold text-[var(--color-text)]">{t('jobs.benefits', 'Benefits')}</h2>
							<ul class="space-y-2.5">
								{#each benefits() as benefit (benefit.label)}
									{@const BIcon = benefit.icon}
									<li class="flex items-start gap-2.5 text-sm text-[var(--color-text)]">
										<BIcon size={17} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
										<span>{benefit.label}</span>
									</li>
								{/each}
							</ul>
						{/if}
					</section>
				{/if}

				<!-- Apply form -->
				<section id="apply" class="mt-6 scroll-mt-24 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
						{t('jobs.apply', 'Apply for this job')}
					</h2>

					{#if externalApplyUrl}
						<p class="mb-3 text-sm text-[var(--color-muted)]">
							{t('jobs.apply_external_hint', 'This job is listed externally. Apply on the employer site.')}
						</p>
						<a
							href={externalApplyUrl}
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
											accept=".pdf,.doc,.docx,.odt,.rtf,.txt,.md"
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
									{t('jobs.cv_formats', 'PDF, DOC, DOCX, ODT, RTF, TXT or MD')} ·
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
				</section>

				<!-- Same-type recommendations, ranked by embedding similarity. -->
				{#if (data.recommendedContent ?? []).length > 0}
					<section id="similar" class="mt-6 scroll-mt-24">
						<RecommendedContentCard items={data.recommendedContent} contentType="job_listing" />
					</section>
				{/if}
			</div>

			<!-- Sidebar -->
			<aside class="min-w-0 space-y-4">
				<!-- Job summary -->
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">{t('jobs.summary', 'Job Summary')}</h2>
					<dl class="space-y-3">
						{#if salaryStr()}
							<div class="flex items-center gap-2.5">
								<Wallet size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.salary', 'Salary')}</dt>
								<dd class="text-xs font-medium text-[var(--color-success)]">{salaryStr()}</dd>
							</div>
						{/if}
						{#if field('job_role')}
							<div class="flex items-center gap-2.5">
								<UserRound size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.role', 'Role')}</dt>
								<dd class="text-xs font-medium text-[var(--color-text)]">{field('job_role')}</dd>
							</div>
						{/if}
						{#if jobFunctionLabel}
							<div class="flex items-center gap-2.5">
								<Layers size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.function', 'Function')}</dt>
								<dd class="text-xs font-medium text-[var(--color-text)]">{jobFunctionLabel}</dd>
							</div>
						{/if}
						{#if jobTypeLabel}
							<div class="flex items-center gap-2.5">
								<Briefcase size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.job_type', 'Job type')}</dt>
								<dd class="text-xs font-medium text-[var(--color-text)]">{jobTypeLabel}</dd>
							</div>
						{/if}
						{#if field('experience_level')}
							<div class="flex items-center gap-2.5">
								<GraduationCap size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.experience', 'Experience')}</dt>
								<dd class="text-xs font-medium text-[var(--color-text)]">{prettify(field('experience_level'))}</dd>
							</div>
						{/if}
						{#if locationTypeLabel || locationStr()}
							<div class="flex items-center gap-2.5">
								<MapPin size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.location', 'Location')}</dt>
								<dd class="text-xs font-medium text-[var(--color-text)]">
									{locationTypeLabel}{locationStr() ? ` (${locationStr()})` : ''}
								</dd>
							</div>
						{/if}
						{#if deadlineStr()}
							<div class="flex items-center gap-2.5">
								<Calendar size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.deadline', 'Deadline')}</dt>
								<dd class="text-xs font-medium text-[var(--color-text)]">{deadlineStr()}</dd>
							</div>
						{/if}
						{#if field('visa_sponsorship') === 'true'}
							<div class="flex items-center gap-2.5">
								<Globe size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.visa_sponsorship', 'Visa sponsorship')}</dt>
								<dd class="text-xs font-medium text-[var(--color-success)]">{t('jobs.included', 'Included')}</dd>
							</div>
						{/if}
						{#if field('relocation_assistance') === 'true'}
							<div class="flex items-center gap-2.5">
								<Truck size={15} class="shrink-0 text-[var(--color-muted)]" />
								<dt class="w-24 shrink-0 text-xs text-[var(--color-muted)]">{t('jobs.relocation', 'Relocation assistance')}</dt>
								<dd class="text-xs font-medium text-[var(--color-success)]">{t('jobs.included', 'Included')}</dd>
							</div>
						{/if}
					</dl>
					<button
						type="button"
						onclick={openReport}
						class="mt-4 inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-xs font-medium text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-danger)]"
					>
						<Flag size={13} /> {t('jobs.report_job', 'Report this job')}
					</button>
				</div>

				<!-- About company -->
				{#if companyName}
					<div id="company" class="scroll-mt-24 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2.5">
							<div class="grid h-9 w-9 shrink-0 place-items-center overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-subtle)]">
								{#if companyLogo}
									<img src={companyLogo} alt={companyName} class="h-full w-full object-cover" />
								{:else}
									<Building2 size={16} class="text-[var(--color-muted)]" />
								{/if}
							</div>
							<div class="min-w-0">
								<div class="flex items-center gap-1 text-sm font-semibold text-[var(--color-text)]">
									<span class="truncate">{t('jobs.about_company', 'About {company}').replace('{company}', companyName)}</span>
								</div>
								<div class="flex items-center gap-1 text-xs text-[var(--color-muted)]">
									<span class="truncate">{companyName}</span>
									{#if companyVerified}
										<BadgeCheck size={13} class="shrink-0 text-[var(--color-primary)]" />
									{/if}
								</div>
							</div>
						</div>
						{#if data.org?.description || data.org?.tagline}
							<p class="mt-3 text-xs leading-relaxed text-[var(--color-muted)]">{data.org.description ?? data.org.tagline}</p>
						{/if}
						{#if orgHref}
							<a href={orgHref} class="mt-3 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:underline">
								{t('jobs.visit_company', 'Visit company page')} <ChevronRight size={13} />
							</a>
						{:else if safeHttpUrl(field('company_website'))}
							<a href={safeHttpUrl(field('company_website'))} target="_blank" rel="noopener noreferrer" class="mt-3 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:underline">
								{t('jobs.visit_company', 'Visit company page')} <ExternalLink size={12} />
							</a>
						{/if}
					</div>
				{/if}

				<!-- More jobs from this org -->
				{#if data.moreJobs.length > 0}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
							{t('jobs.more_jobs', 'More Jobs from {company}').replace('{company}', companyName)}
						</h2>
						<ul class="space-y-3">
							{#each data.moreJobs as item (item.id)}
								<li>
									<a href="/jobs/{item.slug}" class="block text-sm font-medium text-[var(--color-primary)] hover:underline">
										{item.title}
									</a>
									{#if item.published_at}
										<span class="text-xs text-[var(--color-muted)]">{relativeTime(item.published_at, new Date(), i18n.locale)}</span>
									{/if}
								</li>
							{/each}
						</ul>
						{#if orgHref}
							<a href="{orgHref}?tab=jobs" class="mt-3 inline-flex items-center gap-1 text-xs font-medium text-[var(--color-primary)] hover:underline">
								{t('jobs.view_all_jobs', 'View all jobs')} <ChevronRight size={13} />
							</a>
						{/if}
					</div>
				{/if}
			</aside>
		</div>
	{/if}
</div>

<!-- Report job dialog — POST /content/{content_id}/report (Section 51) -->
{#if reportOpen}
	<Modal open={reportOpen} onOpenChange={(v) => { if (!v) closeReport(); }} icon={Flag} title={t('jobs.report_title', 'Report this job')} overlayClass="backdrop-blur-sm" size="md">
				{#if reportSubmitted}
					<p class="rounded-lg bg-green-50 px-4 py-3 text-sm text-green-700 dark:bg-green-950/30 dark:text-green-400">
						{t('jobs.report_submitted', 'Your report has been submitted. Our team will review it.')}
					</p>
					<button
						type="button"
						onclick={closeReport}
						class="mt-4 rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
					>
						{t('common.close', 'Close')}
					</button>
				{:else}
					<form onsubmit={(e) => { e.preventDefault(); void submitReport(); }} class="space-y-3">
						{#if reportError}
							<p class="rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-950/30 dark:text-red-400">{reportError}</p>
						{/if}
						<div>
							<label for="report-reason" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('jobs.report_reason', 'Reason')}
							</label>
							<AutocompleteSelect
								id="report-reason"
								bind:value={reportReason}
								options={[
									{ value: '', label: t('jobs.report_reason_select', 'Select a reason…') },
									{ value: 'spam', label: t('forum.report_spam', 'Spam') },
									{ value: 'harassment', label: t('forum.report_harassment', 'Harassment') },
									{ value: 'hate_speech', label: t('forum.report_hate_speech', 'Hate speech') },
									{ value: 'misinformation', label: t('forum.report_misinformation', 'Misinformation') },
									{ value: 'illegal_content', label: t('forum.report_illegal', 'Illegal content') },
									{ value: 'self_harm', label: t('forum.report_self_harm', 'Self harm') },
									{ value: 'copyright', label: t('forum.report_copyright', 'Copyright') },
									{ value: 'other', label: t('forum.report_other', 'Other') }
								]}
							/>
						</div>
						<div>
							<label for="report-notes" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
								{t('jobs.report_notes', 'Additional notes (optional)')}
							</label>
							<textarea
								id="report-notes"
								bind:value={reportNotes}
								rows={3}
								class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
							></textarea>
						</div>
						<div class="flex items-center gap-2">
							<button
								type="submit"
								disabled={reportSubmitting || !reportReason.trim()}
								class="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
							>
								{#if reportSubmitting}<Loader2 size={14} class="animate-spin" />{:else}
								{t('jobs.report_submit', 'Submit report')}{/if}
							</button>
							<button
								type="button"
								onclick={closeReport}
								class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('common.cancel', 'Cancel')}
							</button>
						</div>
					</form>
				{/if}
	</Modal>
{/if}
