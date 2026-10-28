<script lang="ts">
	import { untrack } from 'svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchPrivacy } from '$lib/api/hub';
	import { t } from '$lib/i18n';
	import type { PrivacySettings } from '$lib/types';

	interface PrivacySectionData {
		settings?: PrivacySettings | null;
		options?: { dm_privacy?: string[]; profile_visibility?: string[] } | null;
	}

	let { data }: { data: unknown } = $props();
	const sectionData = $derived((data as PrivacySectionData | null) ?? {});
	const privacy = $derived(sectionData.settings ?? {});
	const options = $derived(sectionData.options ?? { dm_privacy: [], profile_visibility: [] });

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

	// Rule 16: allowed enum values come from GET /me/privacy/options.
	// Labels use i18n keys keyed by the option value; fallback to a
	// title-cased version of the value.
	const visibilityOptions = $derived(
		(options.profile_visibility ?? []).map((value) => ({
			value,
			labelKey: `settings.privacy.visibility_${value}`
		}))
	);
	const dmPrivacyOptions = $derived(
		(options.dm_privacy ?? []).map((value) => ({
			value,
			labelKey: `settings.privacy.dm_privacy_${value}`
		}))
	);

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
					onclick={() => form.set({ profile_visibility: opt.value as PrivacySettings['profile_visibility'] })}
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
					onclick={() => form.set({ dm_privacy: opt.value as PrivacySettings['dm_privacy'] })}
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

	<!-- Manage blocked users — the blocked-accounts list lives on the
	     dashboard Relationships tab; surface it here where users look for
	     privacy controls (BLK-017). -->
	<div class="mt-4">
		<a
			href="/dashboard/relationships"
			class="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--color-primary)] hover:underline"
		>
			{t('settings.privacy.blocked_users', 'Manage blocked users')}
		</a>
		<p class="mt-0.5 text-xs text-[var(--color-muted)]">
			{t('settings.privacy.blocked_users_desc', 'Review and unblock accounts you have blocked.')}
		</p>
	</div>

	<!-- Manage blocked sources — the unblock list lived at an orphaned URL
	     (/me/blocked-sources) with no navigation path to it. -->
	<div class="mt-4 border-t border-[var(--color-border)] pt-4">
		<a
			href="/me/blocked-sources"
			class="text-sm font-medium text-[var(--color-primary)] hover:underline"
		>
			{t('settings.privacy.blocked_sources', 'Manage blocked sources')}
		</a>
		<p class="mt-0.5 text-xs text-[var(--color-muted)]">
			{t('settings.privacy.blocked_sources_desc', 'Review and unblock sources you have hidden from your feeds.')}
		</p>
	</div>
</SettingsSection>
