<script lang="ts">
	import { untrack } from 'svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import { patchProfile } from '$lib/api/hub';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { Camera, Image as ImageIcon, Loader2 } from 'lucide-svelte';
	import type { MeProfile, MediaAsset } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const profile = $derived(data as MeProfile | null);

	const form = new DirtyForm(
		untrack(() => ({
			display_name: profile?.display_name ?? '',
			bio: profile?.bio ?? '',
			avatar_url: profile?.avatar_url ?? '',
			cover_url: profile?.cover_url ?? ''
		})),
		(v) => patchProfile(v)
	);

	let avatarPickerOpen = $state(false);
	let coverPickerOpen = $state(false);
	let profileSaving = $state(false);
	let profileError = $state<string | null>(null);

	async function handleAvatarSelect(asset: MediaAsset) {
		avatarPickerOpen = false;
		profileSaving = true;
		profileError = null;
		const res = await api.patch<MeProfile>('/me/profile', { avatar_media_id: asset.id });
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
		const res = await api.patch<MeProfile>('/me/profile', { header_media_id: asset.id });
		profileSaving = false;
		if (res.error) {
			profileError = res.error.message;
		} else {
			// Already persisted — advance the snapshot so dirty stays false.
			form.setSaved({ cover_url: asset.url });
		}
	}
</script>

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
				<button type="button" disabled={profileSaving} class="absolute -bottom-1 -right-1 grid h-6 w-6 cursor-pointer place-items-center rounded-full bg-[var(--color-primary)] text-white hover:bg-[var(--color-primary-hover)] disabled:opacity-50" onclick={() => (avatarPickerOpen = true)}>
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
			{#if profileError}
				<p class="mt-1 text-xs text-[var(--color-danger,#dc2626)]">{profileError}</p>
			{/if}
		</div>

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
</SettingsSection>

{#if avatarPickerOpen}
	<MediaLibraryModal onSelect={handleAvatarSelect} onClose={() => (avatarPickerOpen = false)} allowedType="image" uploadContext="avatar" />
{/if}
{#if coverPickerOpen}
	<MediaLibraryModal onSelect={handleCoverSelect} onClose={() => (coverPickerOpen = false)} allowedType="image" />
{/if}
