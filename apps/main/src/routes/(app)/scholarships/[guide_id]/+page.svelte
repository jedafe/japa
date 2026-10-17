<script lang="ts">
	import { t } from '$lib/i18n';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import {
		GraduationCap,
		Bookmark,
		BookmarkCheck,
		CalendarClock,
		Building2,
		Globe,
		DollarSign,
		RefreshCw,
		ShieldCheck,
		ShieldAlert,
		ExternalLink,
		ArrowLeft,
		ClipboardList,
		BookOpen
	} from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState, AutocompleteSelect } from '$lib/components/ui';
	import type { Scholarship, SavedScholarship } from '$lib/types';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';

	/**
	 * Scholarship detail page — Section 73, ADR-073.
	 * Shows full scholarship info and lets authenticated users save/unsave,
	 * update application status, edit notes, and configure reminder timing.
	 *
	 * Ref: japaconnect-engineering/sections/73-scholarship-finder.md
	 */
	let { data }: { data: PageData } = $props();

	let scholarship = $state<Scholarship>(untrack(() => data.scholarship!));
	let savedItem = $state<SavedScholarship | null>(untrack(() => data.savedItem));
	let authed = $state(untrack(() => data.authed));

	// Resync when the server load re-runs (e.g. navigating between scholarship
	// detail pages in-place). Local optimistic mutations don't touch `data`,
	// so they are not clobbered.
	$effect(() => {
		scholarship = data.scholarship!;
		savedItem = data.savedItem;
		authed = data.authed;
	});

	let showRegisterPrompt = $state(false);
	let editingNotes = $state(false);
	let notesDraft = $state('');
	let editingReminder = $state(false);
	let reminderDraft = $state(untrack(() => data.defaultReminderDays));
	let updating = $state(false);

	const statusOptions = $derived(
		(data.statuses ?? []).map((o) => ({
			value: o.value,
			label: t(`scholarships.status_${o.value}`, o.label)
		}))
	);

	const title = $derived(`${scholarship.title} — ${t('tools.scholarships', 'Scholarship Finder — {site}')}`);
	const description = $derived(scholarship.summary ?? t('tools.scholarships_desc', 'Find and track scholarships by country and level.'));

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'EducationalOccupationalProgram',
			name: scholarship.title,
			description: scholarship.summary ?? '',
			provider: scholarship.institution ?? undefined,
			url: page.url.href
		})
	);

	async function toggleSave() {
		if (!authed) {
			showRegisterPrompt = true;
			return;
		}
		if (savedItem) {
			const res = await api.delete(`/scholarships/${scholarship.guide_id}/save`);
			if (!res.error) savedItem = null;
		} else {
			const res = await api.post<{ id: string }>(`/scholarships/${scholarship.guide_id}/save`);
			if (!res.error && res.data) {
				savedItem = {
					id: res.data.id,
					scholarship,
					status: 'saved',
					notes: null,
					remind_before_days: data.defaultReminderDays,
					last_reminded_at: null,
					created_at: new Date().toISOString(),
					updated_at: new Date().toISOString()
				} as SavedScholarship;
			}
		}
	}

	async function updateStatus(status: string) {
		if (!authed || !savedItem) return;
		updating = true;
		const res = await api.patch(`/scholarships/${scholarship.guide_id}/save`, { status });
		updating = false;
		if (!res.error && savedItem) {
			savedItem = { ...savedItem, status };
		}
	}

	function startEditNotes() {
		notesDraft = savedItem?.notes ?? '';
		editingNotes = true;
	}

	async function saveNotes() {
		if (!authed || !savedItem) return;
		editingNotes = false;
		const res = await api.patch(`/scholarships/${scholarship.guide_id}/save`, { notes: notesDraft });
		if (!res.error && savedItem) {
			savedItem = { ...savedItem, notes: notesDraft };
		}
	}

	function startEditReminder() {
		reminderDraft = savedItem?.remind_before_days ?? data.defaultReminderDays;
		editingReminder = true;
	}

	async function saveReminder() {
		if (!authed || !savedItem) return;
		editingReminder = false;
		const res = await api.patch(`/scholarships/${scholarship.guide_id}/save`, {
			remind_before_days: reminderDraft
		});
		if (!res.error && savedItem) {
			savedItem = { ...savedItem, remind_before_days: reminderDraft };
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
			month: 'long',
			day: 'numeric'
		});
	}

	// Provenance — the source's trust tier (1=official … 4=community) powers
	// the verified pill in the header band and the trust bar below, matching
	// the radar detail page's signal readout.
	const trustTier = $derived(scholarship.source?.trust_level ?? 0);
	const verified = $derived(trustTier > 0 && trustTier <= 2);
	const trustPct = $derived(
		trustTier > 0 ? Math.max(0, Math.min(100, (4 - trustTier) * 25 + 25)) : 0
	);

	// Deadline callout — the most actionable datum for a scholarship.
	const deadlineDate = $derived(
		scholarship.application_deadline ? new Date(scholarship.application_deadline) : null
	);
	const deadlineValid = $derived(
		deadlineDate !== null && !isNaN(deadlineDate.getTime())
	);
	const daysToDeadline = $derived(
		deadlineValid ? Math.ceil((deadlineDate!.getTime() - Date.now()) / 86_400_000) : null
	);
	const deadlinePassed = $derived(daysToDeadline !== null && daysToDeadline < 0);

	// Bare domain of the source URL for the facts grid.
	const sourceDomain = $derived(
		scholarship.source?.source_url ? extractDomain(scholarship.source.source_url) : ''
	);

	function extractDomain(url: string): string {
		try {
			return new URL(url).hostname.replace(/^www\./, '');
		} catch {
			return url;
		}
	}

	// field_values keys already covered by flat fields/sections — the rest
	// render verbatim under "Additional details" so nothing the API returns
	// is hidden (e.g. eligible_countries, field_of_study, dimension).
	const PROJECTED_KEYS = new Set([
		'provider_institution',
		'countries_eligible',
		'level_of_study',
		'application_deadline',
		'application_link',
		'eligibility_criteria',
		'how_to_apply',
		'contact_email',
		'description',
		'award_amount_usd',
		'is_renewable',
		'field_of_study',
		'fields_of_study'
	]);

	function humanize(key: string): string {
		return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	const extraFields = $derived(
		Object.entries(scholarship.field_values ?? {})
			.filter(([k, v]) => v !== '' && !PROJECTED_KEYS.has(k))
			.map(([k, v]) => ({ label: humanize(k), value: v }))
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
	{#if jsonLd}
		{@html safeJsonLdScript(jsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto max-w-3xl px-4 py-6">
	{#if !scholarship}
		<EmptyState
			icon={GraduationCap}
			title={t('scholarships.not_found', 'Scholarship not found')}
			body={t('scholarships.not_found_desc', 'This scholarship may have been removed or is no longer available.')}
		/>
	{:else}
		<!-- Back link + save button -->
		<div class="flex items-center justify-between">
			<a
				href="/scholarships"
				class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
			>
				<ArrowLeft size={15} />
				{t('scholarships.back_to_list', 'Back to scholarships')}
			</a>
			<button
				type="button"
				onclick={toggleSave}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				{#if savedItem}
					<BookmarkCheck size={14} class="text-[var(--color-primary)]" />
					{t('scholarships.saved', 'Saved')}
				{:else}
					<Bookmark size={14} />
					{t('scholarships.save', 'Save')}
				{/if}
			</button>
		</div>

		<article class="mt-5 overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
			<!-- Header band: type pill + country + verified status -->
			<div class="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-5 py-3">
				<span class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-primary)]/10 px-2.5 py-1 text-xs font-medium capitalize text-[var(--color-primary)]">
					<GraduationCap size={12} />
					{t('scholarships.type_label', 'Scholarship')}
				</span>
				{#if scholarship.country}
					<span class="inline-flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
						<Globe size={12} />
						{scholarship.country}
					</span>
				{/if}
				{#if trustTier > 0}
					<span class="ml-auto inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium {verified ? 'bg-[var(--color-success)]/10 text-[var(--color-success)]' : 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'}">
						{#if verified}
							<ShieldCheck size={12} />
							{t('scholarships.verified', 'Verified')}
						{:else}
							<ShieldAlert size={12} />
							{t('scholarships.needs_review', 'Needs review')}
						{/if}
					</span>
				{/if}
			</div>

			<div class="px-5 py-5">
				<!-- Title -->
				<h1 class="text-xl font-medium leading-snug text-[var(--color-text)] sm:text-2xl">
					{scholarship.title}
				</h1>
				{#if scholarship.institution}
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{scholarship.institution}</p>
				{/if}

				<!-- Deadline callout — the most actionable datum for a scholarship -->
				{#if deadlineValid}
					<div
						class="mt-4 flex items-center gap-3 rounded-[var(--radius)] border px-4 py-3 {deadlinePassed
							? 'border-[var(--color-danger)]/30 bg-[var(--color-danger)]/5'
							: 'border-[var(--color-primary)]/30 bg-[var(--color-primary)]/5'}"
					>
						<CalendarClock
							size={18}
							class={deadlinePassed ? 'text-[var(--color-danger)]' : 'text-[var(--color-primary)]'}
						/>
						<div class="min-w-0 flex-1">
							<p class="text-xs text-[var(--color-muted)]">{t('scholarships.deadline', 'Deadline')}</p>
							<p class="text-sm font-medium text-[var(--color-text)]">
								{formatDate(scholarship.application_deadline)}
							</p>
						</div>
						{#if daysToDeadline !== null}
							<span
								class="shrink-0 text-xs font-medium {deadlinePassed
									? 'text-[var(--color-danger)]'
									: 'text-[var(--color-primary)]'}"
							>
								{#if deadlinePassed}
									{t('scholarships.deadline_passed', 'Deadline passed')}
								{:else}
									{daysToDeadline} {t('scholarships.days_away', 'days away')}
								{/if}
							</span>
						{/if}
					</div>
				{:else}
					<p class="mt-4 text-xs text-[var(--color-muted)]">{t('scholarships.no_deadline', 'No deadline recorded')}</p>
				{/if}

				<!-- Quick facts grid -->
				<dl class="mt-5 grid grid-cols-1 gap-px overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-border)] sm:grid-cols-2">
					{#if scholarship.institution}
						<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
							<Building2 size={15} class="shrink-0 text-[var(--color-muted)]" />
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{t('scholarships.institution', 'Institution')}</dt>
								<dd class="truncate text-sm text-[var(--color-text)]">{scholarship.institution}</dd>
							</div>
						</div>
					{/if}
					{#if scholarship.level_of_study}
						<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
							<GraduationCap size={15} class="shrink-0 text-[var(--color-muted)]" />
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{t('scholarships.level', 'Level')}</dt>
								<dd class="truncate text-sm capitalize text-[var(--color-text)]">{scholarship.level_of_study}</dd>
							</div>
						</div>
					{/if}
					{#if scholarship.award_amount_usd !== null && scholarship.award_amount_usd !== undefined}
						<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
							<DollarSign size={15} class="shrink-0 text-[var(--color-muted)]" />
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{t('scholarships.award_amount', 'Award amount')}</dt>
								<dd class="truncate text-sm text-[var(--color-text)]">{formatAmount(scholarship.award_amount_usd)}</dd>
							</div>
						</div>
					{/if}
					{#if scholarship.is_renewable !== null && scholarship.is_renewable !== undefined}
						<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
							<RefreshCw size={15} class="shrink-0 text-[var(--color-muted)]" />
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{t('scholarships.renewable', 'Renewable')}</dt>
								<dd class="truncate text-sm text-[var(--color-text)]">
									{scholarship.is_renewable
										? t('scholarships.renewable_yes', 'Yes')
										: t('scholarships.one_time', 'One-time')}
								</dd>
							</div>
						</div>
					{/if}
					{#if scholarship.fields_of_study && scholarship.fields_of_study.length > 0}
						<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
							<BookOpen size={15} class="shrink-0 text-[var(--color-muted)]" />
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{t('scholarships.field_of_study', 'Field of study')}</dt>
								<dd class="truncate text-sm text-[var(--color-text)]">{scholarship.fields_of_study.join(', ')}</dd>
							</div>
						</div>
					{/if}
					{#if sourceDomain}
						<div class="flex items-center gap-2.5 bg-[var(--color-surface)] px-4 py-3">
							<Globe size={15} class="shrink-0 text-[var(--color-muted)]" />
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{t('scholarships.source', 'Source')}</dt>
								<dd class="truncate text-sm text-[var(--color-text)]">{sourceDomain}</dd>
							</div>
						</div>
					{/if}
				</dl>

				<!-- Overview -->
				{#if scholarship.description || scholarship.summary}
					<section class="mt-6">
						<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('scholarships.overview', 'Overview')}
						</h2>
						{#if scholarship.summary}
							<p class="whitespace-pre-line text-sm leading-relaxed text-[var(--color-text)]">{scholarship.summary}</p>
						{/if}
						{#if scholarship.description && scholarship.description !== scholarship.summary}
							<p class="whitespace-pre-line text-sm leading-relaxed text-[var(--color-text)] {scholarship.summary ? 'mt-3' : ''}">
								{scholarship.description}
							</p>
						{/if}
					</section>
				{/if}

				<!-- Eligibility -->
				{#if scholarship.eligibility}
					<section class="mt-6">
						<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('scholarships.eligibility', 'Eligibility')}
						</h2>
						<p class="whitespace-pre-line text-sm leading-relaxed text-[var(--color-text)]">{scholarship.eligibility}</p>
					</section>
				{/if}

				<!-- How to apply -->
				{#if scholarship.how_to_apply}
					<section class="mt-6">
						<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('scholarships.how_to_apply', 'How to apply')}
						</h2>
						<p class="whitespace-pre-line text-sm leading-relaxed text-[var(--color-text)]">{scholarship.how_to_apply}</p>
					</section>
				{/if}

				<!-- Contact -->
				{#if scholarship.contact_email}
					<section class="mt-6">
						<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('scholarships.contact', 'Contact')}
						</h2>
						<a
							href={`mailto:${scholarship.contact_email}`}
							class="text-sm text-[var(--color-primary)] hover:underline"
						>
							{scholarship.contact_email}
						</a>
					</section>
				{/if}

				<!-- Additional details — any field_values not covered above -->
				{#if extraFields.length > 0}
					<section class="mt-6">
						<h2 class="mb-2 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('scholarships.additional_details', 'Additional details')}
						</h2>
						<dl class="grid grid-cols-1 gap-px overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-border)] sm:grid-cols-2">
							{#each extraFields as f (f.label)}
								<div class="bg-[var(--color-surface)] px-4 py-3">
									<dt class="text-xs text-[var(--color-muted)]">{f.label}</dt>
									<dd class="text-sm text-[var(--color-text)]">{f.value}</dd>
								</div>
							{/each}
						</dl>
					</section>
				{/if}

				<!-- Provenance — source attribution + trust-tier readout -->
				{#if scholarship.source?.slug || trustTier > 0}
					{@const source = scholarship.source!}
					<section class="mt-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
						<h2 class="mb-3 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('scholarships.provenance', 'Provenance')}
						</h2>
						{#if source.slug}
							<div class="mb-3 flex items-center gap-1 text-sm">
								<EntityHoverChip
									type="source"
									id={source.id}
									slug={source.slug}
									name={source.name}
									avatar={source.logo_url}
									class="inline-flex items-center gap-0.5 font-medium text-[var(--color-primary)] hover:underline"
								>
									{#snippet trigger()}
										{t('scholarships.source', 'Source')}: {source.name}
									{/snippet}
								</EntityHoverChip>
								{#if source.source_url}
									<a
										href={source.source_url}
										target="_blank"
										rel="noopener noreferrer"
										class="inline-flex items-center gap-0.5 text-[var(--color-muted)] hover:text-[var(--color-primary)]"
										aria-label={t('scholarships.original_source', 'View original source')}
									>
										<ExternalLink size={14} />
									</a>
								{/if}
							</div>
						{/if}
						{#if trustTier > 0}
							<div>
								<div class="mb-1 flex items-center justify-between text-xs">
									<span class="text-[var(--color-muted)]">{t('scholarships.trust_tier', 'Trust tier')}</span>
									<span class="font-medium text-[var(--color-text)]">{trustPct}%</span>
								</div>
								<div class="h-1.5 overflow-hidden rounded-full bg-[var(--color-border)]">
									<div
										class="h-full rounded-full bg-[var(--color-primary)] transition-[width] duration-500"
										style="width: {trustPct}%"
									></div>
								</div>
							</div>
						{/if}
					</section>
				{/if}

				<!-- CTAs -->
				<div class="mt-6 flex flex-wrap items-center gap-4">
					{#if scholarship.application_url}
						<a
							href={scholarship.application_url}
							target="_blank"
							rel="noopener noreferrer nofollow"
							class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
						>
							{t('scholarships.apply', 'Apply')}
							<ExternalLink size={15} />
						</a>
					{/if}
					<a
						href={`/guide/${scholarship.slug}`}
						class="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-primary)] hover:underline"
					>
						<BookOpen size={15} />
						{t('scholarships.read_more_details', 'Read more details')}
					</a>
				</div>
			</div>
		</article>

		<!-- Save + tracker panel -->
		<div class="mt-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 space-y-4">
			<div class="flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
				<ClipboardList size={16} />
				{t('scholarships.tracker', 'Application Tracker')}
			</div>

			<div class="flex items-center justify-between">
				<button
					onclick={toggleSave}
					class="inline-flex items-center gap-2 rounded-[var(--radius)] px-4 py-2 text-sm font-medium transition-colors {savedItem ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'border border-[var(--color-border)] text-[var(--color-text)] hover:border-[var(--color-primary)]'}"
				>
					{#if savedItem}
						<BookmarkCheck size={18} />
						{t('scholarships.saved', 'Saved')}
					{:else}
						<Bookmark size={18} />
						{t('scholarships.save', 'Save scholarship')}
					{/if}
				</button>
			</div>

			{#if savedItem}
				<!-- Status dropdown -->
				<div>
					<label class="mb-1 block text-xs font-medium text-[var(--color-muted)]" for="status">
						{t('scholarships.application_status', 'Application status')}
					</label>
					<AutocompleteSelect
						id="status"
						value={savedItem.status}
						options={statusOptions}
						onValueChange={(v) => updateStatus(v)}
						disabled={updating}
					/>
				</div>

				<!-- Notes editor -->
				<div>
					<span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('scholarships.notes', 'Notes')}</span>
					{#if editingNotes}
						<div class="space-y-2">
							<textarea
								bind:value={notesDraft}
								rows="3"
								placeholder={t('scholarships.notes_placeholder', 'Add your notes about this scholarship...')}
								class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
							></textarea>
							<div class="flex gap-2">
								<button
									onclick={saveNotes}
									class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90"
								>
									{t('common.save', 'Save')}
								</button>
								<button
									onclick={() => (editingNotes = false)}
									class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
								>
									{t('common.cancel', 'Cancel')}
								</button>
							</div>
						</div>
					{:else}
						<div class="flex items-start justify-between gap-2">
							<p class="flex-1 whitespace-pre-line text-sm text-[var(--color-text-subtle)]">
								{savedItem.notes || t('scholarships.no_notes', 'No notes yet.')}
							</p>
							<button
								onclick={startEditNotes}
								class="flex-shrink-0 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
							>
								{t('scholarships.edit_notes', 'Edit')}
							</button>
						</div>
					{/if}
				</div>

				<!-- Reminder configuration -->
				<div>
					<span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">{t('scholarships.reminder_days', 'Reminder (days before deadline)')}</span>
					{#if editingReminder}
						<div class="flex items-center gap-2">
							<input
								type="number"
								bind:value={reminderDraft}
								min="0"
								max="90"
								class="w-24 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
							/>
							<button
								onclick={saveReminder}
								class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90"
							>
								{t('common.save', 'Save')}
							</button>
							<button
								onclick={() => (editingReminder = false)}
								class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
							>
								{t('common.cancel', 'Cancel')}
							</button>
						</div>
					{:else}
						<div class="flex items-center justify-between">
							<span class="text-sm text-[var(--color-text-subtle)]">
								{savedItem.remind_before_days === 0
									? t('scholarships.no_reminder', 'No reminder')
									: t('scholarships.reminder_value', '{days} days before').replace('{days}', String(savedItem.remind_before_days))}
							</span>
							<button
								onclick={startEditReminder}
								class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
							>
								{t('scholarships.edit_reminder', 'Edit')}
							</button>
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{/if}
</div>

{#if showRegisterPrompt}
	<!-- Registration prompt modal placeholder; main app shell handles login redirect -->
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4">
		<div class="w-full max-w-sm rounded-[var(--radius-card)] bg-[var(--color-surface)] p-5">
			<div class="mb-4 flex justify-center">
				<div class="grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
					<GraduationCap size={24} />
				</div>
			</div>
			<h2 class="text-center text-base font-semibold text-[var(--color-text)]">
				{t('register.save_scholarships', 'Save scholarships')}
			</h2>
			<p class="mt-1 text-center text-sm text-[var(--color-text-subtle)]">
				{t('register.save_scholarships_desc', 'Create a free account to save scholarships, track your application status, and get deadline reminders.')}
			</p>
			<div class="mt-4 flex flex-col gap-2">
				<a
					href="/login?redirect={encodeURIComponent(page.url.pathname + page.url.search)}"
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-center text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90"
				>
					{t('common.sign_in', 'Sign in')}
				</a>
				<a
					href="/register"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-center text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.create_account', 'Create free account')}
				</a>
			</div>
			<button
				type="button"
				onclick={() => (showRegisterPrompt = false)}
				class="mt-3 w-full text-center text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
			>
				{t('common.close', 'Close')}
			</button>
		</div>
	</div>
{/if}
