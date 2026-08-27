<script lang="ts">
	import { untrack } from 'svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchPrivacy } from '$lib/api/hub';
	import { t } from '$lib/i18n';
	import type { PrivacySettings } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const privacy = $derived((data as PrivacySettings | null) ?? {});

	// Nested map — proves useDirtyForm's nested dirty detection (a toggle inside
	// `prefs` must surface as dirty without manual diffing).
	const form = new DirtyForm(
		untrack(() => ({
			profile_visibility: privacy.profile_visibility ?? 'public',
			dm_privacy: privacy.dm_privacy ?? 'everyone',
			prefs: {
				show_online_status: privacy.show_online_status ?? true,
				show_activity: privacy.show_activity ?? false,
				search_indexable: privacy.search_indexable ?? true,
				show_points_publicly: privacy.show_points_publicly ?? false
			}
		})),
		(v) =>
			patchPrivacy({
				profile_visibility: v.profile_visibility,
				dm_privacy: v.dm_privacy,
				show_online_status: v.prefs.show_online_status,
				show_activity: v.prefs.show_activity,
				search_indexable: v.prefs.search_indexable,
				show_points_publicly: v.prefs.show_points_publicly
			})
	);

	// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
	// Spec: profile_visibility: 'public'|'members'|'followers'
	const visibilityOptions: { value: 'public' | 'members' | 'followers'; labelKey: string }[] = [
		{ value: 'public', labelKey: 'settings.privacy.visibility_public' },
		{ value: 'members', labelKey: 'settings.privacy.visibility_members' },
		{ value: 'followers', labelKey: 'settings.privacy.visibility_followers' }
	];

	// Spec: dm_privacy: 'everyone'|'followers_only'|'nobody'
	const dmPrivacyOptions: { value: 'everyone' | 'followers_only' | 'nobody'; labelKey: string }[] = [
		{ value: 'everyone', labelKey: 'settings.privacy.dm_privacy_everyone' },
		{ value: 'followers_only', labelKey: 'settings.privacy.dm_privacy_followers_only' },
		{ value: 'nobody', labelKey: 'settings.privacy.dm_privacy_nobody' }
	];

	function togglePref(key: 'show_online_status' | 'show_activity' | 'search_indexable' | 'show_points_publicly', next: boolean) {
		form.set({ prefs: { ...form.values.prefs, [key]: next } });
	}
</script>

<SettingsSection titleKey="settings.nav.privacy" descKey="settings.privacy.desc" {form}>
	<div class="mb-4">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.privacy.visibility')}</span>
		<div class="flex flex-wrap gap-2">
			{#each visibilityOptions as opt (opt.value)}
				<button
					type="button"
					class="rounded-[var(--radius)] border px-3 py-1.5 text-sm {form.values.profile_visibility ===
					opt.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-white'
						: 'border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
					onclick={() => form.set({ profile_visibility: opt.value })}
				>
					{t(opt.labelKey)}
				</button>
			{/each}
		</div>
	</div>
	<div class="mb-4">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.privacy.dm_privacy')}</span>
		<div class="flex flex-wrap gap-2">
			{#each dmPrivacyOptions as opt (opt.value)}
				<button
					type="button"
					class="rounded-[var(--radius)] border px-3 py-1.5 text-sm {form.values.dm_privacy ===
					opt.value
						? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-white'
						: 'border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
					onclick={() => form.set({ dm_privacy: opt.value })}
				>
					{t(opt.labelKey)}
				</button>
			{/each}
		</div>
	</div>
	<label class="mb-3 flex items-center justify-between gap-3">
		<span class="text-sm text-[var(--color-text)]">{t('settings.privacy.show_online_status')}</span>
		<input
			type="checkbox"
			class="h-4 w-4 rounded border-[var(--color-border)]"
			checked={form.values.prefs.show_online_status}
			onchange={(e) => togglePref('show_online_status', e.currentTarget.checked)}
		/>
	</label>
	<label class="mb-3 flex items-center justify-between gap-3">
		<span class="text-sm text-[var(--color-text)]">{t('settings.privacy.show_activity')}</span>
		<input
			type="checkbox"
			class="h-4 w-4 rounded border-[var(--color-border)]"
			checked={form.values.prefs.show_activity}
			onchange={(e) => togglePref('show_activity', e.currentTarget.checked)}
		/>
	</label>
	<label class="flex items-center justify-between gap-3">
		<span class="text-sm text-[var(--color-text)]">{t('settings.privacy.searchable')}</span>
		<input
			type="checkbox"
			class="h-4 w-4 rounded border-[var(--color-border)]"
			checked={form.values.prefs.search_indexable}
			onchange={(e) => togglePref('search_indexable', e.currentTarget.checked)}
		/>
	</label>
	<label class="flex items-center justify-between gap-3">
		<span class="text-sm text-[var(--color-text)]">{t('settings.privacy.show_points_publicly')}</span>
		<input
			type="checkbox"
			class="h-4 w-4 rounded border-[var(--color-border)]"
			checked={form.values.prefs.show_points_publicly}
			onchange={(e) => togglePref('show_points_publicly', e.currentTarget.checked)}
		/>
	</label>
</SettingsSection>
