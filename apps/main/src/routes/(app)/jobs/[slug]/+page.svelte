<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Briefcase, MapPin, Clock, CheckCircle2, AlertCircle } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { enhance } from '$app/forms';
	import type { JobDetail } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const job = $state<JobDetail | null>(data.job);
	const authed = $state(data.authed);

	let coverLetter = $state('');
	let resumeUrl = $state('');
	let applyResult = $state<{ ok?: boolean; error?: string; status?: string } | null>(null);
	let applying = $state(false);

	const title = $derived(job ? `${job.title} — JapaConnect` : 'Job — JapaConnect');

	function field(key: string): string {
		return job?.field_values?.[key] ?? '';
	}
</script>

<svelte:head>
	<title>{title}</title>
	{#if job?.summary}
		<meta name="description" content={job.summary} />
	{/if}
	<meta property="og:title" content={job?.title ?? 'Job'} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
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
			<div class="mt-2 flex flex-wrap gap-3 text-xs text-[var(--color-muted)]">
				{#if field('company_name')}
					<span class="flex items-center gap-1"><Briefcase size={12} /> {field('company_name')}</span>
				{/if}
				{#if field('location')}
					<span class="flex items-center gap-1"><MapPin size={12} /> {field('location')}</span>
				{/if}
				{#if field('job_type')}
					<span class="flex items-center gap-1"><Clock size={12} /> {field('job_type')}</span>
				{/if}
				{#if field('salary_range')}
					<span>{field('salary_range')}</span>
				{/if}
			</div>
		</div>

		<div class="p-4">
			{#if job.summary}
				<p class="mb-3 text-sm text-[var(--color-muted)]">{job.summary}</p>
			{/if}
			{#if job.body}
				<div class="prose prose-sm max-w-none text-[var(--color-text)]">
					{@html job.body}
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
					<div class="flex items-center gap-2 text-sm text-green-600">
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
							<div class="flex items-center gap-2 text-sm text-red-500">
								<AlertCircle size={16} /> {applyResult.error}
							</div>
						{/if}
						<div>
							<label for="resume_url" class="block text-xs font-medium text-[var(--color-muted)]">
								{t('jobs.resume_url', 'Resume URL')}
							</label>
							<input
								id="resume_url"
								name="resume_url"
								type="url"
								bind:value={resumeUrl}
								placeholder="https://…"
								class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
							/>
						</div>
						<div>
							<label for="cover_letter" class="block text-xs font-medium text-[var(--color-muted)]">
								{t('jobs.cover_letter', 'Cover letter')}
							</label>
							<textarea
								id="cover_letter"
								name="cover_letter"
								bind:value={coverLetter}
								rows="4"
								class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
							></textarea>
						</div>
						<button
							type="submit"
							disabled={applying}
							class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
						>
							{applying ? t('common.submitting', 'Submitting…') : t('jobs.submit_application', 'Submit application')}
						</button>
					</form>
				{/if}
			</div>
		</div>
	{/if}
</div>
