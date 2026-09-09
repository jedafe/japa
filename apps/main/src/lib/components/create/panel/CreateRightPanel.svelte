<script lang="ts">
	import { Tabs } from 'bits-ui';
	import { t } from '$lib/i18n';
	import CreateGuidePanel from './CreateGuidePanel.svelte';
	import CreateSettingsPanel from './CreateSettingsPanel.svelte';
	import CreateJourneyPanel from './CreateJourneyPanel.svelte';
	import CreateSeoPanel from './CreateSeoPanel.svelte';
	import type { BlogSeries, MediaAsset, MeSpace, Visibility } from '$lib/types';

	interface Props {
		focusedField: 'space' | 'tags' | 'series' | 'body' | null;
		journeyStage: string | null;
		visaRoute: string | null;
		sourceCountry: string | null;
		targetCountry: string | null;
		dimension: string | null;
		purpose: string | null;
		assessmentDimensions: string[];
		onjourney: (v: string | null) => void;
		onvisa: (v: string | null) => void;
		onsource: (v: string | null) => void;
		ontarget: (v: string | null) => void;
		ondimension: (v: string | null) => void;
		onpurpose: (v: string | null) => void;
		slug: string;
		visibility: Visibility;
		canonicalUrl: string;
		publishAt: string | null;
		visibilityOptions?: { value: Visibility; label: string }[];
		series: BlogSeries[];
		seriesId: string | null;
		spaces: MeSpace[];
		spaceIds: string[];
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
		onspace: (ids: string[]) => void;
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
		journeyStage,
		visaRoute,
		sourceCountry,
		targetCountry,
		dimension,
		purpose,
		assessmentDimensions,
		onjourney,
		onvisa,
		onsource,
		ontarget,
		ondimension,
		onpurpose,
		slug,
		visibility,
		canonicalUrl,
		publishAt,
		visibilityOptions,
		series = $bindable(),
		seriesId,
		spaces = $bindable(),
		spaceIds,
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
		<Tabs.Trigger value="journey" class="right-panel-tab">
			{t('create.panel.journey')}
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
			{spaceIds}
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
	<Tabs.Content value="journey" class="pt-2">
		<CreateJourneyPanel
			{journeyStage}
			{visaRoute}
			{sourceCountry}
			{targetCountry}
			{dimension}
			{purpose}
			{assessmentDimensions}
			{onjourney}
			{onvisa}
			{onsource}
			{ontarget}
			{ondimension}
			{onpurpose}
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
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		padding: 10px 12px;
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
