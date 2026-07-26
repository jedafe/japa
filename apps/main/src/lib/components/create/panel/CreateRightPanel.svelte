<script lang="ts">
	import { Tabs } from 'bits-ui';
	import { t } from '$lib/i18n';
	import CreateGuidePanel from './CreateGuidePanel.svelte';
	import CreateSettingsPanel from './CreateSettingsPanel.svelte';
	import type { BlogSeries, CustomFieldGroup, MediaAsset, Visibility } from '$lib/types';

	interface Props {
		focusedField: 'space' | 'tags' | 'series' | 'body' | null;
		visibility: Visibility;
		canonicalUrl: string;
		publishAt: string | null;
		fieldValues: Record<string, unknown>;
		fieldGroups: CustomFieldGroup[];
		series: BlogSeries[];
		seriesId: string | null;
		metaTitle: string | null;
		metaDescription: string | null;
		ogImageUrl: string | null;
		ogImagePreviewUrl: string | null;
		seoIndexable: boolean;
		onvisibility: (v: Visibility) => void;
		oncanonical: (v: string) => void;
		onpublishat: (v: string | null) => void;
		onfieldvalues: (v: Record<string, unknown>) => void;
		onseries: (id: string | null) => void;
		onseriesfocus: () => void;
		onmetatitle: (v: string | null) => void;
		onmetadescription: (v: string | null) => void;
		onogimageselect: (asset: MediaAsset | null) => void;
		onseoindexable: (v: boolean) => void;
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
		metaTitle,
		metaDescription,
		ogImageUrl,
		ogImagePreviewUrl,
		seoIndexable,
		onvisibility,
		oncanonical,
		onpublishat,
		onfieldvalues,
		onseries,
		onseriesfocus,
		onmetatitle,
		onmetadescription,
		onogimageselect,
		onseoindexable,
		onedit
	}: Props = $props();
</script>

<Tabs.Root value="guide">
	<Tabs.List class="right-panel-tabs">
		<Tabs.Trigger value="guide" class="right-panel-tab">
			{t('create.panel.guide')}
		</Tabs.Trigger>
		<Tabs.Trigger value="settings" class="right-panel-tab">
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
			{metaTitle}
			{metaDescription}
			{ogImageUrl}
			{ogImagePreviewUrl}
			{seoIndexable}
			{onvisibility}
			{oncanonical}
			{onpublishat}
			{onfieldvalues}
			{onseries}
			{onseriesfocus}
			{onmetatitle}
			{onmetadescription}
			{onogimageselect}
			{onseoindexable}
			{onedit}
		/>
	</Tabs.Content>
</Tabs.Root>

<style>
	:global(.right-panel-tabs) {
		display: flex;
		border-bottom: 1px solid var(--color-border);
	}
	:global(.right-panel-tab) {
		padding: 8px 12px;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-muted);
		cursor: pointer;
		transition: color 0.15s;
	}
	:global(.right-panel-tab:hover) {
		color: var(--color-text);
	}
	:global([data-state='active'].right-panel-tab) {
		color: var(--color-text);
		border-bottom: 2px solid var(--color-primary);
	}
</style>
