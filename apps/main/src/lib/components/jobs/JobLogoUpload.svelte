<script lang="ts">
	// Company logo picker for the job composer — stores the selected media
	// URL in field_values.company_logo (Section 63, migration 000734). When
	// unset, finder cards fall back to the poster org/user avatar.
	//
	// Ref: japaconnect-engineering/sections/63-jobs-board.md
	import { t } from '$lib/i18n';
	import { ImagePlus, X } from 'lucide-svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import { remoteImageUrl } from '$lib/utils/remoteImage';
	import type { MediaAsset } from '$lib/types';

	interface Props {
		values: Record<string, unknown>;
		onvalues: (v: Record<string, unknown>) => void;
		onedit: () => void;
	}

	let { values, onvalues, onedit }: Props = $props();

	const logoUrl = $derived(typeof values.company_logo === 'string' ? values.company_logo : '');
	let pickerOpen = $state(false);

	function setLogo(url: string) {
		onvalues({ ...values, company_logo: url });
		onedit();
	}

	function handleSelect(asset: MediaAsset) {
		pickerOpen = false;
		if (asset.url) setLogo(asset.url);
	}
</script>

<div class="logo-field">
	<span class="logo-label">{t('jobs.company_logo', 'Company logo')}</span>
	<div class="logo-row">
		{#if logoUrl}
			<img src={remoteImageUrl(logoUrl)} alt={t('jobs.company_logo', 'Company logo')} class="logo-preview" />
		{/if}
		<button type="button" onclick={() => (pickerOpen = true)} class="logo-button">
			<ImagePlus size={14} />
			{logoUrl ? t('jobs.logo_replace', 'Replace logo') : t('jobs.logo_upload', 'Upload logo')}
		</button>
		{#if logoUrl}
			<button
				type="button"
				onclick={() => setLogo('')}
				class="logo-remove"
				aria-label={t('common.remove', 'Remove')}
			>
				<X size={14} />
			</button>
		{/if}
	</div>
</div>

{#if pickerOpen}
	<MediaLibraryModal allowedType="image" onSelect={handleSelect} onClose={() => (pickerOpen = false)} />
{/if}

<style>
	.logo-field {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}
	.logo-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.logo-row {
		display: flex;
		align-items: center;
		gap: 10px;
	}
	.logo-preview {
		height: 3rem;
		width: 3rem;
		border-radius: 0.75rem;
		border: 1px solid var(--color-border);
		background: var(--color-surface);
		object-fit: contain;
	}
	.logo-button {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 2.375rem;
		padding: 0 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		cursor: pointer;
		transition: border-color 0.15s;
	}
	.logo-button:hover {
		border-color: var(--color-primary);
	}
	.logo-remove {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		height: 2rem;
		width: 2rem;
		border-radius: var(--radius);
		color: var(--color-muted);
	}
	.logo-remove:hover {
		color: var(--color-danger, #dc2626);
		background: var(--color-surface-subtle);
	}
</style>
