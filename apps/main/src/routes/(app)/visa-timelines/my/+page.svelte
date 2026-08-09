// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { ArrowLeft, CalendarClock, Check, ChevronDown, ChevronRight, Pencil, Plus, X } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type {
		VisaApplicationMethod,
		VisaMilestoneType,
		VisaTimeline,
		VisaTimelineMilestone,
		VisaTimelineOutcome
	} from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let timelines = $state<VisaTimeline[]>(untrack(() => data.timelines));
	let loadError = $derived(data.loadError);

	// Resolve a space UUID to its display name using the SSR-loaded country list.
	const spaceName = (id: string | undefined): string => {
		if (!id) return '';
		return data.countries?.find((c) => c.id === id)?.name ?? id;
	};

	// Refresh local state when SSR data changes (e.g. on navigation).
	$effect(() => {
		timelines = data.timelines;
	});

	const title = $derived(t('visa_timelines.my_title', 'My Timelines — JapaConnect'));

	// --- Create form ---
	let showCreate = $state(false);
	let creating = $state(false);
	let createError = $state('');

	let cOrigin = $state('');
	let cDest = $state('');
	let cVisa = $state('');
	let cConsulate = $state('');
	let cMethod = $state<VisaApplicationMethod>('online');
	let cOutcome = $state<VisaTimelineOutcome>('pending');
	let cShared = $state(true);

	const applicationMethods: VisaApplicationMethod[] = ['online', 'in_person', 'vfs'];
	const outcomes: VisaTimelineOutcome[] = ['approved', 'refused', 'withdrawn', 'pending'];
	const milestoneTypes: VisaMilestoneType[] = [
		'applied',
		'biometrics',
		'medical',
		'docs_requested',
		'decision',
		'visa_issued'
	];

	function resetCreateForm() {
		cOrigin = '';
		cDest = '';
		cVisa = '';
		cConsulate = '';
		cMethod = 'online';
		cOutcome = 'pending';
		cShared = true;
		createError = '';
	}

	async function submitCreate(e: SubmitEvent) {
		e.preventDefault();
		if (!cDest || !cVisa.trim()) {
			createError = t('visa_timelines.errors.required', 'Origin, destination, and visa category are required.');
			return;
		}
		creating = true;
		createError = '';
		const res = await api.post<VisaTimeline>('/me/visa-timelines', {
			origin_space_id: cOrigin || undefined,
			destination_space_id: cDest,
			visa_category: cVisa.trim(),
			consulate: cConsulate.trim(),
			application_method: cMethod,
			outcome: cOutcome,
			is_shared: cShared
		});
		creating = false;
		if (res.data) {
			timelines = [...timelines, { ...res.data, milestones: res.data.milestones ?? [] }];
			resetCreateForm();
			showCreate = false;
		} else {
			createError = res.error?.message ?? t('visa_timelines.errors.create_failed', 'Failed to create timeline.');
		}
	}

	// --- is_shared toggle ---
	let togglingId = $state<string | null>(null);
	async function toggleShared(tl: VisaTimeline) {
		togglingId = tl.id;
		const res = await api.patch<VisaTimeline>(`/me/visa-timelines/${tl.id}`, {
			is_shared: !tl.is_shared
		});
		togglingId = null;
		if (res.data) {
			timelines = timelines.map((x) => (x.id === tl.id ? { ...x, is_shared: res.data!.is_shared } : x));
		}
	}

	// --- Milestone add form (per timeline) ---
	let expandedId = $state<string | null>(null);
	let milestoneForms = $state<Record<string, { type: VisaMilestoneType; occurred_at: string; notes: string }>>({});
	let milestoneSaving = $state<string | null>(null);
	let milestoneError = $state<Record<string, string>>({});

	function ensureMilestoneForm(id: string) {
		if (!milestoneForms[id]) {
			milestoneForms = { ...milestoneForms, [id]: { type: 'applied', occurred_at: '', notes: '' } };
		}
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
		if (expandedId === id) ensureMilestoneForm(id);
	}

	async function addMilestone(tl: VisaTimeline, e: SubmitEvent) {
		e.preventDefault();
		const form = milestoneForms[tl.id];
		if (!form || !form.occurred_at) {
			milestoneError = { ...milestoneError, [tl.id]: t('visa_timelines.errors.date_required', 'Date is required.') };
			return;
		}
		milestoneSaving = tl.id;
		milestoneError = { ...milestoneError, [tl.id]: '' };
		const res = await api.post<VisaTimelineMilestone>(`/me/visa-timelines/${tl.id}/milestones`, {
			milestone_type: form.type,
			occurred_at: form.occurred_at,
			notes: form.notes.trim()
		});
		milestoneSaving = null;
		if (res.data) {
			timelines = timelines.map((x) =>
				x.id === tl.id ? { ...x, milestones: [...(x.milestones ?? []), res.data!] } : x
			);
			milestoneForms = { ...milestoneForms, [tl.id]: { type: 'applied', occurred_at: '', notes: '' } };
		} else {
			milestoneError = {
				...milestoneError,
				[tl.id]: res.error?.message ?? t('visa_timelines.errors.milestone_failed', 'Failed to add milestone.')
			};
		}
	}

	// --- Milestone edit ---
	let editingMilestone = $state<string | null>(null);
	let editForm = $state<{ occurred_at: string; notes: string }>({ occurred_at: '', notes: '' });
	let editSaving = $state(false);
	let editError = $state('');

	function startEditMilestone(m: VisaTimelineMilestone) {
		editingMilestone = m.id;
		editForm = { occurred_at: m.occurred_at.slice(0, 10), notes: m.notes ?? '' };
		editError = '';
	}

	function cancelEditMilestone() {
		editingMilestone = null;
		editError = '';
	}

	async function saveEditMilestone(tl: VisaTimeline, m: VisaTimelineMilestone, e: SubmitEvent) {
		e.preventDefault();
		editSaving = true;
		editError = '';
		const res = await api.patch<VisaTimelineMilestone>(
			`/me/visa-timelines/${tl.id}/milestones/${m.id}`,
			{ occurred_at: editForm.occurred_at, notes: editForm.notes.trim() }
		);
		editSaving = false;
		if (res.data) {
			timelines = timelines.map((x) =>
				x.id === tl.id
					? {
							...x,
							milestones: (x.milestones ?? []).map((mm) => (mm.id === m.id ? res.data! : mm))
						}
					: x
			);
			editingMilestone = null;
		} else {
			editError = res.error?.message ?? t('visa_timelines.errors.milestone_update_failed', 'Failed to update milestone.');
		}
	}

	function outcomeColor(o: VisaTimelineOutcome): string {
		switch (o) {
			case 'approved':
				return 'bg-[var(--color-success)]/15 text-[var(--color-success)]';
			case 'refused':
				return 'bg-[var(--color-danger)]/15 text-[var(--color-danger)]';
			case 'withdrawn':
				return 'bg-[var(--color-muted)]/15 text-[var(--color-muted)]';
			default:
				return 'bg-[var(--color-info)]/15 text-[var(--color-info)]';
		}
	}

	function fmtDate(iso: string): string {
		try {
			return new Date(iso).toLocaleDateString();
		} catch {
			return iso;
		}
	}

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
	const labelClass = 'text-xs font-medium text-[var(--color-text-subtle)]';
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<CalendarClock size={20} /> {t('visa_timelines.my_heading', 'My Timelines')}
		</h1>
		<button
			type="button"
			onclick={() => (showCreate = !showCreate)}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			<Plus size={14} /> {t('visa_timelines.add_timeline', 'Add Timeline')}
		</button>
	</div>

	<div class="p-4">
		<a href="/visa-timelines" class="mb-4 inline-flex items-center gap-1 text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]">
			<ArrowLeft size={13} /> {t('visa_timelines.back_to_stats', 'Back to stats')}
		</a>

		{#if loadError}
			<div class="rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 px-4 py-3 text-sm text-[var(--color-danger)]">
				{t('visa_timelines.load_error', 'Could not load your timelines. Please try again later.')}
			</div>
		{/if}

		{#if showCreate}
			<form
				onsubmit={submitCreate}
				class="mb-5 space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				novalidate
			>
				<div class="flex items-center justify-between">
					<h2 class="text-sm font-medium text-[var(--color-text)]">
						{t('visa_timelines.new_timeline', 'New timeline')}
					</h2>
					<button
						type="button"
						onclick={() => (showCreate = false)}
						class="text-[var(--color-muted)] hover:text-[var(--color-text)]"
						aria-label={t('common.cancel', 'Cancel')}
					>
						<X size={16} />
					</button>
				</div>
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
					<div class="flex flex-col gap-1">
						<label for="c-origin" class={labelClass}>{t('visa_timelines.origin', 'Origin')}</label>
						<select id="c-origin" bind:value={cOrigin} class={inputClass}>
							<option value="">{t('visa_timelines.origin', 'Origin')}</option>
							{#each data.countries as country (country.id)}
								<option value={country.id}>{country.name}</option>
							{/each}
						</select>
					</div>
					<div class="flex flex-col gap-1">
						<label for="c-dest" class={labelClass}>{t('visa_timelines.destination', 'Destination')} *</label>
						<select id="c-dest" bind:value={cDest} class={inputClass}>
							<option value="">{t('visa_timelines.destination', 'Destination')}</option>
							{#each data.countries as country (country.id)}
								<option value={country.id}>{country.name}</option>
							{/each}
						</select>
					</div>
					<div class="flex flex-col gap-1">
						<label for="c-visa" class={labelClass}>{t('visa_timelines.visa_category', 'Visa category')} *</label>
						<input id="c-visa" type="text" bind:value={cVisa} class={inputClass} placeholder="e.g. student" />
					</div>
					<div class="flex flex-col gap-1">
						<label for="c-consulate" class={labelClass}>{t('visa_timelines.consulate', 'Consulate')}</label>
						<input id="c-consulate" type="text" bind:value={cConsulate} class={inputClass} placeholder="e.g. Lagos" />
					</div>
					<div class="flex flex-col gap-1">
						<label for="c-method" class={labelClass}>{t('visa_timelines.application_method', 'Application method')}</label>
						<select id="c-method" bind:value={cMethod} class={inputClass}>
							{#each applicationMethods as m (m)}
								<option value={m}>{m.replace('_', ' ')}</option>
							{/each}
						</select>
					</div>
					<div class="flex flex-col gap-1">
						<label for="c-outcome" class={labelClass}>{t('visa_timelines.outcome', 'Outcome')}</label>
						<select id="c-outcome" bind:value={cOutcome} class={inputClass}>
							{#each outcomes as o (o)}
								<option value={o}>{o}</option>
							{/each}
						</select>
					</div>
				</div>
				<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						checked={cShared}
						onchange={(e) => (cShared = e.currentTarget.checked)}
						class="h-4 w-4 rounded border-[var(--color-border)]"
					/>
					{t('visa_timelines.share_anonymously', 'Share my timeline anonymously in community stats')}
				</label>
				{#if createError}
					<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">
						{createError}
					</p>
				{/if}
				<div class="flex justify-end">
					<button
						type="submit"
						disabled={creating}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if creating}{t('common.submitting', 'Submitting…')}{:else}{t('visa_timelines.create', 'Create')}{/if}
					</button>
				</div>
			</form>
		{/if}

		{#if timelines.length === 0 && !loadError}
			<EmptyState
				icon={CalendarClock}
				title={t('visa_timelines.no_timelines', 'No timelines yet')}
				body={t('visa_timelines.no_timelines_body', 'Add your visa application timeline to track milestones and contribute to community stats.')}
				action={{ label: t('visa_timelines.add_timeline', 'Add Timeline'), onclick: () => (showCreate = true) }}
			/>
		{:else}
			<div class="space-y-3">
				{#each timelines as tl (tl.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<div class="flex flex-wrap items-center gap-2">
									<span class="text-sm font-semibold text-[var(--color-text)]">
										{spaceName(tl.destination_space_id)}
									</span>
									<span class="text-xs text-[var(--color-muted)]">← {spaceName(tl.origin_space_id)}</span>
									<span class="rounded-full px-2 py-0.5 text-xs font-medium capitalize {outcomeColor(tl.outcome as VisaTimelineOutcome)}">
										{tl.outcome}
									</span>
								</div>
								<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
									{tl.visa_category}{#if tl.consulate} · {tl.consulate}{/if} · {(tl.application_method ?? '').replace('_', ' ')}
								</p>
							</div>
							<button
								type="button"
								onclick={() => toggleShared(tl)}
								disabled={togglingId === tl.id}
								class="shrink-0 rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs transition {tl.is_shared
									? 'border-[var(--color-success)]/40 text-[var(--color-success)]'
									: 'text-[var(--color-muted)]'}"
								title={tl.is_shared
									? t('visa_timelines.shared', 'Shared in community stats')
									: t('visa_timelines.private', 'Private')}
							>
								{#if tl.is_shared}
									<Check size={12} class="mr-1 inline" />{t('visa_timelines.shared', 'Shared')}
								{:else}
									{t('visa_timelines.private', 'Private')}
								{/if}
							</button>
						</div>

						<!-- Milestones section -->
						<div class="mt-3 border-t border-[var(--color-border)] pt-3">
							<button
								type="button"
								onclick={() => toggleExpand(tl.id)}
								class="flex w-full items-center justify-between text-xs font-medium text-[var(--color-text)]"
							>
								<span>
									{t('visa_timelines.milestones', 'Milestones')} ({tl.milestones?.length ?? 0})
								</span>
								{#if expandedId === tl.id}
									<ChevronDown size={14} />
								{:else}
									<ChevronRight size={14} />
								{/if}
							</button>

							{#if expandedId === tl.id}
								<div class="mt-3 space-y-2">
									{#each tl.milestones ?? [] as m (m.id)}
										<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3">
											{#if editingMilestone === m.id}
												<form onsubmit={(e) => saveEditMilestone(tl, m, e)} class="space-y-2" novalidate>
													<div class="flex items-center gap-2">
														<span class="text-xs font-medium capitalize text-[var(--color-text)]">{m.milestone_type.replace('_', ' ')}</span>
													</div>
													<input type="date" bind:value={editForm.occurred_at} class={inputClass} />
													<textarea
														rows={2}
														bind:value={editForm.notes}
														class={inputClass}
														placeholder={t('visa_timelines.notes_placeholder', 'Notes…')}
													></textarea>
													{#if editError}
														<p class="text-xs text-[var(--color-danger)]">{editError}</p>
													{/if}
													<div class="flex items-center gap-2">
														<button
															type="submit"
															disabled={editSaving}
															class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-[var(--color-primary-contrast)] disabled:opacity-50"
														>
															{t('common.submitting', 'Saving…') === 'Submitting…' && !editSaving ? t('common.save', 'Save') : t('common.submitting', 'Saving…')}
														</button>
														<button
															type="button"
															onclick={cancelEditMilestone}
															class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text-subtle)]"
														>
															{t('common.cancel', 'Cancel')}
														</button>
													</div>
												</form>
											{:else}
												<div class="flex items-start justify-between gap-2">
													<div class="min-w-0 flex-1">
														<div class="flex items-center gap-2">
															<span class="text-xs font-medium capitalize text-[var(--color-text)]">
																{m.milestone_type.replace('_', ' ')}
															</span>
															<span class="text-xs text-[var(--color-muted)]">{fmtDate(m.occurred_at)}</span>
														</div>
														{#if m.notes}
															<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{m.notes}</p>
														{/if}
													</div>
													<button
														type="button"
														onclick={() => startEditMilestone(m)}
														class="shrink-0 text-[var(--color-muted)] hover:text-[var(--color-text)]"
														aria-label={t('comments.edit', 'Edit')}
													>
														<Pencil size={13} />
													</button>
												</div>
											{/if}
										</div>
									{/each}

									{#if (tl.milestones?.length ?? 0) === 0}
										<p class="text-xs text-[var(--color-muted)]">
											{t('visa_timelines.no_milestones', 'No milestones yet. Add one below.')}
										</p>
									{/if}

									<!-- Add milestone form -->
									{#if milestoneForms[tl.id]}
										<form
											onsubmit={(e) => addMilestone(tl, e)}
											class="space-y-2 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] p-3"
											novalidate
										>
											<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
												<div class="flex flex-col gap-1">
													<label for="m-type-{tl.id}" class={labelClass}>
														{t('visa_timelines.milestone_type', 'Milestone type')}
													</label>
													<select
														id="m-type-{tl.id}"
														bind:value={milestoneForms[tl.id].type}
														class={inputClass}
													>
														{#each milestoneTypes as mt (mt)}
															<option value={mt}>{mt.replace('_', ' ')}</option>
														{/each}
													</select>
												</div>
												<div class="flex flex-col gap-1">
													<label for="m-date-{tl.id}" class={labelClass}>
														{t('visa_timelines.occurred_on', 'Occurred on')}
													</label>
													<input
														id="m-date-{tl.id}"
														type="date"
														bind:value={milestoneForms[tl.id].occurred_at}
														class={inputClass}
													/>
												</div>
											</div>
											<div class="flex flex-col gap-1">
												<label for="m-notes-{tl.id}" class={labelClass}>
													{t('visa_timelines.notes', 'Notes')}
												</label>
												<textarea
													id="m-notes-{tl.id}"
													rows={2}
													bind:value={milestoneForms[tl.id].notes}
													class={inputClass}
													placeholder={t('visa_timelines.notes_placeholder', 'Notes…')}
												></textarea>
											</div>
											{#if milestoneError[tl.id]}
												<p class="text-xs text-[var(--color-danger)]">{milestoneError[tl.id]}</p>
											{/if}
											<div class="flex justify-end">
												<button
													type="submit"
													disabled={milestoneSaving === tl.id}
													class="inline-flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] disabled:opacity-50"
												>
													<Plus size={12} />
													{#if milestoneSaving === tl.id}
														{t('common.submitting', 'Adding…')}
													{:else}
														{t('visa_timelines.add_milestone', 'Add milestone')}
													{/if}
												</button>
											</div>
										</form>
									{/if}
								</div>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
