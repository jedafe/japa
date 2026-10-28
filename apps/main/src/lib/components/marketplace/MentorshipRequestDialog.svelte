<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// Mentorship request dialog (000824 unification) — the "Request
	// mentorship" CTA on a listing_kind='mentorship' detail page. Submits a
	// service booking with intent='inquiry', the mentee's goals as note, and
	// an optional destination space (POST /services/{id}/bookings). Dedupe,
	// capacity and self-mentorship are enforced server-side and surface here
	// as typed error codes.
	//
	// Ref: docs/superpowers/plans/2026-10-09-services-mentorship-unification.md
	//
	// Form shape + destination picker reuse the (app)/mentors/[username]
	// request flow; the Modal/idempotency/CSRF pattern mirrors BookingDialog
	// (one Idempotency-Key per dialog open — Rule 9).
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Modal } from '$lib/components/ui';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { CheckCircle2, GraduationCap, Loader2, Send } from 'lucide-svelte';
	import type { GeoCountry, ServiceBookingResult, ServiceListing } from '$lib/types';

	interface Props {
		service: ServiceListing;
		authed: boolean;
		/** Prefill for the goals field — set when a tier card's "Choose"
		 *  opens the dialog (§114 2026-10-10: mentorship tier CTA). */
		initialGoals?: string;
		open?: boolean;
	}
	let { service, authed, initialGoals = '', open = $bindable(false) }: Props = $props();

	const mentor = $derived(service.mentor ?? null);
	const mentorName = $derived(mentor?.display_name || mentor?.username || service.title);
	const loginHref = $derived(
		`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`
	);

	let goals = $state('');
	let destinationSpaceId = $state('');
	let submitting = $state(false);
	let submitOk = $state(false);
	let submitError = $state('');
	let idemKey = $state('');

	// Destination spaces — GET /geo/countries lazily on first open, then
	// filtered client-side like the mentors directory picker.
	let countries = $state<GeoCountry[] | null>(null);
	let countriesBusy = $state(false);

	// The destination picker is scoped to the mentor's "Countries Covered"
	// (settings/mentorship → mentor_countries on the listing's mentor
	// block). A mentor who hasn't filled it falls back to the full list so
	// the field never dead-ends.
	const mentorCountries = $derived.by(() => {
		const covered = (mentor?.countries ?? []).map((c) => c.toLowerCase());
		const all = countries ?? [];
		return covered.length > 0
			? all.filter((c) => covered.includes(c.name.toLowerCase()))
			: all;
	});

	let prevOpen = false;
	$effect(() => {
		if (open && !prevOpen) {
			idemKey = crypto.randomUUID();
			submitError = '';
			submitOk = false;
			// Fresh form per open — a stale goals/destination draft from a
			// previous mentor must never leak into this request (same
			// contract as BookingDialog). A tier-CTA prefill fills an
			// empty box only.
			goals = '';
			destinationSpaceId = '';
			if (initialGoals) goals = initialGoals;
			if (countries === null) void loadCountries();
		}
		prevOpen = open;
	});

	async function loadCountries() {
		if (countriesBusy) return;
		countriesBusy = true;
		const res = await api.get<GeoCountry[]>('/geo/countries');
		countriesBusy = false;
		if (res.data) countries = res.data;
	}

	function fetchDestinationCountries(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const items = mentorCountries
			.filter(
				(c) =>
					c.name.toLowerCase().includes(query) || c.geo_code.toLowerCase().includes(query)
			)
			.map((c) => ({ id: c.id, label: c.name, sublabel: c.geo_code }));
		return Promise.resolve(items);
	}
	function fetchDestinationCountriesInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinationCountries('');
	}
	const selectedDestinationLabel = $derived(
		destinationSpaceId
			? (countries ?? []).find((c) => c.id === destinationSpaceId)?.name ?? null
			: null
	);

	// Typed booking errors → friendly copy. Codes are the writeServiceError
	// snake_case contract (duplicate_mentorship_request, mentor_at_capacity,
	// self_mentorship, mentorship_disabled). Unknown codes fall back to the
	// server message, then the generic mentors.request_failed line.
	function mentorErrorMessage(code: string, fallback?: string): string {
		switch (code) {
			case 'duplicate_mentorship_request':
				return t(
					'services.mentor_err_duplicate',
					'You already have an open mentorship request with this mentor.'
				);
			case 'mentor_at_capacity':
				return t(
					'services.mentor_err_full',
					'This mentor is not accepting new mentees right now.'
				);
			case 'self_mentorship':
				return t(
					'services.mentor_err_self',
					'You cannot request mentorship from yourself.'
				);
			case 'mentorship_disabled':
				return t('services.mentor_err_disabled', 'Mentorship is not available.');
			default:
				return (
					fallback ??
					t('mentors.errors.request_failed', 'Request failed. Please try again.')
				);
		}
	}

	async function submitRequest(e: SubmitEvent) {
		e.preventDefault();
		submitError = '';
		if (!goals.trim()) {
			submitError = t('mentors.errors.goals_required', 'Please describe your goals.');
			return;
		}
		submitting = true;
		const res = await api.post<ServiceBookingResult>(
			`/services/${service.id}/bookings`,
			{
				intent: 'inquiry',
				// goals rides both surfaces server-side — the note column and
				// meta->>'goals' (the structured read the booking DTO prefers).
				note: goals.trim(),
				goals: goals.trim(),
				...(destinationSpaceId ? { destination_space_id: destinationSpaceId } : {})
			},
			{ 'Idempotency-Key': idemKey || crypto.randomUUID() }
		);
		submitting = false;
		if (res.data) {
			submitOk = true;
			idemKey = crypto.randomUUID();
			goals = '';
			destinationSpaceId = '';
		} else {
			submitError = mentorErrorMessage(
				res.error?.code ?? '',
				res.error?.message || undefined
			);
		}
	}
</script>

<Modal bind:open title={t('mentors.request_heading', 'Request Mentorship')} size="md">
	{#if !authed}
		<!-- Guests get a login prompt — mentorship requests require an account. -->
		<p class="text-sm text-[var(--color-muted)]">
			<a href={loginHref} class="text-[var(--color-primary)] hover:underline">
				{t('services.mentor_login', 'Log in to request mentorship')}
			</a>
		</p>
	{:else if submitOk}
		<div
			class="flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-success)]/30 bg-[var(--color-success)]/10 px-4 py-3 text-sm text-[var(--color-success)]"
			role="status"
		>
			<CheckCircle2 size={18} class="mt-0.5 shrink-0" aria-hidden="true" />
			<div>
				<p class="font-medium">{t('mentors.request_sent', 'Request sent!')}</p>
				<p class="mt-1 text-[var(--color-text-subtle)]">
					{t(
						'services.mentor_sent_body',
						'{name} will respond in your mentorship inbox.',
						{ name: mentorName }
					)}
				</p>
			</div>
		</div>
		<div class="mt-4 flex flex-wrap items-center justify-end gap-2">
			<a
				href="/messages/mentorship"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('services.mentor_view_inbox', 'Open mentorship inbox')}
			</a>
			<button
				type="button"
				onclick={() => (open = false)}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
			>
				{t('common.close', 'Close')}
			</button>
		</div>
	{:else}
		<div class="mb-4 flex items-center gap-3">
			{#if mentor?.avatar_url}
				<img
					src={mentor.avatar_url}
					alt=""
					class="h-11 w-11 rounded-full object-cover"
				/>
			{:else}
				<div
					class="grid h-11 w-11 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]"
				>
					<GraduationCap size={20} aria-hidden="true" />
				</div>
			{/if}
			<div class="min-w-0">
				<a
					href="/u/{mentor?.username ?? service.author_name}"
					class="block truncate text-sm font-semibold text-[var(--color-text)] hover:text-[var(--color-primary)] hover:underline"
				>
					{mentorName}
				</a>
				<p class="text-xs text-[var(--color-muted)]">
					{t('mentors.request_body', 'Share your goals so your mentor can help you plan.')}
				</p>
			</div>
		</div>

		<form onsubmit={submitRequest} class="space-y-3" novalidate>
			<div class="flex flex-col gap-1.5">
				<label for="mreq-goals" class="text-sm font-medium text-[var(--color-text)]">
					{t('mentors.field_goals', 'Your goals')}
				</label>
				<textarea
					id="mreq-goals"
					rows={4}
					bind:value={goals}
					placeholder={t('mentors.field_goals_placeholder', 'What do you hope to achieve?')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				></textarea>
					<HintIcon text={t('mentors.goals_hint', 'What you want to get out of the mentorship — be specific.')} />
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="mreq-dest" class="text-sm font-medium text-[var(--color-text)]">
					{t('services.mentor_destination', 'Your Destination')}
				</label>
				<Autocomplete
					id="mreq-dest"
					placeholder={t('common.select', 'Select...')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={fetchDestinationCountries}
					initialFetcher={fetchDestinationCountriesInitial}
					selectedId={destinationSpaceId || null}
					selectedLabel={selectedDestinationLabel}
					onSelect={(item) => (destinationSpaceId = item.id)}
					onClear={() => (destinationSpaceId = '')}
				/>
			</div>

			{#if submitError}
				<p
					class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500"
					role="alert"
				>
					{submitError}
				</p>
			{/if}

			<button
				type="submit"
				disabled={submitting}
				class="inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{#if submitting}
					<Loader2 size={16} class="motion-safe:animate-spin" aria-hidden="true" />
				{:else}
					<Send class="rtl:-scale-x-100" size={16} aria-hidden="true" />
				{/if}
				{t('mentors.send_request', 'Send request')}
			</button>
		</form>
	{/if}
</Modal>
