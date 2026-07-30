<script lang="ts">
	import { Select } from 'bits-ui';
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchAccount } from '$lib/api/hub';
	import { theme as themeStore } from '$lib/stores/theme.svelte';
	import { t } from '$lib/i18n';
	import type { MeProfile, ThemeName } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const profile = $derived(data as MeProfile | null);

	const themeItems: { value: ThemeName; label: string }[] = [
		{ value: 'light', label: t('theme.light', 'Light') },
		{ value: 'dark', label: t('theme.dark', 'Dark') }
	];

	// Default to the user's saved preference, then the server-resolved theme
	// from page.data.theme (which already accounts for the admin
	// appearance.dark_mode_default setting), then 'light'. This keeps the
	// selector in sync with the actual <html data-theme> attribute.
	const form = new DirtyForm(
		untrack(() => ({
			username: profile?.username ?? '',
			theme: (profile?.ui?.theme as ThemeName) ?? (page.data.theme as ThemeName) ?? 'light'
		})),
		(v) => patchAccount({ username: v.username, ui: { theme: v.theme } })
	);
</script>

<SettingsSection titleKey="settings.nav.account" descKey="settings.account.desc" {form}>
	<div class="mb-3">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.account.email')}</span>
		<p class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-muted)]">
			{profile?.email ?? '—'}
		</p>
	</div>
	<label class="mb-3 block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.account.username')}</span>
		<input
			class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
			value={form.values.username}
			oninput={(e) => form.set({ username: e.currentTarget.value })}
		/>
	</label>
	<div class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.account.theme')}</span>
		<Select.Root
			type="single"
			value={form.values.theme}
			onValueChange={(v) => {
				const next = (v as ThemeName) ?? 'light';
				form.set({ theme: next });
				themeStore.set(next);
			}}
		>
			<Select.Trigger
				class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				aria-label={t('settings.account.theme')}
			>
				{form.values.theme === 'dark' ? t('settings.account.theme_dark') : t('settings.account.theme_light')}
			</Select.Trigger>
			<Select.Portal>
				<Select.Content
					class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow"
				>
					<Select.Viewport>
						{#each themeItems as item (item.value)}
							<Select.Item
								value={item.value}
								label={item.value === 'dark' ? t('settings.account.theme_dark') : t('settings.account.theme_light')}
								class="cursor-pointer rounded px-2 py-1.5 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								{#snippet children({ selected })}
									{selected ? '✓ ' : ''}{item.value === 'dark' ? t('settings.account.theme_dark') : t('settings.account.theme_light')}
								{/snippet}
							</Select.Item>
						{/each}
					</Select.Viewport>
				</Select.Content>
			</Select.Portal>
		</Select.Root>
	</div>
</SettingsSection>
