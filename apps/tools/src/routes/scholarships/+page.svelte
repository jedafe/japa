<script lang="ts">
	import { api } from '$lib/api/client';
	import { mainAppUrl } from '$lib/seo';
	import { buildSeoMeta } from '$lib/seo';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import LoadingSkeleton from '$lib/components/ui/LoadingSkeleton.svelte';
	import type { Scholarship, SavedScholarship, ScholarshipStatus } from '$lib/types';

	/**
	 * Scholarship Finder tool — §73, ADR-073.
	 * Filter panel → results list → save-to-tracker with application status.
	 * Anonymous users can browse; saving requires auth.
	 */
	let { data } = $props();

	let scholarships = $state<Scholarship[]>(data.scholarships);
	let saved = $state<SavedScholarship[]>(data.saved);
	let user = $state(data.user);

	// Filter state
	let filterCountry = $state('');
	let filterLevel = $state('');
	let filterMinAmount = $state('');
	let filterMaxAmount = $state('');
	let filterRenewable = $state('');
	let filterDeadlineFrom = $state('');
	let filterDeadlineTo = $state('');
	let filterQuery = $state('');
	let loading = $state(false);
	let showRegisterPrompt = $state(false);

	const seo = $derived(
		buildSeoMeta({
			title: 'Scholarship Finder — JapaConnect Tools',
			description:
				'Filter scholarships by country, level of study, award amount, and deadline. Save scholarships to your tracker and get deadline reminders.',
			path: '/scholarships',
			origin: page.url.origin,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'EducationalOccupationalProgram',
				name: 'JapaConnect Scholarship Finder',
				description: 'Find scholarships for studying abroad.'
			}
		})
	);

	async function applyFilters() {
		loading = true;
		const params = new URLSearchParams();
		if (filterCountry) params.set('country', filterCountry);
		if (filterLevel) params.set('level', filterLevel);
		if (filterMinAmount) params.set('min_amount', filterMinAmount);
		if (filterMaxAmount) params.set('max_amount', filterMaxAmount);
		if (filterRenewable) params.set('renewable', filterRenewable);
		if (filterDeadlineFrom) params.set('deadline_from', filterDeadlineFrom);
		if (filterDeadlineTo) params.set('deadline_to', filterDeadlineTo);
		if (filterQuery) params.set('q', filterQuery);
		const qs = params.toString();
		const res = await api.get<Scholarship[]>(`/scholarships${qs ? `?${qs}` : ''}`);
		loading = false;
		if (res.error) return;
		scholarships = res.data ?? [];
		// Update URL for shareability
		const url = new URL(window.location.href);
		url.search = qs;
		window.history.replaceState({}, '', url);
	}

	function isSaved(guideId: string): SavedScholarship | undefined {
		return saved.find((s) => s.scholarship.id === guideId);
	}

	async function toggleSave(scholarship: Scholarship) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		const existing = isSaved(scholarship.id);
		if (existing) {
			const res = await api.delete(`/scholarships/${scholarship.id}/save`);
			if (!res.error) {
				saved = saved.filter((s) => s.scholarship.id !== scholarship.id);
			}
		} else {
			const res = await api.post<{ id: string }>(`/scholarships/${scholarship.id}/save`);
			if (!res.error) {
				saved = [
					...saved,
					{
						id: res.data?.id ?? '',
						scholarship,
						status: 'saved' as ScholarshipStatus,
						notes: null,
						remind_before_days: 14,
						last_reminded_at: null,
						created_at: new Date().toISOString(),
						updated_at: new Date().toISOString()
					}
				];
			}
		}
	}

	async function updateStatus(guideId: string, status: ScholarshipStatus) {
		const res = await api.patch(`/scholarships/${guideId}/save`, { status });
		if (!res.error) {
			saved = saved.map((s) =>
				s.scholarship.id === guideId ? { ...s, status } : s
			);
		}
	}

	function formatAmount(amount: number | null | undefined): string {
		if (amount === null || amount === undefined) return '—';
		return new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: 'USD',
			maximumFractionDigits: 0
		}).format(amount);
	}

	function formatDate(date: string | null | undefined): string {
		if (!date) return '—';
		return new Date(date).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric'
		});
	}

	function daysUntilDeadline(deadline: string | null | undefined): number | null {
		if (!deadline) return null;
		const diff = new Date(deadline).getTime() - Date.now();
		return Math.ceil(diff / (1000 * 60 * 60 * 24));
	}

	const statusOptions: { value: ScholarshipStatus; label: string }[] = [
		{ value: 'saved', label: 'Saved' },
		{ value: 'researching', label: 'Researching' },
		{ value: 'applied', label: 'Applied' },
		{ value: 'accepted', label: 'Accepted' },
		{ value: 'rejected', label: 'Rejected' },
		{ value: 'withdrawn', label: 'Withdrawn' }
	];
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	{#if seo.jsonLdString}
		<script type="application/ld+json">{seo.jsonLdString}</script>
	{/if}
</svelte:head>

<div class="mx-auto max-w-5xl px-4 py-8">
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">Scholarship Finder</h1>
		<p class="mt-2 text-text-subtle">
			Find funding for your studies abroad. Filter, save, and track your applications.
		</p>
	</div>

	<div class="grid grid-cols-1 lg:grid-cols-[280px_1fr] gap-6">
		<!-- Filter panel -->
		<aside class="space-y-4">
			<div class="p-4 bg-surface border border-border rounded-lg space-y-4">
				<h2 class="text-sm font-semibold text-text">Filters</h2>

				<div>
					<label class="block text-xs font-medium text-muted mb-1">Search</label>
					<input
						type="search"
						bind:value={filterQuery}
						onkeydown={(e) => e.key === 'Enter' && applyFilters()}
						placeholder="Name, institution..."
						class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
					/>
				</div>

				<div>
					<label class="block text-xs font-medium text-muted mb-1">Country</label>
					<input
						type="text"
						bind:value={filterCountry}
						placeholder="e.g. United Kingdom"
						class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
					/>
				</div>

				<div>
					<label class="block text-xs font-medium text-muted mb-1">Level of study</label>
					<select
						bind:value={filterLevel}
						class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
					>
						<option value="">Any level</option>
						<option value="undergraduate">Undergraduate</option>
						<option value="masters">Masters</option>
						<option value="phd">PhD</option>
						<option value="postdoc">Postdoc</option>
					</select>
				</div>

				<div class="grid grid-cols-2 gap-2">
					<div>
						<label class="block text-xs font-medium text-muted mb-1">Min $</label>
						<input
							type="number"
							bind:value={filterMinAmount}
							placeholder="0"
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
					</div>
					<div>
						<label class="block text-xs font-medium text-muted mb-1">Max $</label>
						<input
							type="number"
							bind:value={filterMaxAmount}
							placeholder="any"
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
					</div>
				</div>

				<div>
					<label class="block text-xs font-medium text-muted mb-1">Renewable</label>
					<select
						bind:value={filterRenewable}
						class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
					>
						<option value="">Any</option>
						<option value="true">Yes</option>
						<option value="false">No</option>
					</select>
				</div>

				<div class="grid grid-cols-2 gap-2">
					<div>
						<label class="block text-xs font-medium text-muted mb-1">Deadline from</label>
						<input
							type="date"
							bind:value={filterDeadlineFrom}
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
					</div>
					<div>
						<label class="block text-xs font-medium text-muted mb-1">Deadline to</label>
						<input
							type="date"
							bind:value={filterDeadlineTo}
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						/>
					</div>
				</div>

				<button
					onclick={applyFilters}
					disabled={loading}
					class="w-full px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors disabled:opacity-50"
				>
					{#if loading}
						Searching...
					{:else}
						Apply filters
					{/if}
				</button>
			</div>
		</aside>

		<!-- Results list -->
		<div>
			{#if loading}
				<div class="space-y-3">
					<LoadingSkeleton height="120px" />
					<LoadingSkeleton height="120px" />
					<LoadingSkeleton height="120px" />
				</div>
			{:else if scholarships.length === 0}
				<EmptyState
					icon="search-x"
					title="No scholarships found"
					description="Try adjusting your filters or clearing them to see all scholarships."
				/>
			{:else}
				<div class="space-y-3">
					{#each scholarships as s (s.id)}
						{@const savedItem = isSaved(s.id)}
						{@const days = daysUntilDeadline(s.application_deadline)}
						<div class="p-5 bg-surface border border-border rounded-lg">
							<div class="flex items-start justify-between gap-4">
								<div class="flex-1 min-w-0">
									<h3 class="text-base font-semibold text-text">{s.title}</h3>
									{#if s.institution}
										<p class="text-sm text-text-subtle mt-0.5">{s.institution}</p>
									{/if}
									<div class="flex flex-wrap gap-2 mt-2">
										{#if s.country}
											<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">
												{s.country}
											</span>
										{/if}
										{#if s.level_of_study}
											<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded capitalize">
												{s.level_of_study}
											</span>
										{/if}
										{#if s.is_renewable}
											<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">
												Renewable
											</span>
										{/if}
									</div>
									{#if s.description}
										<p class="text-sm text-text-subtle mt-2 line-clamp-2">{s.description}</p>
									{/if}
								</div>
								<div class="flex-shrink-0 text-right">
									{#if s.award_amount_usd !== null && s.award_amount_usd !== undefined}
										<div class="text-lg font-bold text-[var(--tool-accent)]">
											{formatAmount(s.award_amount_usd)}
										</div>
										<div class="text-xs text-muted">award</div>
									{/if}
								</div>
							</div>

							<div class="flex items-center justify-between mt-4 pt-4 border-t border-border">
								<div class="flex items-center gap-3">
									{#if s.application_deadline}
										<div class="flex items-center gap-1.5 text-sm">
											<Icon name="calendar" size={14} class="text-muted" />
											<span class="text-text-subtle">{formatDate(s.application_deadline)}</span>
											{#if days !== null && days >= 0 && days <= 30}
												<span class="text-xs px-1.5 py-0.5 bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400 rounded font-medium">
													{days} days left
												</span>
											{:else if days !== null && days < 0}
												<span class="text-xs px-1.5 py-0.5 bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400 rounded font-medium">
													Passed
												</span>
											{/if}
										</div>
									{/if}
								</div>

								<div class="flex items-center gap-2">
									{#if savedItem}
										<select
											value={savedItem.status}
											onchange={(e) => updateStatus(s.id, e.currentTarget.value as ScholarshipStatus)}
											class="px-2 py-1 text-xs bg-surface border border-border rounded focus:border-primary focus:outline-none"
										>
											{#each statusOptions as opt (opt.value)}
												<option value={opt.value}>{opt.label}</option>
											{/each}
										</select>
									{/if}
									<button
										onclick={() => toggleSave(s)}
										class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium rounded-lg transition-colors {savedItem ? 'bg-[var(--tool-accent)]/10 text-[var(--tool-accent)]' : 'border border-border text-text hover:border-[var(--tool-accent)]'}"
									>
										<Icon name={savedItem ? 'bookmark-check' : 'bookmark'} size={16} />
										{savedItem ? 'Saved' : 'Save'}
									</button>
									{#if s.application_url}
										<a
											href={s.application_url}
											target="_blank"
											rel="noopener noreferrer"
											class="inline-flex items-center gap-1 px-3 py-1.5 text-sm font-medium text-primary hover:text-primary-hover"
										>
											Apply
											<Icon name="external-link" size={14} />
										</a>
									{/if}
								</div>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>

	{#if showRegisterPrompt}
		<div
			class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
			onclick={() => (showRegisterPrompt = false)}
			onkeydown={(e) => e.key === 'Escape' && (showRegisterPrompt = false)}
			role="button"
			tabindex="0"
		>
			<div
				class="bg-surface rounded-[var(--radius-card)] p-6 max-w-sm w-full text-center"
				onclick={(e) => e.stopPropagation()}
				onkeydown={(e) => e.stopPropagation()}
				role="button"
				tabindex="0"
			>
				<div class="flex justify-center mb-4">
					<div class="w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center">
						<Icon name="user-plus" size={24} class="text-primary" />
					</div>
				</div>
				<h3 class="text-lg font-semibold text-text">Save scholarships</h3>
				<p class="mt-2 text-sm text-text-subtle">
					Create a free account to save scholarships, track your application status, and get
					deadline reminders.
				</p>
				<div class="mt-5 flex flex-col gap-2">
					<a
						href={mainAppUrl('/signup')}
						class="px-4 py-2.5 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
					>
						Create free account
					</a>
					<a
						href={mainAppUrl('/login')}
						class="px-4 py-2.5 text-sm font-medium text-text hover:text-primary transition-colors"
					>
						Sign in
					</a>
				</div>
			</div>
		</div>
	{/if}
</div>
