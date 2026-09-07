<script lang="ts">
	/**
	 * Mentor settings — §102 Mentorship Matching (ADR-110).
	 *
	 * Lets elevated-role users (mentorship.become_mentor permission) toggle
	 * their mentor status, set capacity, areas of expertise, and bio.
	 * Calls PATCH /me/mentor-profile. The backend enforces the permission
	 * and validates capacity against the system_settings
	 * `mentorship.max_capacity` cap.
	 *
	 * Ref: japaconnect-engineering/sections/102-mentorship-matching.md
	 *      japaconnect-engineering/adr/ADR-110-mentorship-matching.md
	 */
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { api } from '$lib/api/client';
	import { GraduationCap, Loader2, Check } from 'lucide-svelte';
	import type { MentorProfile } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const payload = $derived(data as { profile: MentorProfile | null } | null);
	const profile = $derived(payload?.profile ?? null);

	// §102: mentor settings are gated by the mentorship.become_mentor permission.
	// The backend returns 403 when the user lacks it; the loader catches that
	// and returns null. We also check the feature flag client-side.
	const mentorshipEnabled = $derived(page.data.features?.['mentorship.enabled'] === true);

	// Platform cap for the capacity input, read from the server-provided profile.
	const capacityMax = $derived(profile?.mentor_max_capacity ?? 10);

	// Form state — seeded from the server-provided profile.
	let isMentor = $state(untrack(() => profile?.is_mentor ?? false));
	let capacity = $state(untrack(() => profile?.mentor_capacity ?? Math.min(5, capacityMax)));
	let areas = $state(untrack(() => (profile?.mentor_areas ?? []).join(', ')));
	let bio = $state(untrack(() => profile?.mentor_bio ?? ''));

	let saving = $state(false);
	let saved = $state(false);
	let error: string | null = $state(null);

	// Re-seed when the server payload changes (e.g. after invalidateAll).
	$effect(() => {
		isMentor = profile?.is_mentor ?? false;
		capacity = profile?.mentor_capacity ?? Math.min(5, capacityMax);
		areas = (profile?.mentor_areas ?? []).join(', ');
		bio = profile?.mentor_bio ?? '';
	});

	async function save() {
		saving = true;
		saved = false;
		error = null;
		const areasArr = areas
			.split(',')
			.map((a) => a.trim())
			.filter(Boolean);
		const res = await api.patch<MentorProfile>('/me/mentor-profile', {
			is_mentor: isMentor,
			mentor_capacity: Number(capacity),
			mentor_areas: areasArr,
			mentor_bio: bio.trim() || null
		});
		saving = false;
		if (res.error) {
			error = res.error.message || t('mentors.settings_error', 'Failed to save mentor settings.');
			return;
		}
		saved = true;
		await invalidateAll();
	}
</script>

<SettingsSection
	titleKey="mentors.settings_title"
	descKey="mentors.settings_description"
>
	{#if !mentorshipEnabled}
		<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-4 text-sm text-[var(--color-text-subtle)]">
			{t('mentors.settings_disabled', 'Mentorship is not currently available.')}
		</div>
	{:else if !profile}
		<div class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] p-4 text-sm text-[var(--color-text-subtle)]">
			{t(
				'mentors.settings_no_permission',
				'You do not have permission to become a mentor. Contact an administrator if you believe this is an error.'
			)}
		</div>
	{:else}
		<form onsubmit={(e) => { e.preventDefault(); save(); }} class="space-y-5">
			<!-- is_mentor toggle -->
			<div class="flex items-start gap-3">
				<input
					id="is-mentor"
					type="checkbox"
					checked={isMentor}
					onchange={(e) => (isMentor = e.currentTarget.checked)}
					class="mt-1 h-4 w-4 rounded border-[var(--color-border)]"
				/>
				<div>
					<label for="is-mentor" class="text-sm font-medium text-[var(--color-text)]">
						{t('mentors.settings_is_mentor', 'I am available as a mentor')}
					</label>
					<p class="text-xs text-[var(--color-text-subtle)]">
						{t(
							'mentors.settings_is_mentor_help',
							'When enabled, you appear in mentor search results and can receive mentorship requests.'
						)}
					</p>
				</div>
			</div>

			<!-- Capacity -->
			<div class="flex flex-col gap-1">
				<label for="capacity" class="text-sm font-medium text-[var(--color-text)]">
					{t('mentors.settings_capacity', 'Mentee capacity')}
				</label>
				<input
					id="capacity"
					type="number"
					min="1"
					max={capacityMax}
					bind:value={capacity}
					class="w-24 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t(
						'mentors.settings_capacity_help',
						'Maximum number of active mentees. Platform cap: {max}.',
						{ max: capacityMax }
					)}
				</p>
			</div>

			<!-- Areas -->
			<div class="flex flex-col gap-1">
				<label for="areas" class="text-sm font-medium text-[var(--color-text)]">
					{t('mentors.settings_areas', 'Areas of expertise')}
				</label>
				<input
					id="areas"
					type="text"
					bind:value={areas}
					placeholder={t('mentors.settings_areas_placeholder', 'e.g. visa, housing, career')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				/>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{t('mentors.settings_areas_help', 'Comma-separated keywords used for matching.')}
				</p>
			</div>

			<!-- Bio -->
			<div class="flex flex-col gap-1">
				<label for="bio" class="text-sm font-medium text-[var(--color-text)]">
					{t('mentors.settings_bio', 'Mentor bio')}
				</label>
				<textarea
					id="bio"
					bind:value={bio}
					rows="4"
					maxlength="1000"
					placeholder={t(
						'mentors.settings_bio_placeholder',
						'Share your background and what you can help with.'
					)}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
				></textarea>
				<p class="text-xs text-[var(--color-text-subtle)]">
					{bio.length}/1000
				</p>
			</div>

			{#if error}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{error}
				</p>
			{/if}
			{#if saved}
				<p class="inline-flex items-center gap-1.5 text-sm text-green-600">
					<Check size={14} /> {t('mentors.settings_saved', 'Saved.')}
				</p>
			{/if}

			<div class="flex justify-end">
				<button
					type="submit"
					disabled={saving}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if saving}
						<Loader2 size={14} class="animate-spin" />
					{:else}
						<GraduationCap size={14} />
					{/if}
					{t('common.save', 'Save')}
				</button>
			</div>
		</form>
	{/if}
</SettingsSection>
