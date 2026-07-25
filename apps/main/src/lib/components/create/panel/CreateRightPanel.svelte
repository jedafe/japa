<script lang="ts">
	import { Tabs } from 'bits-ui';
	import { t } from '$lib/i18n';
	import CreateGuidePanel from './CreateGuidePanel.svelte';
	import CreateSettingsPanel from './CreateSettingsPanel.svelte';
	import type { BlogSeries, CustomFieldGroup, Visibility } from '$lib/types';

	interface Props {
		focusedField: 'space' | 'tags' | 'series' | 'body' | null;
		visibility: Visibility;
		canonicalUrl: string;
		publishAt: string | null;
		fieldValues: Record<string, unknown>;
		fieldGroups: CustomFieldGroup[];
		series: BlogSeries[];
		seriesId: string | null;
		onvisibility: (v: Visibility) => void;
		oncanonical: (v: string) => void;
		onpublishat: (v: string | null) => void;
		onfieldvalues: (v: Record<string, unknown>) => void;
		onseries: (id: string | null) => void;
		onedit: () => void;
	}

	let {
		focusedField,
		visibility,
		canonicalUrl,
		publishAt,
		fieldValues,
		fieldGroups,
		series,
		seriesId,
		onvisibility,
		oncanonical,
		onpublishat,
		onfieldvalues,
		onseries,
		onedit
	}: Props = $props();
</script>

<Tabs.Root value="guide">
	<Tabs.List class="flex border-b border-[var(--color-border)]">
		<Tabs.Trigger
			value="guide"
			class="px-3 py-2 text-sm font-medium text-[var(--color-muted)] data-[state=active]:text-[var(--color-text)] data-[state=active]:border-b-2 data-[state=active]:border-[var(--color-primary)]"
		>
			{t('create.panel.guide')}
		</Tabs.Trigger>
		<Tabs.Trigger
			value="settings"
			class="px-3 py-2 text-sm font-medium text-[var(--color-muted)] data-[state=active]:text-[var(--color-text)] data-[state=active]:border-b-2 data-[state=active]:border-[var(--color-primary)]"
		>
			{t('create.panel.settings')}
		</Tabs.Trigger>
	</Tabs.List>
	<Tabs.Content value="guide" class="pt-2">
		<CreateGuidePanel {focusedField} />
	</Tabs.Content>
	<Tabs.Content value="settings" class="pt-2">
		<CreateSettingsPanel
			{visibility}
			{canonicalUrl}
			{publishAt}
			{fieldValues}
			{fieldGroups}
			{series}
			{seriesId}
			onvisibility={onvisibility}
			oncanonical={oncanonical}
			onpublishat={onpublishat}
			onfieldvalues={onfieldvalues}
			onseries={onseries}
			onedit={onedit}
		/>
	</Tabs.Content>
</Tabs.Root>
