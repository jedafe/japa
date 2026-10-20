<script lang="ts">
	import { untrack } from 'svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { patchProfile } from '$lib/api/hub';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { Camera, Image as ImageIcon, Loader2 } from 'lucide-svelte';
	import type { MeProfile, MediaAsset } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const profile = $derived(data as MeProfile | null);

	// Static enums — these are platform-level option sets, not backend data.
	const genderOptions = [
		{ value: '', label: t('settings.profile.unspecified', 'Prefer not to say') },
		{ value: 'male', label: t('settings.profile.gender_male', 'Male') },
		{ value: 'female', label: t('settings.profile.gender_female', 'Female') },
		{ value: 'non_binary', label: t('settings.profile.gender_non_binary', 'Non-binary') },
		{ value: 'other', label: t('settings.profile.gender_other', 'Other') }
	];
	const maritalOptions = [
		{ value: '', label: t('settings.profile.unspecified', 'Prefer not to say') },
		{ value: 'single', label: t('settings.profile.marital_single', 'Single') },
		{ value: 'married', label: t('settings.profile.marital_married', 'Married') },
		{ value: 'divorced', label: t('settings.profile.marital_divorced', 'Divorced') },
		{ value: 'widowed', label: t('settings.profile.marital_widowed', 'Widowed') },
		{ value: 'other', label: t('settings.profile.marital_other', 'Other') }
	];

	// Prepopulated pick-lists for profession/education — platform-level option
	// sets (curated for the site's japa-relevant audience), not backend data.
	// AutocompleteSelect allows free-text entry beyond the list.
	const professionOptions = [
		'Software Engineer', 'Product Manager', 'Designer', 'Data Scientist',
		'Doctor / Physician', 'Nurse / Midwife', 'Pharmacist', 'Dentist',
		'Teacher / Lecturer', 'Professor / Academic', 'Student',
		'Engineer (Civil/Mechanical/Electrical)', 'Architect',
		'Accountant / Auditor', 'Banker / Financial Analyst',
		'Lawyer / Solicitor', 'Journalist / Media',
		'Entrepreneur / Founder', 'Trader / Business Owner',
		'Civil Servant', 'Military / Paramilitary',
		'Pilot / Aviation', 'Logistics / Supply Chain',
		'Chef / Hospitality', 'Driver / Transport',
		'Artisan / Skilled Trade', 'Farmer / Agriculture',
		'Artist / Creative', 'Musician / Entertainer',
		'NGO / Social Worker', 'Clergy / Religious Worker',
		'Real Estate Agent', 'Sales / Marketing', 'HR / Recruiter',
		'Customer Support', 'Security Professional', 'Other'
	].map((v) => ({ value: v, label: v }));

	const educationOptions = [
		'No formal education', 'Primary school', 'Secondary school (SSCE/WAEC)',
		'Vocational / Trade certificate', 'OND / NCE', 'HND',
		"Bachelor's degree (BSc/BA/BEng)", 'Postgraduate diploma (PGD)',
		"Master's degree (MSc/MA/MBA)", 'Doctorate (PhD/MD)', 'Other'
	].map((v) => ({ value: v, label: v }));

	const form = new DirtyForm(
		untrack(() => ({
			display_name: profile?.display_name ?? '',
			bio: profile?.bio ?? '',
			website_url: profile?.website_url ?? '',
			education: profile?.education ?? '',
			profession: profile?.profession ?? '',
			marital_status: profile?.marital_status ?? '',
			date_of_birth: profile?.date_of_birth ?? '',
			gender: profile?.gender ?? '',
			avatar_url: profile?.avatar_url ?? '',
			cover_url: profile?.cover_url ?? ''
		})),
		// avatar_url/cover_url are persisted immediately by the media-picker
		// PATCH below — strip them here so a stale in-form URL can't clobber a
		// newer server-side value on a plain text save.
		(v) => {
			const { avatar_url: _a, cover_url: _c, ...fields } = v;
			return patchProfile(fields);
		}
	);

	// Constrain DOB client-side to the range the backend accepts
	// (1900 → today) so invalid picks never reach the API.
	const dobMax = new Date().toISOString().slice(0, 10);

	let avatarPickerOpen = $state(false);
	let coverPickerOpen = $state(false);
	let profileSaving = $state(false);
	let profileError = $state<string | null>(null);

	async function handleAvatarSelect(asset: MediaAsset) {
		avatarPickerOpen = false;
		profileSaving = true;
		profileError = null;
		const res = await api.patch<{ status: string }>('/me/profile', { avatar_media_id: asset.id });
		profileSaving = false;
		if (res.error) {
			profileError = res.error.message;
		} else {
			// Already persisted — advance the snapshot so dirty stays false.
			form.setSaved({ avatar_url: asset.url });
		}
	}

	async function handleCoverSelect(asset: MediaAsset) {
		coverPickerOpen = false;
		profileSaving = true;
		profileError = null;
		const res = await api.patch<{ status: string }>('/me/profile', { header_media_id: asset.id });
		profileSaving = false;
		if (res.error) {
			profileError = res.error.message;
		} else {
			// Already persisted — advance the snapshot so dirty stays false.
			form.setSaved({ cover_url: asset.url });
		}
	}
</script>

{#if !profile}
	<!-- The /me loader degraded (network/backend failure). Rendering an empty
		 editable form would PATCH empty strings over the user's real profile —
		 show a recoverable error instead. -->
	<section class="max-w-2xl">
		<header class="mb-4">
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.profile')}</h1>
			<p class="text-sm text-[var(--color-muted)]">{t('settings.profile.desc')}</p>
		</header>
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<p class="text-sm text-[var(--color-text)]">{t('settings.profile.load_error', 'Your profile could not be loaded.')}</p>
			<button
				type="button"
				class="mt-3 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white hover:bg-[var(--color-primary-hover)]"
				onclick={() => location.reload()}
			>
				{t('settings.profile.reload', 'Reload')}
			</button>
		</div>
	</section>
{:else}
<SettingsSection titleKey="settings.nav.profile" descKey="settings.profile.desc" {form}>
	<!-- Avatar + cover upload (Phase 18.04) -->
	<div class="mb-4 space-y-3">
		<div class="flex items-center gap-4">
			<div class="relative">
				{#if form.values.avatar_url}
					<img src={form.values.avatar_url} alt="Avatar" class="h-16 w-16 rounded-full object-cover" />
				{:else}
					<div class="grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
						<ImageIcon size={24} />
					</div>
				{/if}
				{#if profileSaving}
					<div class="absolute inset-0 grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface)]/60">
						<Loader2 size={16} class="animate-spin text-[var(--color-primary)]" />
					</div>
				{/if}
				<button type="button" disabled={profileSaving} aria-label={t('settings.profile.change_avatar', 'Change avatar')} class="absolute -bottom-1 -right-1 grid h-6 w-6 cursor-pointer place-items-center rounded-full bg-[var(--color-primary)] text-white hover:bg-[var(--color-primary-hover)] disabled:opacity-50" onclick={() => (avatarPickerOpen = true)}>
					<Camera size={12} />
				</button>
			</div>
			<div>
				<p class="text-sm font-medium text-[var(--color-text)]">{t('settings.profile.avatar', 'Avatar')}</p>
				<p class="text-xs text-[var(--color-muted)]">{t('settings.profile.avatar_hint', 'Click the camera to upload')}</p>
			</div>
		</div>

		<div>
			<p class="mb-1 text-sm font-medium text-[var(--color-text)]">{t('settings.profile.cover', 'Cover photo')}</p>
			<div class="relative h-24 overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]">
				{#if form.values.cover_url}
					<img src={form.values.cover_url} alt="Cover" class="h-full w-full object-cover" />
				{:else}
					<div class="grid h-full place-items-center text-[var(--color-muted)]">
						<ImageIcon size={28} />
					</div>
				{/if}
				{#if profileSaving}
					<div class="absolute inset-0 grid place-items-center bg-[var(--color-surface)]/60">
						<Loader2 size={18} class="animate-spin text-[var(--color-primary)]" />
					</div>
				{/if}
				<button type="button" disabled={profileSaving} class="absolute bottom-2 right-2 inline-flex cursor-pointer items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-2 py-1 text-xs text-white hover:bg-[var(--color-primary-hover)] disabled:opacity-50" onclick={() => (coverPickerOpen = true)}>
					<Camera size={12} />
					{t('settings.profile.upload', 'Upload')}
				</button>
			</div>
		</div>
		{#if profileError}
			<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{profileError}</p>
		{/if}

	</div>

	<label class="mb-3 block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.name')}</span>
		<input
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
			value={form.values.display_name}
			oninput={(e) => form.set({ display_name: e.currentTarget.value })}
		/>
	</label>
	<label class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.bio')}</span>
		<textarea
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
			rows="4"
			value={form.values.bio}
			oninput={(e) => form.set({ bio: e.currentTarget.value })}
		></textarea>
	</label>
	<label class="mt-3 block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.website', 'Website')}</span>
		<input
			type="url"
			placeholder="https://"
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
			value={form.values.website_url}
			oninput={(e) => form.set({ website_url: e.currentTarget.value })}
		/>
	</label>

	<div class="mt-4 grid grid-cols-1 gap-3 border-t border-[var(--color-border)] pt-4 sm:grid-cols-2">
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.profession', 'Profession')}</span>
			<AutocompleteSelect
				options={professionOptions}
				value={form.values.profession}
				ariaLabel={t('settings.profile.profession', 'Profession')}
				onValueChange={(v) => form.set({ profession: v ?? '' })}
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.education', 'Education')}</span>
			<AutocompleteSelect
				options={educationOptions}
				value={form.values.education}
				ariaLabel={t('settings.profile.education', 'Education')}
				onValueChange={(v) => form.set({ education: v ?? '' })}
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.date_of_birth', 'Date of birth')}</span>
			<input
				type="date"
				min="1900-01-01"
				max={dobMax}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
				value={form.values.date_of_birth}
				oninput={(e) => form.set({ date_of_birth: e.currentTarget.value })}
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.gender', 'Gender')}</span>
			<AutocompleteSelect
				options={genderOptions}
				value={form.values.gender}
				ariaLabel={t('settings.profile.gender', 'Gender')}
				onValueChange={(v) => form.set({ gender: v ?? '' })}
			/>
		</label>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.marital_status', 'Marital status')}</span>
			<AutocompleteSelect
				options={maritalOptions}
				value={form.values.marital_status}
				ariaLabel={t('settings.profile.marital_status', 'Marital status')}
				onValueChange={(v) => form.set({ marital_status: v ?? '' })}
			/>
		</label>
	</div>
</SettingsSection>
{/if}

{#if avatarPickerOpen}
	<MediaLibraryModal onSelect={handleAvatarSelect} onClose={() => (avatarPickerOpen = false)} allowedType="image" uploadContext="avatar" />
{/if}
{#if coverPickerOpen}
	<MediaLibraryModal onSelect={handleCoverSelect} onClose={() => (coverPickerOpen = false)} allowedType="image" />
{/if}
