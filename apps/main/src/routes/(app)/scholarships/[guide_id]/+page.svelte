<script lang="ts">
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import {
		GraduationCap,
		Bookmark,
		BookmarkCheck,
		Calendar,
		ExternalLink,
		ArrowLeft,
		ClipboardList,
		Bell
	} from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
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

	let showRegisterPrompt = $state(false);
	let editingNotes = $state(false);
	let notesDraft = $state('');
	let editingReminder = $state(false);
	let reminderDraft = $state(14);
	let updating = $state(false);

	const statusOptions = $derived(
		(data.statuses ?? []).map((o) => ({
			value: o.value,
			label: t(`scholarships.status_${o.value}`, o.label)
		}))
	);

	const title = $derived(`${scholarship.title} — ${t('tools.scholarships', 'Scholarship Finder — JapaConnect')}`);
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
					remind_before_days: 14,
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
		reminderDraft = savedItem?.remind_before_days ?? 14;
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

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/scholarships"
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('scholarships.back_to_list', 'Back to scholarships')}
		</a>
	</div>

	{#if !scholarship}
		<div class="p-4">
			<EmptyState
				icon={GraduationCap}
				title={t('scholarships.not_found', 'Scholarship not found')}
				body={t('scholarships.not_found_desc', 'This scholarship may have been removed or is no longer available.')}
			/>
		</div>
	{:else}
		<div class="space-y-4 p-4">
			<!-- Header -->
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
				<div class="flex items-start justify-between gap-3">
					<div class="min-w-0 flex-1">
						<h1 class="text-lg font-semibold text-[var(--color-text)]">{scholarship.title}</h1>
						{#if scholarship.institution}
							<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{scholarship.institution}</p>
						{/if}
						<div class="mt-2 flex flex-wrap gap-2">
							{#if scholarship.country}
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
									{scholarship.country}
								</span>
							{/if}
							{#if scholarship.level_of_study}
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-muted)] capitalize">
									{scholarship.level_of_study}
								</span>
							{/if}
							{#if scholarship.is_renewable}
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs text-[var(--color-muted)]">
									{t('scholarships.renewable_badge', 'Renewable')}
								</span>
							{/if}
						</div>
					</div>
					<button
						type="button"
						onclick={toggleSave}
						class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] p-2 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						aria-label={savedItem ? t('common.saved', 'Saved') : t('common.save', 'Save')}
					>
						{#if savedItem}
							<BookmarkCheck size={18} class="text-[var(--color-primary)]" />
						{:else}
							<Bookmark size={18} />
						{/if}
					</button>
				</div>

				{#if scholarship.award_amount_usd !== null && scholarship.award_amount_usd !== undefined}
					<div class="mt-4 text-2xl font-bold text-[var(--color-primary)]">
						{formatAmount(scholarship.award_amount_usd)}
						<span class="text-sm font-normal text-[var(--color-muted)]">{t('scholarships.award', 'award')}</span>
					</div>
				{/if}
			</div>

			<!-- Key dates -->
			{#if scholarship.application_deadline || scholarship.application_url}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 space-y-2">
					{#if scholarship.application_deadline}
						<div class="flex items-center gap-2 text-sm text-[var(--color-text-subtle)]">
							<Calendar size={16} class="text-[var(--color-muted)]" />
							<span>
								{t('scholarships.deadline', 'Deadline:')}
								<strong class="text-[var(--color-text)]">{formatDate(scholarship.application_deadline)}</strong>
							</span>
						</div>
					{/if}
					{#if scholarship.application_url}
						<a
							href={scholarship.application_url}
							target="_blank"
							rel="noopener noreferrer"
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:opacity-90 transition-opacity"
						>
							{t('scholarships.apply', 'Apply')}
							<ExternalLink size={14} />
						</a>
					{/if}
				</div>
			{/if}

			<!-- Description -->
			{#if scholarship.description}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<p class="text-sm text-[var(--color-text-subtle)] whitespace-pre-line">{scholarship.description}</p>
				</div>
			{/if}

			<!-- Eligibility -->
			{#if scholarship.eligibility}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('scholarships.eligibility', 'Eligibility')}</h2>
					<p class="text-sm text-[var(--color-text-subtle)] whitespace-pre-line">{scholarship.eligibility}</p>
				</div>
			{/if}

			<!-- How to apply -->
			{#if scholarship.how_to_apply}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('scholarships.how_to_apply', 'How to apply')}</h2>
					<p class="text-sm text-[var(--color-text-subtle)] whitespace-pre-line">{scholarship.how_to_apply}</p>
				</div>
			{/if}

			<!-- Contact -->
			{#if scholarship.contact_email}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('scholarships.contact', 'Contact')}</h2>
					<a
						href={`mailto:${scholarship.contact_email}`}
						class="text-sm text-[var(--color-primary)] hover:underline"
					>
						{scholarship.contact_email}
					</a>
				</div>
			{/if}

			<!-- Save + tracker panel -->
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 space-y-4">
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
						<select
							id="status"
							value={savedItem.status}
							onchange={(e) => updateStatus((e.currentTarget as HTMLSelectElement).value)}
							disabled={updating}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						>
							{#each statusOptions as opt (opt.value)}
								<option value={opt.value}>{opt.label}</option>
							{/each}
						</select>
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
