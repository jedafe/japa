<script lang="ts">
	import { t } from '$lib/i18n';
	import type { BlogSeries, MediaAsset } from '$lib/types';
	import SeriesSelector from './SeriesSelector.svelte';
	import CoverUploader from '../CoverUploader.svelte';

	type FocusedField = 'space' | 'tags' | 'series' | 'body' | null;

	interface Props {
		focusedField: FocusedField;
		series: BlogSeries[];
		seriesId: string | null;
		coverMediaId: string | null;
		coverPreviewUrl: string | null;
		onseries: (id: string | null) => void;
		onseriesfocus: () => void;
		oncoveruploaded: (asset: MediaAsset) => void;
		oncoverremove: () => void;
		onedit: () => void;
	}

	let {
		focusedField,
		series = $bindable(),
		seriesId,
		coverMediaId,
		coverPreviewUrl,
		onseries,
		onseriesfocus,
		oncoveruploaded,
		oncoverremove,
		onedit
	}: Props = $props();

	const helpKey = $derived(
		focusedField ? `create.guide.${focusedField}` : 'create.guide.default'
	);
</script>

<div class="guide-panel">
	<div class="guide-help">
		<h3 class="guide-title">{t('create.guide.title')}</h3>
		<p class="guide-text">{t(helpKey)}</p>
	</div>

	<div class="guide-footer">
		<div class="footer-section">
			<span class="footer-label">{t('create.label.cover')}</span>
			<CoverUploader
				mediaId={coverMediaId}
				previewUrl={coverPreviewUrl}
				onUploaded={oncoveruploaded}
				onRemove={oncoverremove}
			/>
		</div>
		<div class="footer-section">
			<span class="footer-label">{t('create.label.series')}</span>
			<SeriesSelector
				bind:series
				{seriesId}
				onselect={(id) => { onseries(id); onedit(); }}
				onfocus={onseriesfocus}
			/>
		</div>
	</div>
</div>

<style>
	.guide-panel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 0.75rem;
	}

	.guide-help {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.guide-title {
		margin: 0;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}

	.guide-text {
		margin: 0;
		font-size: 0.8125rem;
		color: var(--color-muted);
		line-height: 1.5;
	}

	.guide-footer {
		display: flex;
		flex-direction: column;
		gap: 16px;
		margin-top: auto;
		padding-top: 16px;
		border-top: 1px solid var(--color-border);
	}

	.footer-section {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	.footer-label {
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--color-muted);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
</style>
