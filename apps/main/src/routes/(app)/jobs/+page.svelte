<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Briefcase, Lock, Search, MapPin, Clock, ArrowRight } from 'lucide-svelte';
	import { EmptyState, LoadingSkeleton, PaginationControls } from '$lib/components/ui';
	import { goto } from '$app/navigation';
	import type { JobListing } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('jobs.title', 'Jobs — JapaConnect'));
	const description = $derived(
		t('jobs.description', 'Find jobs relevant to the Nigerian diaspora community.')
	);

	const jobs = $state<JobListing[]>(data.jobs);
	const disabled = $state(data.disabled);
	const authed = $state(data.authed);

	let q = $state(page.url.searchParams.get('q') ?? '');
	let country = $state(page.url.searchParams.get('country') ?? '');
	let jobType = $state(page.url.searchParams.get('job_type') ?? '');
	let remote = $state(page.url.searchParams.get('remote') ?? '');

	function applyFilters() {
		const params = new URLSearchParams();
		if (q) params.set('q', q);
		if (country) params.set('country', country);
		if (jobType) params.set('job_type', jobType);
		if (remote) params.set('remote', remote);
		goto(`/jobs?${params.toString()}`, { keepFocus: true });
	}

	function field(job: JobListing, key: string): string {
		return job.field_values?.[key] ?? '';
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Briefcase size={20} /> {t('jobs.heading', 'Jobs')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	{#if disabled}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('tools.disabled_title', 'This tool is currently unavailable')}
				body={t('tools.disabled_body', 'Check back soon or contact support.')}
			/>
		</div>
	{:else}
		<!-- Filter panel -->
		<div class="border-b border-[var(--color-border)] px-4 py-3">
			<form onsubmit={(e) => { e.preventDefault(); applyFilters(); }} class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
				<div class="relative">
					<Search size={14} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-muted)]" />
					<input
						type="text"
						placeholder={t('jobs.search', 'Search jobs…')}
						bind:value={q}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] py-2 pl-8 pr-3 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
					/>
				</div>
				<input
					type="text"
					placeholder={t('jobs.country', 'Country')}
					bind:value={country}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
				<input
					type="text"
					placeholder={t('jobs.job_type', 'Job type')}
					bind:value={jobType}
					class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
				/>
				<button
					type="submit"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90"
				>
					{t('common.filter', 'Filter')}
				</button>
			</form>
		</div>

		<!-- Results -->
		<div class="p-4">
			{#if jobs.length === 0}
				<EmptyState
					icon={Briefcase}
					title={t('jobs.empty', 'No jobs found')}
					body={t('jobs.empty_body', 'Try adjusting your filters.')}
				/>
			{:else}
				<div class="space-y-3">
					{#each jobs as job (job.id)}
						<a
							href="/jobs/{job.slug}"
							class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0 flex-1">
									<h3 class="text-sm font-medium text-[var(--color-text)]">{job.title}</h3>
									{#if job.summary}
										<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{job.summary}</p>
									{/if}
									<div class="mt-2 flex flex-wrap gap-2 text-xs text-[var(--color-muted)]">
										{#if field(job, 'company_name')}
											<span class="flex items-center gap-1"><Briefcase size={12} /> {field(job, 'company_name')}</span>
										{/if}
										{#if field(job, 'location')}
											<span class="flex items-center gap-1"><MapPin size={12} /> {field(job, 'location')}</span>
										{/if}
										{#if field(job, 'job_type')}
											<span class="flex items-center gap-1"><Clock size={12} /> {field(job, 'job_type')}</span>
										{/if}
										{#if field(job, 'is_remote') === 'true'}
											<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5">{t('jobs.remote', 'Remote')}</span>
										{/if}
									</div>
								</div>
								<ArrowRight size={16} class="shrink-0 text-[var(--color-muted)]" />
							</div>
						</a>
					{/each}
				</div>

				{#if data.total > data.pageSize}
					<div class="mt-4">
						<PaginationControls
							page={data.currentPage}
							totalPages={Math.ceil(data.total / data.pageSize)}
							onPageChange={(p) => goto(`/jobs?page=${p}`)}
						/>
					</div>
				{/if}
			{/if}
		</div>
	{/if}
</div>
