<script lang="ts">
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import type { SavedScholarship, ScholarshipStatus, CatalogOption } from '$lib/types';

	/**
	 * Saved Scholarships tracker — §73, ADR-073.
	 * Lists all scholarships the user has saved with their application
	 * status. Quick status change via dropdown; links to the detail page
	 * for notes/reminder editing.
	 */
	let { data } = $props();

	let saved = $state<SavedScholarship[]>(untrack(() => data.saved));
	let statusOptions = $state<CatalogOption[]>(untrack(() => data.statusOptions));

	// Scholarship application status options are loaded from
	// GET /scholarships/statuses (Rule 16 — no hardcoded option lists).
	// Labels are translated client-side using value-based i18n keys.

	const seo = $derived(
		buildSeoMeta({
			title: t('scholarships.tracker_title', 'Saved Scholarships — JapaConnect'),
			description: t('scholarships.tracker_seo_desc', 'Track your saved scholarships and application status.'),
			path: '/scholarships/saved',
			origin: page.url.origin,
			noindex: true
		})
	);

	async function updateStatus(guideId: string, status: ScholarshipStatus) {
		const res = await api.patch(`/scholarships/${guideId}/save`, { status });
		if (!res.error) {
			saved = saved.map((s) => (s.scholarship.guide_id === guideId ? { ...s, status } : s));
		}
	}

	async function unsave(guideId: string) {
		const res = await api.delete(`/scholarships/${guideId}/save`);
		if (!res.error) {
			saved = saved.filter((s) => s.scholarship.guide_id !== guideId);
		}
	}

	async function updateNotes(guideId: string, notes: string) {
		const res = await api.patch(`/scholarships/${guideId}/save`, { notes });
		if (!res.error) {
			saved = saved.map((s) => (s.scholarship.guide_id === guideId ? { ...s, notes } : s));
		}
	}

	async function updateReminder(guideId: string, remindBeforeDays: number) {
		const res = await api.patch(`/scholarships/${guideId}/save`, { remind_before_days: remindBeforeDays });
		if (!res.error) {
			saved = saved.map((s) => (s.scholarship.guide_id === guideId ? { ...s, remind_before_days: remindBeforeDays } : s));
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
		return new Date(date).toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
	}

	function daysUntilDeadline(deadline: string | null | undefined): number | null {
		if (!deadline) return null;
		const diff = new Date(deadline).getTime() - Date.now();
		return Math.ceil(diff / (1000 * 60 * 60 * 24));
	}
</script>

<svelte:head>
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	{#if seo.noindex}
		<meta name="robots" content="noindex" />
	{/if}
</svelte:head>

{#if !data.enabled}
	<ToolDisabled tool={data.tool} />
{:else}

<div class="mx-auto max-w-4xl px-4 py-8">
	<div class="mb-8">
		<h1 class="text-2xl sm:text-3xl font-bold text-text">{t('scholarships.tracker_title', 'Saved Scholarships')}</h1>
		<p class="mt-2 text-text-subtle">
			{t('scholarships.tracker_subtitle', 'Track your saved scholarships and application progress.')}
		</p>
	</div>

	{#if saved.length === 0}
		<EmptyState
			icon="bookmark"
			title={t('scholarships.tracker_empty', 'No saved scholarships yet')}
			description={t('scholarships.tracker_empty_desc', 'Browse the scholarship finder and save scholarships to track them here.')}
			action={{ label: t('scholarships.browse', 'Browse scholarships'), href: '/scholarships' }}
		/>
	{:else}
		<div class="space-y-3">
			{#each saved as item (item.id)}
				{@const days = daysUntilDeadline(item.scholarship.application_deadline)}
				<div class="p-5 bg-surface border border-border rounded-lg">
					<div class="flex items-start justify-between gap-4">
						<div class="flex-1 min-w-0">
							<a href={`/scholarships/${item.scholarship.guide_id}`} class="text-base font-semibold text-text hover:text-[var(--tool-accent)] transition-colors">
								{item.scholarship.title}
							</a>
							{#if item.scholarship.institution}
								<p class="text-sm text-text-subtle mt-0.5">{item.scholarship.institution}</p>
							{/if}
							<div class="flex flex-wrap gap-2 mt-2">
								{#if item.scholarship.country}
									<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded">{item.scholarship.country}</span>
								{/if}
								{#if item.scholarship.award_amount_usd !== null && item.scholarship.award_amount_usd !== undefined}
									<span class="text-xs px-2 py-0.5 bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] rounded font-medium">
										{formatAmount(item.scholarship.award_amount_usd)}
									</span>
								{/if}
								{#if item.scholarship.application_deadline}
									<span class="text-xs px-2 py-0.5 bg-surface-subtle text-muted rounded inline-flex items-center gap-1">
										<Icon name="calendar" size={12} />
										{formatDate(item.scholarship.application_deadline)}
										{#if days !== null && days >= 0 && days <= 30}
											<span class="text-orange-600 dark:text-orange-400 font-medium">· {days}d</span>
										{:else if days !== null && days < 0}
											<span class="text-red-600 dark:text-red-400 font-medium">· {t('scholarships.deadline_passed', 'Passed')}</span>
										{/if}
									</span>
								{/if}
							</div>
							{#if item.notes}
								<p class="text-sm text-muted mt-2 italic">{item.notes}</p>
							{/if}
							<div class="mt-3 space-y-2">
								<textarea
									value={item.notes ?? ''}
									onchange={(e) => updateNotes(item.scholarship.guide_id, e.currentTarget.value)}
									rows="2"
									placeholder={t('scholarships.notes_placeholder', 'Add notes...')}
									class="w-full px-3 py-2 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none resize-none"
								></textarea>
								<div class="flex items-center gap-2">
									<label for="reminder-{item.id}" class="text-xs text-muted">{t('scholarships.reminder_days', 'Remind')}:</label>
									<input
										id="reminder-{item.id}"
										type="number"
										value={item.remind_before_days ?? data.defaultReminderDays}
										onchange={(e) => updateReminder(item.scholarship.guide_id, parseInt(e.currentTarget.value, 10) || 0)}
										min="0"
										max="90"
										class="w-24 px-2 py-1 text-sm bg-surface border border-border rounded focus:border-primary focus:outline-none"
									/>
									<span class="text-xs text-muted">{t('scholarships.days_before', 'days before')}</span>
								</div>
							</div>
						</div>
						<div class="flex-shrink-0 flex items-center gap-2">
							<select
								value={item.status}
								onchange={(e) => updateStatus(item.scholarship.guide_id, e.currentTarget.value as ScholarshipStatus)}
								class="px-2 py-1 text-xs bg-surface border border-border rounded focus:border-primary focus:outline-none"
							>
								{#each statusOptions as opt (opt.value)}
									<option value={opt.value}>{t(`scholarships.status_${opt.value}`, opt.label)}</option>
								{/each}
							</select>
							<button
								onclick={() => unsave(item.scholarship.guide_id)}
								class="p-1.5 text-muted hover:text-red-500 transition-colors"
								aria-label={t('scholarships.unsave', 'Remove from saved')}
							>
								<Icon name="trash-2" size={16} />
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
{/if}
