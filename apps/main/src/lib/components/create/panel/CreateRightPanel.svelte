<script lang="ts">
	import { Tabs } from 'bits-ui';
	import { t } from '$lib/i18n';
	import CreateGuidePanel from './CreateGuidePanel.svelte';
	import CreateSettingsPanel from './CreateSettingsPanel.svelte';
	import CreateSeoPanel from './CreateSeoPanel.svelte';
	import type { BlogSeries, MediaAsset, MeSpace, Visibility } from '$lib/types';

	interface Props {
		focusedField: 'space' | 'tags' | 'series' | 'body' | null;
		slug: string;
		visibility: Visibility;
		canonicalUrl: string;
		publishAt: string | null;
		visibilityOptions?: { value: Visibility; label: string }[];
		series: BlogSeries[];
		seriesId: string | null;
		spaces: MeSpace[];
		spaceId: string | null;
		tags: string[];
		maxTags?: number;
		coverMediaId: string | null;
		coverPreviewUrl: string | null;
		metaTitle: string | null;
		metaDescription: string | null;
		ogImageUrl: string | null;
		ogImagePreviewUrl: string | null;
		seoIndexable: boolean;
		title: string;
		bodyHtml: string;
		contentType: string;
		onslug: (v: string) => void;
		onvisibility: (v: Visibility) => void;
		oncanonical: (v: string) => void;
		onpublishat: (v: string | null) => void;
		onseries: (id: string | null) => void;
		onseriesfocus: () => void;
		onspace: (id: string) => void;
		onspacefocus: () => void;
		ontags: (next: string[]) => void;
		ontagsfocus: () => void;
		oncoveruploaded: (asset: MediaAsset) => void;
		oncoverremove: () => void;
		onmetatitle: (v: string | null) => void;
		onmetadescription: (v: string | null) => void;
		onogimageselect: (asset: MediaAsset | null) => void;
		onseoindexable: (v: boolean) => void;
		onedit: () => void;
	}

	let {
		focusedField,
		slug,
		visibility,
		canonicalUrl,
		publishAt,
		visibilityOptions,
		series = $bindable(),
		seriesId,
		spaces = $bindable(),
		spaceId,
		tags,
		maxTags = 5,
		coverMediaId,
		coverPreviewUrl,
		metaTitle,
		metaDescription,
		ogImageUrl,
		ogImagePreviewUrl,
		seoIndexable,
		title,
		bodyHtml,
		contentType,
		onslug,
		onvisibility,
		oncanonical,
		onpublishat,
		onseries,
		onseriesfocus,
		onspace,
		onspacefocus,
		ontags,
		ontagsfocus,
		oncoveruploaded,
		oncoverremove,
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
		<Tabs.Trigger value="seo" class="right-panel-tab">
			{t('create.panel.seo')}
		</Tabs.Trigger>
	</Tabs.List>
	<Tabs.Content value="guide" class="pt-2">
		<CreateGuidePanel
			{focusedField}
			bind:series
			{seriesId}
			bind:spaces
			{spaceId}
			{tags}
			{maxTags}
			{coverMediaId}
			{coverPreviewUrl}
			{onseries}
			{onseriesfocus}
			{onspace}
			{onspacefocus}
			{ontags}
			{ontagsfocus}
			{oncoveruploaded}
			{oncoverremove}
			{onedit}
		/>
	</Tabs.Content>
	<Tabs.Content value="settings" class="pt-2">
		<CreateSettingsPanel
			{slug}
			{visibility}
			{canonicalUrl}
			{publishAt}
			{visibilityOptions}
			{onslug}
			{onvisibility}
			{oncanonical}
			{onpublishat}
			{onedit}
		/>
	</Tabs.Content>
	<Tabs.Content value="seo" class="pt-2">
		<CreateSeoPanel
			{metaTitle}
			{metaDescription}
			{ogImageUrl}
			{ogImagePreviewUrl}
			{seoIndexable}
			{title}
			{slug}
			{canonicalUrl}
			{visibility}
			{bodyHtml}
			{contentType}
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
