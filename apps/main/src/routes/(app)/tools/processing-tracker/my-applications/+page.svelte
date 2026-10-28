<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { t, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { EmptyState, AutocompleteSelect } from '$lib/components/ui';
	import DestinationSelector from '$lib/components/tools/DestinationSelector.svelte';
	import { Timer, ArrowLeft, Plus, ChevronUp, ChevronDown, Pencil } from 'lucide-svelte';
	import type {
		VisaTimeline,
		VisaTimelineMilestone,
		GeoSpaceOption,
		CatalogOption
	} from '$lib/types';
	import type { PageData } from './$types';

	/**
	 * My Applications — the authenticated personal tracker (§83, §93).
	 * Ported from the removed tools surface (ADR-086 superseded). Users log
	 * milestone dates on their live visa applications and can share the
	 * timeline (anonymised) into the community aggregate.
	 */
	let { data }: { data: PageData } = $props();

	let timelines = $state<VisaTimeline[]>(untrack(() => data.timelines));
	let destinations = $state<GeoSpaceOption[]>(untrack(() => data.destinations));
	let milestoneTypes = $state<CatalogOption[]>(untrack(() => data.milestoneTypeOptions));
	let outcomeOptions = $state<CatalogOption[]>(untrack(() => data.outcomeOptions));
	let methodOptions = $state<CatalogOption[]>(untrack(() => data.methodOptions));

	let error = $state<string | null>(null);

	// Create form state.
	let showCreate = $state(false);
	let creating = $state(false);
	let newDestId = $state('');
	let newVisaCategory = $state('');
	let newConsulate = $state('');
	let newMethod = $state('');
	let newShared = $state(false);

	// Per-timeline milestone form state.
	let addingTo = $state<string | null>(null);
	let milestoneType = $state('');
	let milestoneDate = $state('');
	let milestoneNotes = $state('');
	let savingMilestone = $state(false);

	// Milestone edit state.
	let editingMilestone = $state<string | null>(null);
	let editMilestoneType = $state('');
	let editMilestoneDate = $state('');
	let editMilestoneNotes = $state('');

	// Expanded timeline cards.
	let expanded = $state<Set<string>>(new Set());

	// In-flight guards for timeline-level mutations.
	let savingTimeline = $state<string | null>(null);

	function destName(id: string): string {
		return destinations.find((d) => d.id === id)?.name ?? '';
	}

	function optionLabel(options: CatalogOption[], value: string): string {
		return options.find((o) => o.value === value)?.label ?? value;
	}

	function fmtDate(iso: string): string {
		if (!iso) return '';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? iso : d.toLocaleDateString(i18n.locale, { dateStyle: 'medium' });
	}

	/** Sort a timeline's milestones chronologically. */
	function sortedMilestones(tl: VisaTimeline): VisaTimelineMilestone[] {
		return [...(tl.milestones ?? [])].sort(
			(a, b) => new Date(a.occurred_at).getTime() - new Date(b.occurred_at).getTime()
		);
	}

	function toggleExpanded(id: string) {
		const next = new Set(expanded);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		expanded = next;
	}

	async function createTimeline() {
		if (creating || !newDestId || !newVisaCategory.trim()) return;
		creating = true;
		error = null;
		const res = await api.post<VisaTimeline>('/me/visa-timelines', {
			destination_space_id: newDestId,
			visa_category: newVisaCategory.trim(),
			consulate: newConsulate.trim() || undefined,
			application_method: newMethod || undefined,
			is_shared: newShared
		});
		creating = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) {
			timelines = [res.data, ...timelines];
			expanded = new Set([...expanded, res.data.id]);
		}
		showCreate = false;
		newDestId = '';
		newVisaCategory = '';
		newConsulate = '';
		newMethod = '';
		newShared = false;
	}

	async function updateTimeline(tl: VisaTimeline, patch: { outcome?: string; is_shared?: boolean }) {
		if (savingTimeline === tl.id) return;
		savingTimeline = tl.id;
		const res = await api.patch<VisaTimeline>(`/me/visa-timelines/${tl.id}`, patch);
		savingTimeline = null;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) {
			timelines = timelines.map((x) => (x.id === tl.id ? { ...x, ...res.data } : x));
		}
	}

	async function addMilestone(tl: VisaTimeline) {
		if (savingMilestone || !milestoneType || !milestoneDate) return;
		savingMilestone = true;
		error = null;
		const res = await api.post<VisaTimelineMilestone>(`/me/visa-timelines/${tl.id}/milestones`, {
			milestone_type: milestoneType,
			occurred_at: new Date(milestoneDate).toISOString(),
			notes: milestoneNotes.trim() || undefined
		});
		savingMilestone = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		if (res.data) {
			timelines = timelines.map((x) =>
				x.id === tl.id ? { ...x, milestones: [...(x.milestones ?? []), res.data!] } : x
			);
		}
		addingTo = null;
		milestoneType = '';
		milestoneDate = '';
		milestoneNotes = '';
	}

	function startEditMilestone(m: VisaTimelineMilestone) {
		editingMilestone = m.id;
		editMilestoneType = m.milestone_type;
		editMilestoneDate = m.occurred_at.slice(0, 10);
		editMilestoneNotes = m.notes ?? '';
	}

	async function saveMilestone(tl: VisaTimeline, m: VisaTimelineMilestone) {
		if (savingMilestone) return;
		savingMilestone = true;
		error = null;
		const res = await api.patch<VisaTimelineMilestone>(
			`/me/visa-timelines/${tl.id}/milestones/${m.id}`,
			{
				milestone_type: editMilestoneType || undefined,
				occurred_at: editMilestoneDate ? new Date(editMilestoneDate).toISOString() : undefined,
				notes: editMilestoneNotes
			}
		);
		savingMilestone = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		timelines = timelines.map((x) =>
			x.id === tl.id
				? {
						...x,
						milestones: (x.milestones ?? []).map((mm) =>
							mm.id === m.id
								? {
										...mm,
										milestone_type: editMilestoneType || mm.milestone_type,
										occurred_at: editMilestoneDate
											? new Date(editMilestoneDate).toISOString()
											: mm.occurred_at,
										notes: editMilestoneNotes
									}
								: mm
						)
					}
				: x
		);
		editingMilestone = null;
	}
</script>

<svelte:head>
	<title>{t('processing_tracker.my_apps_title_short', 'My Applications')} — {t('processing_tracker.title', 'Visa Processing Time Tracker')}</title>
	<meta
		name="description"
		content={t('processing_tracker.my_apps_desc', 'Track your own visa application milestones and optionally share your timeline anonymously with the community.')}
	/>
	<meta name="robots" content="noindex,nofollow" />
</svelte:head>

<div class="mx-auto max-w-4xl px-4 py-8">
	{#if !data.authed}
		<div class="flex items-center justify-center py-16">
			<div class="text-center max-w-sm">
				<div class="flex justify-center mb-4">
					<div class="w-16 h-16 rounded-2xl bg-[var(--color-primary)] flex items-center justify-center">
						<Timer size={32} class="text-white" />
					</div>
				</div>
				<h2 class="text-xl font-semibold text-[var(--color-text)]">
					{t('processing_tracker.my_apps_title_short', 'My Applications')}
				</h2>
				<p class="mt-2 text-sm text-[var(--color-text-muted)]">
					{t('processing_tracker.my_apps_signin', 'Sign in to track your visa application milestones and share your timeline with the community.')}
				</p>
				<div class="mt-5 flex flex-col gap-2">
					<a
						href="/login?redirect={encodeURIComponent(page.url.pathname)}"
						class="px-4 py-2.5 bg-[var(--color-primary)] text-white rounded-lg text-sm font-medium hover:opacity-90 transition-opacity text-center"
					>
						{t('japabot.sign_in', 'Sign in')}
					</a>
					<a
						href="/signup"
						class="px-4 py-2.5 text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)] transition-colors text-center"
					>
						{t('common.create_account', 'Create free account')}
					</a>
				</div>
			</div>
		</div>
	{:else}
		<div class="mb-6 flex items-start justify-between gap-4">
			<div>
				<a href="/tools/processing-tracker" class="inline-flex items-center gap-1 text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text)] mb-2">
					<ArrowLeft size={14} />
					{t('processing_tracker.back', 'Processing Time Tracker')}
				</a>
				<h1 class="text-2xl sm:text-3xl font-bold text-[var(--color-text)]">
					{t('processing_tracker.my_apps_title_short', 'My Applications')}
				</h1>
				<p class="mt-2 text-[var(--color-text-muted)]">
					{t('processing_tracker.my_apps_subtitle', 'Log milestones on your live visa applications. Sharing is anonymous and helps the community.')}
				</p>
			</div>
			<button
				onclick={() => (showCreate = !showCreate)}
				class="flex-shrink-0 inline-flex items-center gap-2 px-4 py-2 bg-[var(--color-primary)] text-white rounded-lg text-sm font-medium hover:opacity-90 transition-opacity"
			>
				<Plus size={16} />
				{t('processing_tracker.new_application', 'New application')}
			</button>
		</div>

		{#if error}
			<div class="mb-4 p-3 rounded-lg bg-[var(--color-danger)]/10 border border-[var(--color-danger)]/30 text-sm text-[var(--color-danger)]">
				{error}
			</div>
		{/if}

		{#if showCreate}
			<form
				onsubmit={(e) => { e.preventDefault(); createTimeline(); }}
				class="mb-6 p-4 bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg space-y-4"
			>
				<div>
					<label class="block text-xs font-medium text-[var(--color-text-muted)] mb-1" for="new-dest">
						{t('processing_tracker.destination', 'Destination country')} *
					
					<HintIcon text={t('processing_tracker.dest_hint', 'The country you applied to.')}  />
</label>
					<DestinationSelector {destinations} bind:selectedId={newDestId} />
				</div>
				<div class="grid sm:grid-cols-2 gap-4">
					<div>
						<label class="block text-xs font-medium text-[var(--color-text-muted)] mb-1" for="new-visa">
							{t('processing_tracker.visa_category', 'Visa category')} *
						
						<HintIcon text={t('processing_tracker.visa_hint', 'The visa type — e.g. Student, Skilled Worker.')}  />
</label>
						<input
							id="new-visa"
							bind:value={newVisaCategory}
							required
							placeholder={t('processing_tracker.visa_placeholder', 'e.g. Student, Skilled Worker')}
							class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
					<div>
						<label class="block text-xs font-medium text-[var(--color-text-muted)] mb-1" for="new-consulate">
							{t('processing_tracker.consulate', 'Consulate (optional)')}
						
						<HintIcon text={t('processing_tracker.consulate_hint', 'The consulate or office handling your application.')}  />
</label>
						<input
							id="new-consulate"
							bind:value={newConsulate}
							placeholder={t('processing_tracker.consulate_placeholder', 'e.g. Lagos, Abuja')}
							class="w-full px-3 py-2 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
				</div>
				<div class="grid sm:grid-cols-2 gap-4 items-end">
					<div>
						<label class="block text-xs font-medium text-[var(--color-text-muted)] mb-1" for="new-method">
							{t('processing_tracker.method', 'Application method')}
						
						<HintIcon text={t('processing_tracker.method_hint', 'How you applied — online, in person, via agent.')}  />
</label>
						<AutocompleteSelect
							id="new-method"
							bind:value={newMethod}
							options={[
								{ value: '', label: t('common.optional', 'Optional') },
								...methodOptions
							]}
							class="w-full"
						/>
					</div>
					<label class="flex items-center gap-2 text-sm text-[var(--color-text)] pb-2">
						<input type="checkbox" bind:checked={newShared} class="rounded border-[var(--color-border)]" />
						{t('processing_tracker.share_anon', 'Share anonymously with the community')}
					</label>
				</div>
				<div class="flex gap-2">
					<button
						type="submit"
						disabled={creating || !newDestId || !newVisaCategory.trim()}
						class="px-4 py-2 bg-[var(--color-primary)] text-white rounded-lg text-sm font-medium hover:opacity-90 disabled:opacity-50 transition-opacity"
					>
						{creating ? t('common.saving', 'Saving…') : t('processing_tracker.create', 'Create application')}
					</button>
					<button
						type="button"
						onclick={() => (showCreate = false)}
						class="px-4 py-2 text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
					>
						{t('common.cancel', 'Cancel')}
					</button>
				</div>
			</form>
		{/if}

		{#if timelines.length === 0}
			<EmptyState
				icon={Timer}
				title={t('processing_tracker.no_applications', 'No applications yet')}
				body={t('processing_tracker.no_applications_desc', 'Create your first application to start logging milestones.')}
			/>
		{:else}
			<div class="space-y-4">
				{#each timelines as tl (tl.id)}
					<div class="bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg overflow-hidden">
						<button
							onclick={() => toggleExpanded(tl.id)}
							class="w-full flex items-center justify-between gap-3 p-4 text-start hover:bg-[var(--color-surface-elevated)] transition-colors"
						>
							<div class="min-w-0">
								<h3 class="text-sm font-semibold text-[var(--color-text)] truncate">
									{tl.visa_category} — {destName(tl.destination_space_id) || tl.destination_space_id}
								</h3>
								<p class="mt-0.5 text-xs text-[var(--color-text-muted)]">
									{tl.consulate ? `${tl.consulate} · ` : ''}{(tl.milestones ?? []).length}
									{t('processing_tracker.milestones', 'milestones')}
								</p>
							</div>
							<div class="flex items-center gap-2 flex-shrink-0">
								{#if tl.outcome}
									<span class="px-2 py-0.5 rounded-full text-xs font-medium {tl.outcome === 'approved' ? 'bg-[var(--color-success)]/10 text-[var(--color-success)]' : tl.outcome === 'refused' ? 'bg-[var(--color-danger)]/10 text-[var(--color-danger)]' : 'bg-[var(--color-surface-elevated)] text-[var(--color-text-muted)]'}">
										{optionLabel(outcomeOptions, tl.outcome)}
									</span>
								{/if}
								{#if tl.is_shared}
									<span class="px-2 py-0.5 rounded-full text-xs font-medium bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
										{t('processing_tracker.shared', 'Shared')}
									</span>
								{/if}
								{#if expanded.has(tl.id)}
									<ChevronUp size={16} class="text-[var(--color-text-muted)]" />
								{:else}
									<ChevronDown size={16} class="text-[var(--color-text-muted)]" />
								{/if}
							</div>
						</button>

						{#if expanded.has(tl.id)}
							<div class="px-4 pb-4 border-t border-[var(--color-border)] pt-4 space-y-4">
								<div class="flex flex-wrap items-center gap-3">
									<AutocompleteSelect
										value={tl.outcome ?? 'pending'}
										onValueChange={(v) => updateTimeline(tl, { outcome: v })}
										disabled={savingTimeline === tl.id}
										options={outcomeOptions}
										class="w-36"
									/>
									<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
										<input
											type="checkbox"
											checked={tl.is_shared}
											onchange={(e) => updateTimeline(tl, { is_shared: e.currentTarget.checked })}
											disabled={savingTimeline === tl.id}
											class="rounded border-[var(--color-border)]"
										/>
										{t('processing_tracker.share_anon', 'Share anonymously')}
									</label>
								</div>

								<div>
									<h4 class="text-xs font-medium uppercase tracking-wider text-[var(--color-text-muted)] mb-2">
										{t('processing_tracker.milestones', 'Milestones')}
									</h4>
									{#if sortedMilestones(tl).length === 0}
										<p class="text-sm text-[var(--color-text-muted)]">{t('processing_tracker.no_milestones', 'No milestones logged yet.')}</p>
									{:else}
										<div class="space-y-2">
											{#each sortedMilestones(tl) as m (m.id)}
												{#if editingMilestone === m.id}
													<div class="p-3 bg-[var(--color-surface-elevated)] rounded-lg space-y-2">
														<div class="grid sm:grid-cols-2 gap-2">
															<AutocompleteSelect
																bind:value={editMilestoneType}
																options={milestoneTypes}
																class="w-48"
															/>
															<input
																type="date"
																bind:value={editMilestoneDate}
																class="px-3 py-1.5 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg text-[var(--color-text)]"
															/>
														</div>
														<input
															bind:value={editMilestoneNotes}
															placeholder={t('processing_tracker.notes_placeholder', 'Notes (optional)')}
															class="w-full px-3 py-1.5 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg text-[var(--color-text)]"
														/>
														<div class="flex gap-2">
															<button
																onclick={() => saveMilestone(tl, m)}
																disabled={savingMilestone}
																class="px-3 py-1 text-xs font-medium bg-[var(--color-primary)] text-white rounded-md hover:opacity-90 disabled:opacity-50"
															>
																{t('common.save', 'Save')}
															</button>
															<button
																onclick={() => (editingMilestone = null)}
																class="px-3 py-1 text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
															>
																{t('common.cancel', 'Cancel')}
															</button>
														</div>
													</div>
												{:else}
													<div class="flex items-start gap-3 p-2 rounded-lg hover:bg-[var(--color-surface-elevated)] group">
														<div class="flex-shrink-0 w-2 h-2 rounded-full bg-[var(--color-primary)] mt-1.5"></div>
														<div class="flex-1 min-w-0">
															<p class="text-sm font-medium text-[var(--color-text)]">
																{optionLabel(milestoneTypes, m.milestone_type)}
																<span class="text-[var(--color-text-muted)] font-normal">· {fmtDate(m.occurred_at)}</span>
															</p>
															{#if m.notes}
																<p class="text-xs text-[var(--color-text-muted)] mt-0.5">{m.notes}</p>
															{/if}
														</div>
														<button
															onclick={() => startEditMilestone(m)}
															class="flex-shrink-0 p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text)] opacity-0 group-hover:opacity-100 transition-opacity"
															aria-label={t('common.edit', 'Edit')}
														>
															<Pencil size={14} />
														</button>
													</div>
												{/if}
											{/each}
										</div>
									{/if}

									{#if addingTo === tl.id}
										<div class="mt-3 p-3 bg-[var(--color-surface-elevated)] rounded-lg space-y-2">
											<div class="grid sm:grid-cols-2 gap-2">
												<AutocompleteSelect
													bind:value={milestoneType}
													options={[
														{ value: '', label: t('processing_tracker.milestone_type', 'Milestone type…') },
														...milestoneTypes
													]}
													class="w-48"
												/>
												<input
													type="date"
													bind:value={milestoneDate}
													class="px-3 py-1.5 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg text-[var(--color-text)]"
												/>
											</div>
											<input
												bind:value={milestoneNotes}
												placeholder={t('processing_tracker.notes_placeholder', 'Notes (optional)')}
												class="w-full px-3 py-1.5 text-sm bg-[var(--color-surface)] border border-[var(--color-border)] rounded-lg text-[var(--color-text)]"
											/>
											<div class="flex gap-2">
												<button
													onclick={() => addMilestone(tl)}
													disabled={savingMilestone || !milestoneType || !milestoneDate}
													class="px-3 py-1.5 text-xs font-medium bg-[var(--color-primary)] text-white rounded-md hover:opacity-90 disabled:opacity-50"
												>
													{t('processing_tracker.add_milestone', 'Add milestone')}
												</button>
												<button
													onclick={() => (addingTo = null)}
													class="px-3 py-1.5 text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
												>
													{t('common.cancel', 'Cancel')}
												</button>
											</div>
										</div>
									{:else}
										<button
											onclick={() => (addingTo = tl.id)}
											class="mt-3 inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
										>
											<Plus size={14} />
											{t('processing_tracker.add_milestone', 'Add milestone')}
										</button>
									{/if}
								</div>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>
