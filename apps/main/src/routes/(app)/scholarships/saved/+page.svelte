<script lang="ts">
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { GraduationCap, ClipboardList, ArrowLeft, ExternalLink, Trash2 } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
	import type { SavedScholarship } from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * Saved Scholarships tracker — Section 73, ADR-073.
	 *
	 * Ref: japaconnect-engineering/sections/73-scholarship-finder.md
	 */
	let { data }: { data: PageData } = $props();

	let saved = $state<SavedScholarship[]>(untrack(() => data.saved ?? []));
	let updating = $state<Record<string, boolean>>({});

	const statusOptions = $derived(
		(data.statuses ?? []).map((o) => ({
			value: o.value,
			label: t(`scholarships.status_${o.value}`, o.label)
		}))
	);

	const title = $derived(t('scholarships.saved_title', 'Saved Scholarships — JapaConnect'));
	const description = $derived(t('scholarships.saved_desc', 'Track your scholarship applications and deadlines.'));

	function statusBadgeColor(status: string): string {
		if (status === 'accepted') return 'bg-[var(--color-success)] text-white';
		if (status === 'rejected' || status === 'withdrawn') return 'bg-[var(--color-danger)] text-white';
		if (status === 'applied' || status === 'researching') return 'bg-[var(--color-info)] text-white';
		return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
	}

	async function updateStatus(savedItem: SavedScholarship, status: string) {
		updating[savedItem.id] = true;
		const res = await api.patch(`/scholarships/${savedItem.scholarship.guide_id}/save`, { status });
		if (!res.error) {
			saved = saved.map((s) => (s.id === savedItem.id ? { ...s, status } : s));
		}
		updating[savedItem.id] = false;
	}

	async function unsave(savedItem: SavedScholarship) {
		updating[savedItem.id] = true;
		const res = await api.delete(`/scholarships/${savedItem.scholarship.guide_id}/save`);
		if (!res.error) {
			saved = saved.filter((s) => s.id !== savedItem.id);
		}
		updating[savedItem.id] = false;
	}

	async function updateNotes(savedItem: SavedScholarship, notes: string) {
		updating[savedItem.id] = true;
		const res = await api.patch(`/scholarships/${savedItem.scholarship.guide_id}/save`, { notes });
		if (!res.error) {
			saved = saved.map((s) => (s.id === savedItem.id ? { ...s, notes } : s));
		}
		updating[savedItem.id] = false;
	}

	async function updateReminder(savedItem: SavedScholarship, remindBeforeDays: number) {
		updating[savedItem.id] = true;
		const res = await api.patch(`/scholarships/${savedItem.scholarship.guide_id}/save`, { remind_before_days: remindBeforeDays });
		if (!res.error) {
			saved = saved.map((s) => (s.id === savedItem.id ? { ...s, remind_before_days: remindBeforeDays } : s));
		}
		updating[savedItem.id] = false;
	}

	function formatDate(date: string | null | undefined): string {
		if (!date) return '—';
		return new Date(date).toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' });
	}

	function daysUntil(date: string | null | undefined): number | null {
		if (!date) return null;
		const diff = new Date(date).getTime() - Date.now();
		return Math.ceil(diff / (1000 * 60 * 60 * 24));
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<div class="flex items-center gap-2">
			<ClipboardList size={18} class="text-[var(--color-primary)]" />
			<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('scholarships.saved_title', 'Saved Scholarships')}</h1>
		</div>
		<p class="mt-1 text-sm text-[var(--color-muted)]">{description}</p>
	</div>

	<div class="border-b border-[var(--color-border)] px-4 py-2">
		<a
			href="/scholarships"
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('scholarships.back_to_list', 'Back to scholarships')}
		</a>
	</div>

	<div class="p-4">
		{#if saved.length === 0}
			<EmptyState
				icon={GraduationCap}
				title={t('scholarships.no_saved', 'No saved scholarships')}
				body={t('scholarships.no_saved_desc', 'Browse the scholarship finder and save opportunities to track them here.')}
				action={{
					label: t('scholarships.browse', 'Browse scholarships'),
					href: '/scholarships'
				}}
			/>
		{:else}
			<div class="space-y-3">
				{#each saved as item (item.id)}
					{@const s = item.scholarship}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<a
									href="/scholarships/{s.guide_id}"
									class="text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)]"
								>
									{s.title}
								</a>
								{#if s.institution}
									<p class="text-xs text-[var(--color-text-subtle)]">{s.institution}</p>
								{/if}
								<div class="mt-2 flex flex-wrap items-center gap-2 text-xs">
									<span class="rounded-full px-2 py-0.5 font-medium {statusBadgeColor(item.status)}">
										{statusOptions.find((o) => o.value === item.status)?.label ?? item.status}
									</span>
									{#if s.application_deadline}
										{@const days = daysUntil(s.application_deadline)}
										<span class="text-[var(--color-muted)]">
											{t('scholarships.deadline', 'Deadline')}: {formatDate(s.application_deadline)}
											{#if days !== null}
												<span class="ml-1 {days < 7 ? 'text-[var(--color-danger)]' : ''}">
													({days} {days === 1 ? t('scholarships.day', 'day') : t('scholarships.days', 'days')} left)
												</span>
											{/if}
										</span>
									{/if}
								</div>
								{#if item.notes}
									<p class="mt-2 text-xs text-[var(--color-text-subtle)] whitespace-pre-line">{item.notes}</p>
								{/if}
							</div>
							<div class="flex flex-col gap-2">
								<select
									value={item.status}
									onchange={(e) => updateStatus(item, (e.currentTarget as HTMLSelectElement).value)}
									disabled={updating[item.id]}
									class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								>
									{#each statusOptions as opt (opt.value)}
										<option value={opt.value}>{opt.label}</option>
									{/each}
								</select>
								<button
									type="button"
									onclick={() => unsave(item)}
									disabled={updating[item.id]}
									class="inline-flex items-center justify-center rounded-[var(--radius)] border border-[var(--color-border)] p-1.5 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
									aria-label={t('common.remove', 'Remove')}
								>
									<Trash2 size={14} />
								</button>
							</div>
						</div>

						{#if s.application_url}
							<a
								href={s.application_url}
								target="_blank"
								rel="noopener noreferrer"
								class="mt-3 inline-flex items-center gap-1.5 text-xs text-[var(--color-primary)] hover:underline"
							>
								{t('scholarships.apply', 'Apply')}
								<ExternalLink size={12} />
							</a>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
