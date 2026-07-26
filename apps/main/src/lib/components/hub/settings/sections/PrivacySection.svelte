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
			prefs: {
				show_activity: privacy.show_activity ?? false,
				searchable: privacy.searchable ?? true
			}
		})),
		(v) =>
			patchPrivacy({
				profile_visibility: v.profile_visibility,
				show_activity: v.prefs.show_activity,
				searchable: v.prefs.searchable
			})
	);

	const visibilityOptions: { value: 'public' | 'members' | 'private'; labelKey: string }[] = [
		{ value: 'public', labelKey: 'settings.privacy.visibility_public' },
		{ value: 'members', labelKey: 'settings.privacy.visibility_members' },
		{ value: 'private', labelKey: 'settings.privacy.visibility_private' }
	];

	function togglePref(key: 'show_activity' | 'searchable', next: boolean) {
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
			checked={form.values.prefs.searchable}
			onchange={(e) => togglePref('searchable', e.currentTarget.checked)}
		/>
	</label>
</SettingsSection>
