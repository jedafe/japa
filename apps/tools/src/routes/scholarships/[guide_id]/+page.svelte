<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import RegistrationPrompt from '$lib/components/ui/RegistrationPrompt.svelte';
	import type { ScholarshipDetail, SavedScholarship, ScholarshipStatus, CatalogOption } from '$lib/types';

	/**
	 * Scholarship detail page — §73, ADR-073.
	 * Shows full scholarship info (eligibility, how-to-apply, contact) and
	 * lets authenticated users save/unsave, update application status, edit
	 * notes, and configure reminder timing.
	 */
	let { data } = $props();

	let scholarship = $state<ScholarshipDetail | null>(untrack(() => data.scholarship));
	let savedItem = $state<SavedScholarship | null>(untrack(() => data.savedItem));
	let user = $state(untrack(() => data.user));
	let statusOptions = $state<CatalogOption[]>(untrack(() => data.statusOptions));

	let showRegisterPrompt = $state(false);
	let editingNotes = $state(false);
	let notesDraft = $state('');
	let editingReminder = $state(false);
	let reminderDraft = $state(14);
	let updating = $state(false);

	// Scholarship application status options are loaded from
	// GET /scholarships/statuses (Rule 16 — no hardcoded option lists).
	// Labels are translated client-side using value-based i18n keys.

	const seo = $derived(
		buildSeoMeta({
			title: scholarship
				? `${scholarship.title} — ${t('scholarships.seo_title', 'Scholarship Finder — JapaConnect Tools')}`
				: t('scholarships.seo_title', 'Scholarship Finder — JapaConnect Tools'),
			description: scholarship?.description ?? t('scholarships.seo_description', 'Filter scholarships by country, level of study, award amount, and deadline.'),
			path: `/scholarships/${page.params.guide_id}`,
			origin: page.url.origin,
			jsonLd: scholarship
				? {
						'@context': 'https://schema.org',
						'@type': 'EducationalOccupationalProgram',
						name: scholarship.title,
						description: scholarship.description ?? '',
						provider: scholarship.institution ?? undefined
					}
				: undefined
		})
	);

	async function toggleSave() {
		if (!scholarship) return;
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (savedItem) {
			const res = await api.delete(`/scholarships/${scholarship.id}/save`);
			if (!res.error) savedItem = null;
		} else {
			const res = await api.post<{ id: string }>(`/scholarships/${scholarship.id}/save`);
			if (!res.error && res.data) {
				savedItem = {
					id: res.data.id,
					scholarship,
					status: 'saved' as ScholarshipStatus,
					notes: null,
					remind_before_days: 14,
					last_reminded_at: null,
					created_at: new Date().toISOString(),
					updated_at: new Date().toISOString()
				};
			}
		}
	}

	async function updateStatus(status: ScholarshipStatus) {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (!scholarship || !savedItem) return;
		updating = true;
		const res = await api.patch(`/scholarships/${scholarship.id}/save`, { status });
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
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (!scholarship || !savedItem) return;
		editingNotes = false;
		const res = await api.patch(`/scholarships/${scholarship.id}/save`, { notes: notesDraft });
		if (!res.error && savedItem) {
			savedItem = { ...savedItem, notes: notesDraft };
		}
	}

	function startEditReminder() {
		reminderDraft = savedItem?.remind_before_days ?? 14;
		editingReminder = true;
	}

	async function saveReminder() {
		if (!user) {
			showRegisterPrompt = true;
			return;
		}
		if (!scholarship || !savedItem) return;
		editingReminder = false;
		const res = await api.patch(`/scholarships/${scholarship.id}/save`, {
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
		return new Date(date).toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' });
	}
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

<div class="mx-auto max-w-3xl px-4 py-8">
	<a href="/scholarships" class="inline-flex items-center gap-1.5 text-sm text-muted hover:text-text transition-colors mb-6">
		<Icon name="arrow-left" size={16} />
		{t('scholarships.back_to_list', 'Back to scholarships')}
	</a>

	{#if !scholarship}
		<EmptyState
			icon="search-x"
			title={t('scholarships.not_found', 'Scholarship not found')}
			description={t('scholarships.not_found_desc', 'This scholarship may have been removed or is no longer available.')}
		/>
	{:else}
		<div class="space-y-6">
			<!-- Header -->
			<div class="p-6 bg-surface border border-border rounded-lg">
				<h1 class="text-2xl font-bold text-text">{scholarship.title}</h1>
				{#if scholarship.institution}
					<p class="mt-1 text-lg text-text-subtle">{scholarship.institution}</p>
				{/if}
				<div class="flex flex-wrap gap-2 mt-3">
					{#if scholarship.country}
						<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">{scholarship.country}</span>
					{/if}
					{#if scholarship.level_of_study}
						<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded capitalize">{scholarship.level_of_study}</span>
					{/if}
					{#if scholarship.is_renewable}
						<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">{t('scholarships.renewable_badge', 'Renewable')}</span>
					{/if}
				</div>
				{#if scholarship.award_amount_usd !== null && scholarship.award_amount_usd !== undefined}
					<div class="mt-4 text-2xl font-bold text-[var(--tool-accent)]">
						{formatAmount(scholarship.award_amount_usd)}
						<span class="text-sm font-normal text-muted">{t('scholarships.award', 'award')}</span>
					</div>
				{/if}
			</div>

			<!-- Key dates -->
			{#if scholarship.application_deadline || scholarship.application_url}
				<div class="p-4 bg-surface border border-border rounded-lg space-y-2">
					{#if scholarship.application_deadline}
						<div class="flex items-center gap-2 text-sm">
							<Icon name="calendar" size={16} class="text-muted" />
							<span class="text-text-subtle">{t('scholarships.deadline', 'Deadline:')} <strong class="text-text">{formatDate(scholarship.application_deadline)}</strong></span>
						</div>
					{/if}
					{#if scholarship.application_url}
						<a
							href={scholarship.application_url}
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-1.5 px-4 py-2 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
						>
							{t('scholarships.apply', 'Apply')}
							<Icon name="external-link" size={14} />
						</a>
					{/if}
				</div>
			{/if}

			<!-- Description -->
			{#if scholarship.description}
				<div class="prose prose-sm max-w-none text-text-subtle">
					<p>{scholarship.description}</p>
				</div>
			{/if}

			<!-- Eligibility -->
			{#if scholarship.eligibility}
				<div>
					<h2 class="text-lg font-semibold text-text mb-2">{t('scholarships.eligibility', 'Eligibility')}</h2>
					<p class="text-sm text-text-subtle whitespace-pre-line">{scholarship.eligibility}</p>
				</div>
			{/if}

			<!-- How to apply -->
			{#if scholarship.how_to_apply}
				<div>
					<h2 class="text-lg font-semibold text-text mb-2">{t('scholarships.how_to_apply', 'How to apply')}</h2>
					<p class="text-sm text-text-subtle whitespace-pre-line">{scholarship.how_to_apply}</p>
				</div>
			{/if}

			<!-- Contact -->
			{#if scholarship.contact_email}
				<div>
					<h2 class="text-lg font-semibold text-text mb-2">{t('scholarships.contact', 'Contact')}</h2>
					<a href={`mailto:${scholarship.contact_email}`} class="text-sm text-primary hover:text-primary-hover">
						{scholarship.contact_email}
					</a>
				</div>
			{/if}

			<!-- Save + tracker panel -->
			<div class="p-5 bg-surface border border-border rounded-lg space-y-4">
				<div class="flex items-center justify-between">
					<button
						onclick={toggleSave}
						class="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-colors {savedItem ? 'bg-[var(--tool-accent)]/10 text-[var(--tool-accent)]' : 'border border-border text-text hover:border-[var(--tool-accent)]'}"
					>
						<Icon name={savedItem ? 'bookmark-check' : 'bookmark'} size={18} />
						{savedItem ? t('scholarships.saved', 'Saved') : t('scholarships.save', 'Save')}
					</button>
				</div>

				{#if savedItem}
					<!-- Status dropdown -->
					<div>
						<label class="block text-xs font-medium text-muted mb-1" for="status">{t('scholarships.application_status', 'Application status')}</label>
						<select
							id="status"
							value={savedItem.status}
							onchange={(e) => updateStatus(e.currentTarget.value as ScholarshipStatus)}
							disabled={updating}
							class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
						>
							{#each statusOptions as opt (opt.value)}
								<option value={opt.value}>{t(`scholarships.status_${opt.value}`, opt.label)}</option>
							{/each}
						</select>
					</div>

					<!-- Notes editor -->
					<div>
						<span class="block text-xs font-medium text-muted mb-1">{t('scholarships.notes', 'Notes')}</span>
						{#if editingNotes}
							<div class="space-y-2">
								<textarea
									bind:value={notesDraft}
									rows="3"
									placeholder={t('scholarships.notes_placeholder', 'Add your notes about this scholarship...')}
									class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none resize-none"
								></textarea>
								<div class="flex gap-2">
									<button onclick={saveNotes} class="px-3 py-1.5 text-sm bg-primary text-primary-contrast rounded hover:bg-primary-hover">
										{t('common.save', 'Save')}
									</button>
									<button onclick={() => (editingNotes = false)} class="px-3 py-1.5 text-sm text-muted hover:text-text">
										{t('common.cancel', 'Cancel')}
									</button>
								</div>
							</div>
						{:else}
							<div class="flex items-start justify-between gap-2">
								<p class="text-sm text-text-subtle whitespace-pre-line flex-1">
									{savedItem.notes || t('scholarships.no_notes', 'No notes yet.')}
								</p>
								<button onclick={startEditNotes} class="text-xs text-muted hover:text-text flex-shrink-0">
									{t('scholarships.edit_notes', 'Edit')}
								</button>
							</div>
						{/if}
					</div>

					<!-- Reminder configuration -->
					<div>
						<span class="block text-xs font-medium text-muted mb-1">{t('scholarships.reminder_days', 'Reminder (days before deadline)')}</span>
						{#if editingReminder}
							<div class="flex items-center gap-2">
								<input
									type="number"
									bind:value={reminderDraft}
									min="0"
									max="90"
									class="w-24 px-3 py-1.5 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
								/>
								<button onclick={saveReminder} class="px-3 py-1.5 text-sm bg-primary text-primary-contrast rounded hover:bg-primary-hover">
									{t('common.save', 'Save')}
								</button>
								<button onclick={() => (editingReminder = false)} class="px-3 py-1.5 text-sm text-muted hover:text-text">
									{t('common.cancel', 'Cancel')}
								</button>
							</div>
						{:else}
							<div class="flex items-center justify-between">
								<span class="text-sm text-text-subtle">
									{savedItem.remind_before_days === 0
										? t('scholarships.no_reminder', 'No reminder')
										: t('scholarships.reminder_value', '{days} days before').replace('{days}', String(savedItem.remind_before_days))}
								</span>
								<button onclick={startEditReminder} class="text-xs text-muted hover:text-text">
									{t('scholarships.edit_reminder', 'Edit')}
								</button>
							</div>
						{/if}
					</div>
				{/if}
			</div>
		</div>
	{/if}

	{#if showRegisterPrompt}
		<RegistrationPrompt
			open={showRegisterPrompt}
			onClose={() => (showRegisterPrompt = false)}
			icon="user-plus"
			title={t('register.save_scholarships', 'Save scholarships')}
			description={t('register.save_scholarships_desc', 'Create a free account to save scholarships, track your application status, and get deadline reminders.')}
		/>
	{/if}
</div>
