<script lang="ts">
	import { untrack } from 'svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchNotifications } from '$lib/api/hub';
	import { t } from '$lib/i18n';

	let { data }: { data: unknown } = $props();
	const prefs = $derived((data as Record<string, Record<string, boolean>> | null) ?? {});

	type NotifPrefs = {
		email: { mentions: boolean; comments: boolean; digest: boolean };
		push: { mentions: boolean; comments: boolean };
	};

	// Nested-map dirty form — each toggle lives under a channel key (email/push),
	// proving useDirtyForm's nested dirty detection.
	const form = new DirtyForm<NotifPrefs>(
		untrack(() => ({
			email: {
				mentions: prefs.email?.mentions ?? true,
				comments: prefs.email?.comments ?? true,
				digest: prefs.email?.digest ?? false
			},
			push: {
				mentions: prefs.push?.mentions ?? true,
				comments: prefs.push?.comments ?? false
			}
		})),
		(v) => patchNotifications(v as unknown as Record<string, unknown>)
	);

	function toggleEmail(key: 'mentions' | 'comments' | 'digest', next: boolean) {
		form.set({ email: { ...form.values.email, [key]: next } });
	}
	function togglePush(key: 'mentions' | 'comments', next: boolean) {
		form.set({ push: { ...form.values.push, [key]: next } });
	}

	const emailKeys: ('mentions' | 'comments' | 'digest')[] = ['mentions', 'comments', 'digest'];
	const pushKeys: ('mentions' | 'comments')[] = ['mentions', 'comments'];
</script>

<SettingsSection titleKey="settings.nav.notifications" descKey="settings.notifications.desc" {form}>
	<div class="mb-5">
		<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('settings.notifications.email')}</h2>
		{#each emailKeys as key (key)}
			<label class="mb-2 flex items-center justify-between gap-3">
				<span class="text-sm text-[var(--color-text)]">{t(`settings.notifications.email_${key}`)}</span>
				<input
					type="checkbox"
					class="h-4 w-4 rounded border-[var(--color-border)]"
					checked={form.values.email[key]}
					onchange={(e) => toggleEmail(key, e.currentTarget.checked)}
				/>
			</label>
		{/each}
	</div>
	<div class="mb-5">
		<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('settings.notifications.push')}</h2>
		{#each pushKeys as key (key)}
			<label class="mb-2 flex items-center justify-between gap-3">
				<span class="text-sm text-[var(--color-text)]">{t(`settings.notifications.push_${key}`)}</span>
				<input
					type="checkbox"
					class="h-4 w-4 rounded border-[var(--color-border)]"
					checked={form.values.push[key]}
					onchange={(e) => togglePush(key, e.currentTarget.checked)}
				/>
			</label>
		{/each}
	</div>
</SettingsSection>
