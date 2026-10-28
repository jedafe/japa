<script lang="ts">
	import { t } from '$lib/i18n';
	import { Switch } from 'bits-ui';
	import { ImagePlus, X, AlertTriangle, CheckCircle2 } from 'lucide-svelte';
	import type { MediaAsset } from '$lib/types';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import { computeSeoScore, SEO_MIN_LENGTHS, SEO_MAX_LENGTHS } from '$lib/utils/seo-score';

	interface Props {
		metaTitle: string | null;
		metaDescription: string | null;
		ogImageUrl: string | null;
		ogImagePreviewUrl: string | null;
		seoIndexable: boolean;
		title: string;
		slug: string;
		canonicalUrl: string;
		visibility: string;
		bodyHtml: string;
		contentType: string;
		onmetatitle: (v: string | null) => void;
		onmetadescription: (v: string | null) => void;
		onogimageselect: (asset: MediaAsset | null) => void;
		onseoindexable: (v: boolean) => void;
		onedit: () => void;
	}

	let {
		metaTitle,
		metaDescription,
		ogImageUrl,
		ogImagePreviewUrl,
		seoIndexable,
		title,
		slug,
		canonicalUrl,
		visibility,
		bodyHtml,
		contentType,
		onmetatitle,
		onmetadescription,
		onogimageselect,
		onseoindexable,
		onedit
	}: Props = $props();

	let ogPickerOpen = $state(false);

	// Live SEO score — recomputed reactively as the user edits any field.
	// Mirrors the backend ValidateSEO scoring (Section 26).
	const seoScore = $derived(
		computeSeoScore({
			title,
			slug,
			metaTitle,
			metaDescription,
			ogImageUrl,
			canonicalUrl,
			seoIndexable,
			visibility,
			bodyHtml,
			contentType
		})
	);

	function scoreColor(score: number): string {
		if (score >= 80) return 'var(--color-success)';
		if (score >= 50) return 'var(--color-warning)';
		return 'var(--color-danger)';
	}

	// Length status for meta_title: 'ok' | 'short' | 'long' | 'empty'
	const metaTitleLenStatus = $derived.by(() => {
		const len = (metaTitle ?? '').trim().length;
		if (len === 0) return 'empty';
		if (len < SEO_MIN_LENGTHS.meta_title) return 'short';
		if (len > SEO_MAX_LENGTHS.meta_title) return 'long';
		return 'ok';
	});

	// Length status for meta_description: 'ok' | 'short' | 'long' | 'empty'
	const metaDescLenStatus = $derived.by(() => {
		const len = (metaDescription ?? '').trim().length;
		if (len === 0) return 'empty';
		if (len < SEO_MIN_LENGTHS.meta_description) return 'short';
		if (len > SEO_MAX_LENGTHS.meta_description) return 'long';
		return 'ok';
	});

	// Human-readable labels for issue codes (i18n keys with inline fallback).
	const ISSUE_LABELS: Record<string, string> = {
		missing_title: t('seo.issue.missing_title', 'Missing title'),
		missing_slug: t('seo.issue.missing_slug', 'Missing slug'),
		missing_meta_title: t('seo.issue.missing_meta_title', 'Missing meta title'),
		missing_meta_description: t('seo.issue.missing_meta_description', 'Missing meta description'),
		missing_og_title: t('seo.issue.missing_og_title', 'Missing OG title'),
		missing_og_description: t('seo.issue.missing_og_description', 'Missing OG description'),
		missing_og_image_url: t('seo.issue.missing_og_image_url', 'Missing OG image'),
		missing_canonical_url: t('seo.issue.missing_canonical_url', 'Missing canonical URL'),
		missing_json_ld_type: t('seo.issue.missing_json_ld_type', 'Missing JSON-LD type'),
		not_indexable: t('seo.issue.not_indexable', 'Not indexable (noindex)'),
		short_title: t('seo.issue.short_title', 'Title too short (min 10 chars)'),
		short_slug: t('seo.issue.short_slug', 'Slug too short (min 3 chars)'),
		short_meta_title: t('seo.issue.short_meta_title', 'Meta title too short (min 30 chars)'),
		short_meta_description: t('seo.issue.short_meta_description', 'Meta description too short (min 70 chars)'),
		short_og_title: t('seo.issue.short_og_title', 'OG title too short (min 30 chars)'),
		short_og_description: t('seo.issue.short_og_description', 'OG description too short (min 70 chars)'),
		long_meta_title: t('seo.issue.long_meta_title', 'Meta title too long (max 60 chars)'),
		long_meta_description: t('seo.issue.long_meta_description', 'Meta description too long (max 160 chars)')
	};

	function handleOgSelect(asset: MediaAsset) {
		ogPickerOpen = false;
		onogimageselect(asset);
	}

	function handleOgRemove() {
		onogimageselect(null);
	}
</script>

<div class="seo-panel">
	<!-- Live SEO score widget -->
	<div class="seo-score-widget">
		<div class="seo-score-ring">
			<svg class="seo-score-svg" viewBox="0 0 48 48">
				<circle cx="24" cy="24" r="20" fill="none" stroke="var(--color-border)" stroke-width="4" />
				<circle
					cx="24" cy="24" r="20" fill="none" stroke={scoreColor(seoScore.score)} stroke-width="4"
					stroke-linecap="round"
					stroke-dasharray={`${(seoScore.score / 100) * 2 * Math.PI * 20} ${2 * Math.PI * 20}`}
				/>
			</svg>
			<span class="seo-score-value" style="color: {scoreColor(seoScore.score)}">{seoScore.score}</span>
		</div>
		<div class="seo-score-info">
			<span class="seo-score-label">{t('create.seo.score', 'SEO Score')}</span>
			{#if seoScore.issues.length === 0}
				<span class="seo-score-ok">
					<CheckCircle2 size={12} />
					{t('create.seo.no_issues', 'All good')}
				</span>
			{:else}
				<span class="seo-score-issues-count">
					{seoScore.issues.length} {t('create.seo.issues_found', 'issue(s)')}
				</span>
				<ul class="seo-score-issues">
					{#each seoScore.issues as issue}
						<li>
							<AlertTriangle size={11} />
							<span>{ISSUE_LABELS[issue] ?? issue}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	</div>

	<div class="field-group">
		<input
			type="text"
			class="field-input"
			placeholder={t('create.label.meta_title')}
			value={metaTitle ?? ''}
			oninput={(e) => {
				onmetatitle(e.currentTarget.value || null);
				onedit();
			}}
		/>
		<div class="field-hint" data-status={metaTitleLenStatus}>
			<span>{(metaTitle ?? '').length} / {SEO_MIN_LENGTHS.meta_title}–{SEO_MAX_LENGTHS.meta_title}</span>
		</div>
	</div>

	<div class="field-group">
		<textarea
			class="field-input field-textarea"
			rows="2"
			placeholder={t('create.label.meta_description')}
			oninput={(e) => {
				onmetadescription(e.currentTarget.value || null);
				onedit();
			}}
		>{metaDescription ?? ''}</textarea>
		<div class="field-hint" data-status={metaDescLenStatus}>
			<span>{(metaDescription ?? '').length} / {SEO_MIN_LENGTHS.meta_description}–{SEO_MAX_LENGTHS.meta_description}</span>
		</div>
	</div>

	{#if ogImagePreviewUrl}
		<div class="og-preview">
			<img src={ogImagePreviewUrl} alt="" class="og-preview-img" />
			<div class="og-preview-actions">
				<button type="button" class="og-action-btn" onclick={() => (ogPickerOpen = true)}>
					{t('media.picker.tab_library')}
				</button>
				<button
					type="button"
					class="og-action-btn og-action-remove"
					onclick={handleOgRemove}
					aria-label={t('create.action.remove_tag')}
				>
					<X size={14} />
				</button>
			</div>
		</div>
	{:else}
		<button type="button" class="og-empty" onclick={() => (ogPickerOpen = true)}>
			<ImagePlus size={20} />
			<span>{t('create.label.og_image')}</span>
		</button>
	{/if}

	<div class="seo-indexable-row">
		<Switch.Root
			checked={seoIndexable}
			onCheckedChange={(v) => {
				onseoindexable(v as boolean);
				onedit();
			}}
			class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors data-[state=checked]:bg-[var(--color-primary)] data-[state=unchecked]:bg-[var(--color-border)]"
		>
			<Switch.Thumb
				class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0.5"
			/>
		</Switch.Root>
		<span class="seo-indexable-label">{t('create.label.seo_indexable')}</span>
	</div>
</div>

{#if ogPickerOpen}
	<MediaLibraryModal onSelect={handleOgSelect} onClose={() => (ogPickerOpen = false)} />
{/if}

<style>
	.seo-panel {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 0.5rem;
	}

	.seo-score-widget {
		display: flex;
		gap: 12px;
		padding: 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
	}
	.seo-score-ring {
		position: relative;
		flex-shrink: 0;
		width: 48px;
		height: 48px;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.seo-score-svg {
		width: 48px;
		height: 48px;
		transform: rotate(-90deg);
	}
	.seo-score-value {
		position: absolute;
		font-size: 0.875rem;
		font-weight: 700;
		line-height: 1;
	}
	.seo-score-info {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
		flex: 1;
	}
	.seo-score-label {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
	}
	.seo-score-ok {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 0.75rem;
		color: var(--color-success);
	}
	.seo-score-issues-count {
		font-size: 0.75rem;
		color: var(--color-muted);
	}
	.seo-score-issues {
		display: flex;
		flex-direction: column;
		gap: 3px;
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.seo-score-issues li {
		display: flex;
		align-items: center;
		gap: 4px;
		font-size: 0.7rem;
		color: var(--color-text);
	}
	.seo-score-issues li :global(svg) {
		color: var(--color-warning);
		flex-shrink: 0;
	}

	.field-input {
		width: 100%;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.field-input::placeholder {
		color: var(--color-muted);
	}
	.field-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.field-textarea {
		resize: vertical;
		min-height: 60px;
		font-family: inherit;
	}

	.field-group {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.field-hint {
		display: flex;
		justify-content: flex-end;
		font-size: 0.7rem;
		color: var(--color-muted);
	}
	.field-hint[data-status='ok'] {
		color: var(--color-success);
	}
	.field-hint[data-status='short'] {
		color: var(--color-warning);
	}
	.field-hint[data-status='long'] {
		color: var(--color-danger);
	}

	.seo-indexable-row {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.seo-indexable-label {
		font-size: 0.875rem;
		color: var(--color-text);
	}

	.og-empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 8px;
		width: 100%;
		aspect-ratio: 16 / 9;
		border: 2px dashed var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-muted);
		font-size: 0.8rem;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}
	.og-empty:hover {
		border-color: var(--color-primary);
		color: var(--color-primary);
	}

	.og-preview {
		position: relative;
		overflow: hidden;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		aspect-ratio: 16 / 9;
	}
	.og-preview-img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.og-preview-actions {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: flex-end;
		justify-content: flex-end;
		gap: 6px;
		padding: 8px;
		background: linear-gradient(to top, rgb(0 0 0 / 0.55) 0%, transparent 50%);
		opacity: 0;
		transition: opacity 0.2s;
	}
	.og-preview:hover .og-preview-actions {
		opacity: 1;
	}
	.og-action-btn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 6px 10px;
		border: 1px solid rgb(255 255 255 / 0.3);
		border-radius: var(--radius);
		background: rgb(0 0 0 / 0.6);
		color: var(--color-surface);
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.15s;
	}
	.og-action-btn:hover {
		background: rgb(0 0 0 / 0.8);
	}
	.og-action-remove {
		padding: 6px;
	}
</style>
