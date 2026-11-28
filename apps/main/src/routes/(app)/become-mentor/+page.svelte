<script lang="ts">
	/**
	 * Become-a-mentor wizard — §102 2026-11-28 member mentor applications.
	 *
	 * Branches on the GET /me/mentor-application probe:
	 *  - is_mentor            → done card (settings + listing links)
	 *  - eligible             → quick-enable form → PATCH /me/mentor-profile
	 *  - application pending  → review status card
	 *  - application declined → reason + resubmit form
	 *  - otherwise            → application form → POST /me/mentor-application
	 *
	 * Ref: japaconnect-engineering/sections/102-mentorship-matching.md
	 */
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { t } from '#lib/i18n/index.js';
	import { api } from '#lib/api/client.js';
	import { ArrowLeft, GraduationCap, Loader2, Check, CircleCheck, Clock3, TriangleAlert, AlertCircle } from 'lucide-svelte';
	import type { MentorApplication, MentorProfile, MyMentorApplicationStatus } from '#lib/types/index.js';

	let { data }: { data: { status: MyMentorApplicationStatus | null; profile: MentorProfile | null } } =
		$props();

	// Local copy of the probe — mutating it on submit lets the card swap
	// without a full invalidate; invalidateAll still runs to reseed.
	let status = $state<MyMentorApplicationStatus | null>(untrack(() => data.status));
	let profile = $derived(data.profile);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
	const primaryBtn =
		'inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50';
	const ghostBtn =
		'inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]';

	// Derived branch — order matters: an active mentor never sees forms.
	// Successful PATCH/POST mutate `status` below, so the branch swaps on
	// its own (PATCH → is_mentor=true → done; POST → application=pending).
	type WizardBranch = 'done' | 'setup' | 'pending' | 'approved' | 'declined' | 'apply' | 'unavailable';
	const branch = $derived<WizardBranch>(
		status == null
			? 'unavailable'
			: status.is_mentor
				? 'done'
				: status.eligible
					? 'setup'
					: status.application?.status === 'pending'
						? 'pending'
						: status.application?.status === 'approved'
							? 'approved'
							: status.application?.status === 'declined'
								? 'declined'
								: 'apply'
	);

	// Quick-enable form state (eligible path) — seeded from the profile.
	let capacity = $state(untrack(() => data.profile?.mentor_capacity ?? Math.min(5, data.profile?.mentor_max_capacity ?? 10)));
	let areas = $state(untrack(() => (data.profile?.mentor_areas ?? []).join(', ')));
	let bio = $state(untrack(() => data.profile?.mentor_bio ?? ''));
	const capacityMax = $derived(profile?.mentor_max_capacity ?? 10);

	// Application form state (member path) — resubmission pre-fills.
	const application = $derived<MentorApplication | null>(status?.application ?? null);
	let motivation = $state(untrack(() => data.status?.application?.motivation ?? ''));
	let applyAreas = $state(untrack(() => (data.status?.application?.mentor_areas ?? []).join(', ')));

	let saving = $state(false);
	let error: string | null = $state(null);

	function csvToArray(value: string): string[] {
		return value
			.split(',')
			.map((a) => a.trim())
			.filter(Boolean);
	}

	/** Eligible path: one PATCH flips is_mentor and stores the profile. */
	async function enableMentor() {
		saving = true;
		error = null;
		const res = await api.patch<MentorProfile>('/me/mentor-profile', {
			is_mentor: true,
			mentor_capacity: Number(capacity),
			mentor_areas: csvToArray(areas),
			mentor_bio: bio.trim() || null
		});
		saving = false;
		if (res.error) {
			error = res.error.message || t('become_mentor.error_enable', 'Could not enable your mentor profile.');
			return;
		}
		if (status) status = { ...status, is_mentor: true };
		await invalidateAll();
	}

	/** Member path: submit the application for admin review. */
	async function submitApplication() {
		saving = true;
		error = null;
		const res = await api.post<MentorApplication>('/me/mentor-application', {
			motivation: motivation.trim(),
			mentor_areas: csvToArray(applyAreas)
		});
		saving = false;
		if (res.error) {
			error = res.error.message || t('become_mentor.error_apply', 'Could not submit your application.');
			return;
		}
		if (status && res.data) {
			status = { ...status, application: res.data };
		}
		await invalidateAll();
	}

	/** Approved-but-permission-not-yet-visible arm — the perm cache can lag
	 *  a few seconds, so this just re-fetches the probe. */
	let refreshing = $state(false);
	async function refreshStatus() {
		refreshing = true;
		await invalidateAll();
		const res = await api.get<MyMentorApplicationStatus>('/me/mentor-application');
		refreshing = false;
		if (res.data) status = res.data;
	}

	// Approved row whose grant is gone (out-of-band removal or a failed
	// grant) — the member drops into the resubmit form instead of being
	// stuck on the approved card. The POST accepts approved rows.
	let resubmitting = $state(false);
</script>

<SeoHead
	title={t('become_mentor.page_title', 'Become a Mentor — {site}')}
	robots="noindex, nofollow"
	description={t('become_mentor.page_description', 'Share your relocation experience as a mentor.')}
/>

<div class="mx-auto w-full max-w-full">
	<div class="mx-auto flex max-w-2xl items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/services?kind=mentorship"
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('become_mentor.back', 'Back to mentors')}
		>
			<ArrowLeft class="rtl:-scale-x-100" size={18} />
		</a>
		<div class="flex min-w-0 items-center gap-2">
			<GraduationCap size={20} class="shrink-0 text-[var(--color-primary-text)]" />
			<div class="min-w-0">
				<h1 class="text-lg font-semibold text-[var(--color-text)]">
					{t('become_mentor.title', 'Become a mentor')}
				</h1>
				<p class="truncate text-sm text-[var(--color-text-subtle)]">
					{t('become_mentor.subtitle', 'Guide fellow diaspora members through the journey you already made.')}
				</p>
			</div>
		</div>
	</div>

	<div class="mx-auto max-w-2xl p-4">

	{#if branch === 'done'}
		<div class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
			<div class="flex items-start gap-3">
				<CircleCheck size={22} class="mt-0.5 shrink-0 text-[var(--color-success)]" />
				<div>
					<h2 class="text-base font-semibold text-[var(--color-text)]">
						{t('become_mentor.done_title', 'You are a mentor')}
					</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
						{t(
							'become_mentor.done_body',
							'Your mentor profile is live — members can find you in the directory and send you mentorship requests.'
						)}
					</p>
				</div>
			</div>
			<div class="flex flex-wrap gap-2">
				<a href="/settings/mentorship" class={primaryBtn}>
					{t('become_mentor.edit_profile', 'Edit mentor profile')}
				</a>
				<a href="/services?kind=mentorship" class={ghostBtn}>
					{t('become_mentor.view_directory', 'View mentor directory')}
				</a>
				<a href="/messages/mentorship" class={ghostBtn}>
					{t('become_mentor.view_requests', 'View requests')}
				</a>
			</div>
		</div>
	{:else if branch === 'unavailable'}
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
			<div class="flex items-start gap-3">
				<TriangleAlert size={22} class="mt-0.5 shrink-0 text-[var(--color-muted)]" />
				<p class="text-sm text-[var(--color-text-subtle)]">
					{t(
						'become_mentor.unavailable',
						'Mentor applications are not available for your account. Contact an administrator if you believe this is an error.'
					)}
				</p>
			</div>
		</div>
	{:else if branch === 'pending'}
		<div class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
			<div class="flex items-start gap-3">
				<Clock3 size={22} class="mt-0.5 shrink-0 text-[var(--color-primary-text)]" />
				<div>
					<h2 class="text-base font-semibold text-[var(--color-text)]">
						{t('become_mentor.pending_title', 'Application under review')}
					</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
						{t(
							'become_mentor.pending_body',
							'An administrator will review your application — you will get a notification when a decision is made.'
						)}
					</p>
				</div>
			</div>
			{#if application}
				<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-4 text-sm">
					<p class="font-medium text-[var(--color-text)]">
						{t('become_mentor.pending_submitted', 'Submitted {date}', {
							date: new Date(application.created_at).toLocaleDateString()
						})}
					</p>
					{#if application.mentor_areas.length > 0}
						<p class="mt-1 text-[var(--color-text-subtle)]">
							{t('become_mentor.pending_areas', 'Areas: {areas}', {
								areas: application.mentor_areas.join(', ')
							})}
						</p>
					{/if}
				</div>
			{/if}
			<a href="/services?kind=mentorship" class={ghostBtn}>
				{t('become_mentor.browse_mentors', 'Browse mentors meanwhile')}
			</a>
		</div>
	{:else if branch === 'approved' && !resubmitting}
		<div class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
			<div class="flex items-start gap-3">
				<CircleCheck size={22} class="mt-0.5 shrink-0 text-[var(--color-success)]" />
				<div>
					<h2 class="text-base font-semibold text-[var(--color-text)]">
						{t('become_mentor.approved_title', 'Your application was approved')}
					</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
						{t('become_mentor.approved_body', 'Set up your mentor profile to start receiving requests.')}
					</p>
				</div>
			</div>
			<div class="flex flex-wrap gap-2">
				<button type="button" class={primaryBtn} onclick={refreshStatus} disabled={refreshing}>
					{#if refreshing}<Loader2 size={16} class="animate-spin" />{/if}
					{t('become_mentor.approved_continue', 'Continue to setup')}
				</button>
				<!-- Approved-but-not-eligible: grant missing (out-of-band
				     removal or a failed grant). The resubmit form is the
				     supported recovery path. -->
				{#if !status?.eligible}
					<button type="button" class={ghostBtn} onclick={() => (resubmitting = true)}>
						{t('become_mentor.approved_resubmit', 'Resubmit for review')}
					</button>
				{/if}
			</div>
		</div>
	{:else if branch === 'setup'}
		<!-- Eligible fast path — holds mentorship.become_mentor already -->
		<form
			onsubmit={(e) => {
				e.preventDefault();
				enableMentor();
			}}
			class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6"
			novalidate
		>
			<h2 class="text-base font-semibold text-[var(--color-text)]">
				{t('become_mentor.setup_title', 'Set up your mentor profile')}
			</h2>
			<p class="text-sm text-[var(--color-text-subtle)]">
				{t('become_mentor.setup_body', 'Tell members what you can help with — you can refine the public card and page content later in mentor settings.')}
			</p>

			<div class="flex flex-col gap-1.5">
				<label for="bm-areas" class="text-sm font-medium text-[var(--color-text)]">
					{t('become_mentor.areas_label', 'Areas of expertise')}
				</label>
				<input
					id="bm-areas"
					type="text"
					bind:value={areas}
					placeholder={t('become_mentor.areas_placeholder', 'e.g. visa, housing, career')}
					class={inputClass}
				/>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t('become_mentor.areas_help', 'Comma-separated keywords used for matching.')}
				</p>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="bm-bio" class="text-sm font-medium text-[var(--color-text)]">
					{t('become_mentor.bio_label', 'Mentor bio')}
				</label>
				<textarea
					id="bm-bio"
					bind:value={bio}
					rows="4"
					maxlength="1000"
					placeholder={t('become_mentor.bio_placeholder', 'Share your background and what you can help with.')}
					class="{inputClass} resize-y"
				></textarea>
				<p class="text-xs text-[var(--color-text-subtle)]">{bio.length}/1000</p>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="bm-capacity" class="text-sm font-medium text-[var(--color-text)]">
					{t('become_mentor.capacity_label', 'Mentee capacity')}
				</label>
				<input
					id="bm-capacity"
					type="number"
					min="1"
					max={capacityMax}
					bind:value={capacity}
					class="w-24 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t('become_mentor.capacity_help', 'Maximum number of active mentees. Platform cap: {max}.', {
						max: capacityMax
					})}
				</p>
			</div>

			{#if error}
				<div class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					<AlertCircle size={16} class="shrink-0" /> {error}
				</div>
			{/if}

			<div class="flex justify-end">
				<button type="submit" class={primaryBtn} disabled={saving || Number(capacity) < 1}>
					{#if saving}<Loader2 size={16} class="animate-spin" />{:else}<Check size={16} />{/if}
					{t('become_mentor.enable_cta', 'Start mentoring')}
				</button>
			</div>
		</form>
	{:else}
		<!-- Member path — application for admin review (apply + declined-
		     resubmit + approved-but-grantless resubmit) -->
		<form
			onsubmit={(e) => {
				e.preventDefault();
				submitApplication();
			}}
			class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6"
			novalidate
		>
			{#if branch === 'approved'}
				<div class="rounded-[var(--radius)] border border-[var(--color-warning)]/30 bg-[var(--color-warning)]/10 p-4">
					<p class="text-sm font-medium text-[var(--color-text)]">
						{t('become_mentor.resubmit_hint_title', 'Your mentor access is no longer active')}
					</p>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
						{t(
							'become_mentor.resubmit_hint_body',
							'Your grant was removed — update your application and resubmit it for a fresh review.'
						)}
					</p>
				</div>
			{:else if branch === 'declined' && application}
				<div class="rounded-[var(--radius)] border border-[var(--color-danger)]/30 bg-[var(--color-danger)]/10 p-4">
					<p class="text-sm font-medium text-[var(--color-danger)]">
						{t('become_mentor.declined_title', 'Your previous application was declined')}
					</p>
					{#if application.decline_reason}
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
							{application.decline_reason}
						</p>
					{/if}
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
						{t('become_mentor.declined_body', 'You can update your application and submit it again below.')}
					</p>
				</div>
			{/if}

			<div>
				<h2 class="text-base font-semibold text-[var(--color-text)]">
					{t('become_mentor.apply_title', 'Apply to become a mentor')}
				</h2>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
					{t(
						'become_mentor.apply_body',
						'Mentor status on JapaConnect is granted after a quick review — tell us what you can help members with.'
					)}
				</p>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="bm-motivation" class="text-sm font-medium text-[var(--color-text)]">
					{t('become_mentor.motivation_label', 'Why do you want to mentor?')}
					<span class="text-red-500" aria-hidden="true">*</span>
				</label>
				<textarea
					id="bm-motivation"
					bind:value={motivation}
					rows="5"
					maxlength="2000"
					required
					placeholder={t(
						'become_mentor.motivation_placeholder',
						'Share your relocation experience, what you learned, and who you can help.'
					)}
					class="{inputClass} resize-y"
				></textarea>
				<p class="text-xs text-[var(--color-text-subtle)]">{motivation.length}/2000</p>
			</div>

			<div class="flex flex-col gap-1.5">
				<label for="bm-apply-areas" class="text-sm font-medium text-[var(--color-text)]">
					{t('become_mentor.apply_areas_label', 'Areas you can mentor in')}
				</label>
				<input
					id="bm-apply-areas"
					type="text"
					bind:value={applyAreas}
					placeholder={t('become_mentor.areas_placeholder', 'e.g. visa, housing, career')}
					class={inputClass}
				/>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t('become_mentor.apply_areas_help', 'Comma-separated — helps the reviewer route your application.')}
				</p>
			</div>

			{#if error}
				<div class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					<AlertCircle size={16} class="shrink-0" /> {error}
				</div>
			{/if}

			<div class="flex justify-end">
				<button type="submit" class={primaryBtn} disabled={saving || !motivation.trim()}>
					{#if saving}<Loader2 size={16} class="animate-spin" />{:else}<Check size={16} />{/if}
					{branch === 'declined' || branch === 'approved'
						? t('become_mentor.resubmit_cta', 'Resubmit application')
						: t('become_mentor.apply_cta', 'Submit application')}
				</button>
			</div>
		</form>
	{/if}
	</div>
</div>
