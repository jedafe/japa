<script lang="ts">
	import { t } from '$lib/i18n';
	import type { HubGroup } from '$lib/types';

	interface GroupItem {
		group: HubGroup;
		/** First section slug in this group — clicking the group navigates here. */
		firstSlug: string;
		count: number;
	}

	interface Props {
		groups: GroupItem[];
		basePath: string;
		activeGroup: HubGroup;
		labelKey: string;
	}
	let { groups, basePath, activeGroup, labelKey }: Props = $props();
</script>

<nav class="sticky top-[var(--nav-height)] hidden self-start md:block" aria-label={t(labelKey)}>
	{#each groups as item (item.group)}
		<a
			href={`${basePath}/${item.firstSlug}`}
			aria-current={activeGroup === item.group ? 'page' : undefined}
			class="block rounded-[var(--radius)] px-3 py-2 text-sm {activeGroup === item.group
				? 'bg-[var(--color-surface-subtle)] font-medium text-[var(--color-text)]'
				: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
		>
			{t(`settings.group.${item.group}`)}
		</a>
	{/each}
</nav>
