<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import SettingsGroupRail from '$lib/components/hub/settings/SettingsGroupRail.svelte';
	import {
		readyGroups,
		groupForSection,
		sectionsInGroup
	} from '$lib/components/hub/settings/settingsSections';
	let { children } = $props();

	const groups = $derived(readyGroups());
	const activeSection = $derived(page.params.section ?? 'profile');
	const activeGroup = $derived(groupForSection(activeSection) ?? 'account');
	const groupSections = $derived(sectionsInGroup(activeGroup));
</script>

<div class="grid grid-cols-1 gap-6 md:grid-cols-[200px_1fr]">
	<SettingsGroupRail
		{groups}
		basePath="/settings"
		{activeGroup}
		labelKey="settings.nav.label"
	/>

	<div class="min-w-0 px-1 py-4">
		<!-- Section tabs within the active group -->
		{#if groupSections.length > 1}
			<nav
				class="mb-4 flex gap-1 overflow-x-auto overflow-y-hidden border-b border-[var(--color-border)] px-1"
				aria-label={t('settings.tabs_label', 'Settings sections')}
			>
				{#each groupSections as sect (sect.slug)}
					<a
						href="/settings/{sect.slug}"
						data-sveltekit-noscroll
						aria-current={activeSection === sect.slug ? 'page' : undefined}
						class="relative flex shrink-0 items-center px-4 py-2 text-sm font-medium transition-colors {activeSection === sect.slug
							? 'text-[var(--color-text)]'
							: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>
						{t(sect.labelKey)}
						{#if activeSection === sect.slug}
							<span class="absolute inset-x-3 -bottom-px h-0.5 rounded-full bg-[var(--color-primary)]"></span>
						{/if}
					</a>
				{/each}
			</nav>
		{/if}

		{@render children()}
	</div>
</div>
