<script lang="ts">
	import { t } from '$lib/i18n';
	import type { HubGroup } from '$lib/types';

	interface RailItem {
		slug: string;
		labelKey: string;
		backendReady: boolean;
		component: unknown;
		isRoute?: boolean;
	}
	interface Props {
		groups: [HubGroup, RailItem[]][];
		basePath: string;
		active: string;
		labelKey: string;
	}
	let { groups, basePath, active, labelKey }: Props = $props();
</script>

<nav class="sticky top-[var(--nav-height)] hidden self-start md:block" aria-label={t(labelKey)}>
	{#each groups as [group, items] (group)}
		<div class="mb-4">
			<p class="px-3 pb-1 text-xs font-medium text-[var(--color-muted)]">{t(`settings.group.${group}`)}</p>
			{#each items as item (item.slug)}
				{@const disabled = !item.backendReady || (!item.component && !item.isRoute)}
				<a
					href={disabled ? undefined : `${basePath}/${item.slug}`}
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
