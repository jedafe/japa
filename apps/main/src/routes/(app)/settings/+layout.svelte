<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { sectionsByGroup } from '$lib/components/hub/settings/settingsSections';
	let { children } = $props();
	const groups = sectionsByGroup();
	const active = $derived(page.params.section);
</script>

<div class="grid grid-cols-1 gap-6 md:grid-cols-[220px_1fr]">
	<nav class="sticky top-[var(--nav-height)] hidden self-start md:block" aria-label={t('settings.nav.label')}>
		{#each groups as [group, items] (group)}
			<div class="mb-4">
				<p class="px-3 pb-1 text-xs font-medium text-[var(--color-muted)]">{t(`settings.group.${group}`)}</p>
				{#each items as item (item.slug)}
					{@const disabled = !item.backendReady || !item.component}
					<a
						href={disabled ? undefined : `/settings/${item.slug}`}
						aria-current={active === item.slug ? 'page' : undefined}
						aria-disabled={disabled}
						class="block rounded-[var(--radius)] px-3 py-1.5 text-sm {active === item.slug
							? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
							: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'} {disabled
							? 'pointer-events-none opacity-40'
							: ''}"
					>
						{t(item.labelKey)}
					</a>
				{/each}
			</div>
		{/each}
	</nav>
	<div class="min-w-0 px-1 py-4">{@render children()}</div>
</div>
