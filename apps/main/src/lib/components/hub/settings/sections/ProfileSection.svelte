<script lang="ts">
	import { untrack } from 'svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchProfile } from '$lib/api/hub';
	import { t } from '$lib/i18n';
	import type { MeProfile } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const profile = $derived(data as MeProfile | null);

	const form = new DirtyForm(
		untrack(() => ({ displayName: profile?.displayName ?? '', bio: profile?.bio ?? '' })),
		(v) => patchProfile(v)
	);
</script>

<SettingsSection titleKey="settings.nav.profile" descKey="settings.profile.desc" {form}>
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
