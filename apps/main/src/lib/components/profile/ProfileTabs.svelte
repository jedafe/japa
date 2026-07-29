<script lang="ts">
	import { t } from '$lib/i18n';

	interface Tab {
		key: string;
		labelKey: string;
		href: string;
	}

	interface Props {
		tabs: Tab[];
		active: string;
	}
	let { tabs, active }: Props = $props();
</script>

<nav
	class="sticky top-0 z-10 flex gap-1 overflow-x-auto overflow-y-hidden border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] px-3 backdrop-blur"
	style="min-height: var(--nav-height)"
	aria-label={t('profile.tabs_label', 'Profile tabs')}
>
	{#each tabs as tab (tab.key)}
		<a
			href={tab.href}
			data-sveltekit-noscroll
			aria-current={active === tab.key ? 'page' : undefined}
			class="relative flex shrink-0 items-center px-4 text-sm font-medium transition-colors {active === tab.key
				? 'text-[var(--color-text)]'
				: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
		>
			{t(tab.labelKey, tab.key)}
			{#if active === tab.key}
				<span class="absolute inset-x-3 -bottom-px h-0.5 rounded-full bg-[var(--color-primary)]"></span>
			{/if}
		</a>
	{/each}
</nav>
