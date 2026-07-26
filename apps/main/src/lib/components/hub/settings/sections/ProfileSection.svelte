<script lang="ts">
	import { untrack } from 'svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchProfile } from '$lib/api/hub';
	import { initiateUpload, uploadToPresigned, confirmMedia } from '$lib/api/create';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { Camera, Loader2, Image as ImageIcon } from 'lucide-svelte';
	import type { MeProfile } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const profile = $derived(data as MeProfile | null);

	const form = new DirtyForm(
		untrack(() => ({
			displayName: profile?.displayName ?? '',
			bio: profile?.bio ?? '',
			avatar_url: profile?.avatar_url ?? '',
			cover_url: profile?.cover_url ?? ''
		})),
		(v) => patchProfile(v)
	);

	let avatarUploading = $state(false);
	let coverUploading = $state(false);
	let uploadError = $state('');

	async function uploadImage(file: File, kind: 'avatar' | 'cover') {
		if (kind === 'avatar') avatarUploading = true;
		else coverUploading = true;
		uploadError = '';

		const init = await initiateUpload({
			filename: file.name,
			mime_type: file.type,
			file_size: file.size
		});
		if (init.error || !init.data) {
			uploadError = t('settings.profile.upload_failed', 'Upload failed');
			if (kind === 'avatar') avatarUploading = false;
			else coverUploading = false;
			return;
		}

		const up = await uploadToPresigned(init.data.presigned_url, file);
		if (!up.ok) {
			uploadError = t('settings.profile.upload_failed', 'Upload failed');
			if (kind === 'avatar') avatarUploading = false;
			else coverUploading = false;
			return;
		}

		const confirmed = await confirmMedia(init.data.media_id);
		if (confirmed.error || !confirmed.data) {
			uploadError = t('settings.profile.upload_failed', 'Upload failed');
			if (kind === 'avatar') avatarUploading = false;
			else coverUploading = false;
			return;
		}

		// PATCH /me/profile with the new media URL.
		const url = confirmed.data.url;
		const res = await api.patch<MeProfile>('/me/profile', {
			[kind === 'avatar' ? 'avatar_url' : 'cover_url']: url
		});
		if (res.error) {
			uploadError = t('settings.profile.upload_failed', 'Upload failed');
		} else {
			form.set({ [kind === 'avatar' ? 'avatar_url' : 'cover_url']: url } as Record<string, string>);
		}
		if (kind === 'avatar') avatarUploading = false;
		else coverUploading = false;
	}

	function onAvatarChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		if (input.files?.[0]) uploadImage(input.files[0], 'avatar');
	}
	function onCoverChange(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		if (input.files?.[0]) uploadImage(input.files[0], 'cover');
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
				<label class="absolute -bottom-1 -right-1 grid h-6 w-6 cursor-pointer place-items-center rounded-full bg-[var(--color-primary)] text-white hover:bg-[var(--color-primary-hover)]">
					{#if avatarUploading}
						<Loader2 size={12} class="animate-spin" />
					{:else}
						<Camera size={12} />
					{/if}
					<input type="file" accept="image/*" class="hidden" onchange={onAvatarChange} disabled={avatarUploading} />
				</label>
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
				<label class="absolute bottom-2 right-2 inline-flex cursor-pointer items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-2 py-1 text-xs text-white hover:bg-[var(--color-primary-hover)]">
					{#if coverUploading}
						<Loader2 size={12} class="animate-spin" />
					{:else}
						<Camera size={12} />
					{/if}
					{t('settings.profile.upload', 'Upload')}
					<input type="file" accept="image/*" class="hidden" onchange={onCoverChange} disabled={coverUploading} />
				</label>
			</div>
		</div>

		{#if uploadError}
			<p class="text-sm text-red-500" role="alert">{uploadError}</p>
		{/if}
	</div>

	<label class="mb-3 block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.name')}</span>
		<input
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
			value={form.values.displayName}
			oninput={(e) => form.set({ displayName: e.currentTarget.value })}
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
