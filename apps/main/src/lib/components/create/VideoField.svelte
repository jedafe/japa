<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// §106 video composer field — resolves an external video URL or uploads a
	// video file, then links the resulting video_sources row to the draft.
	// On success it fills title/description/featured image and asks the parent
	// to append the backend-built TipTap `video` node to the editor body.
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { Link2, Upload, Loader2, CheckCircle2, Play } from 'lucide-svelte';
	import { resolveVideo, uploadVideo, type VideoResolveResult } from '$lib/api/videos';
	import { formatDuration } from '$lib/utils/duration';
	import type { VideoSource } from '$lib/types';

	interface Props {
		/** Bound composer fields, auto-filled from the resolved source. */
		title?: string;
		description?: string | null;
		featuredImageId?: string | null;
		videoSourceId?: string | null;
		/** §106 video source restored when editing an existing video. */
		initialSource?: VideoSource | null;
		/** Called with the backend-built TipTap doc ({type:'doc',content:[video node]})
		 *  so the parent can append it to the editor body. */
		onBodyAppend?: (bodyJSON: Record<string, unknown>) => void;
		/** Called after a successful resolve/upload with the full source row. */
		onResolved?: (source: VideoSource) => void;
	}

	let {
		title = $bindable(''),
		description = $bindable<string | null>(null),
		featuredImageId = $bindable<string | null>(null),
		videoSourceId = $bindable<string | null>(null),
		initialSource = null,
		onBodyAppend,
		onResolved
	}: Props = $props();

	let linked = $state<VideoSource | null>(untrack(() => initialSource ?? null));

	let tab = $state<'import' | 'upload'>('import');
	let url = $state('');
	let file = $state<File | null>(null);
	let busy = $state(false);
	let error = $state<string | null>(null);

	function applyResult(res: VideoResolveResult) {
		const src = res.source;
		linked = src;
		videoSourceId = src.id;
		// Auto-fill composer fields from the source. The title is only
		// overwritten when the field is still empty so a manually typed
		// title is never clobbered.
		if (src.title && !title.trim()) title = src.title;
		if (src.description && !description?.trim()) description = src.description;
		if (src.thumbnail_media_id) featuredImageId = src.thumbnail_media_id;
		if (res.embed_json) onBodyAppend?.(res.embed_json);
		onResolved?.(src);
	}

	async function handleResolve() {
		const trimmed = url.trim();
		if (!trimmed || busy) return;
		busy = true;
		error = null;
		try {
			const res = await resolveVideo(trimmed);
			if (res.error || !res.data) {
				error = res.error?.message ?? t('create.video.error_generic', 'Could not resolve this video URL');
				return;
			}
			applyResult(res.data);
		} catch {
			error = t('create.video.error_generic', 'Could not resolve this video URL');
		} finally {
			busy = false;
		}
	}

	async function handleUpload() {
		if (!file || busy) return;
		busy = true;
		error = null;
		try {
			const res = await uploadVideo(file, { title: title.trim() || file.name });
			if (res.error || !res.data) {
				error = res.error?.message ?? t('create.video.error_generic', 'Could not upload this video');
				return;
			}
			applyResult(res.data);
		} catch {
			error = t('create.video.error_generic', 'Could not upload this video');
		} finally {
			busy = false;
		}
	}

</script>

<div class="video-field">
	<div class="video-tabs" role="tablist">
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'import'}
			class="video-tab"
			class:active={tab === 'import'}
			onclick={() => (tab = 'import')}
		>
			<Link2 size={14} /> {t('create.video.tab_import', 'Import URL')}
		</button>
		<button
			type="button"
			role="tab"
			aria-selected={tab === 'upload'}
			class="video-tab"
			class:active={tab === 'upload'}
			onclick={() => (tab = 'upload')}
		>
			<Upload size={14} /> {t('create.video.tab_upload', 'Upload')}
		</button>
	</div>

	{#if tab === 'import'}
		<div class="video-row">
			<input
				type="url"
				class="video-input"
				placeholder={t('create.video.url_placeholder', 'Paste a YouTube, Vimeo, TikTok, or Instagram Reels URL…')}
				aria-describedby="video-url-hint"
				bind:value={url}
				disabled={busy}
				onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); handleResolve(); } }}
			/>
			<button type="button" class="video-btn" onclick={handleResolve} disabled={busy || !url.trim()}>
				{#if busy}<Loader2 size={14} class="animate-spin" />{/if}
				{t('create.video.resolve', 'Resolve')}
			</button>
</div>
	{:else}
		<div class="video-row">
			<input
				type="file"
				class="video-input"
				accept="video/*"
				disabled={busy}
				onchange={(e) => { file = e.currentTarget.files?.[0] ?? null; }}
			/>
			<button type="button" class="video-btn" onclick={handleUpload} disabled={busy || !file}>
				{#if busy}<Loader2 size={14} class="animate-spin" />{/if}
				{t('create.video.upload', 'Upload')}
			</button>
		</div>
	{/if}

	{#if error}
		<span class="video-error">{error}</span>
	{/if}

	{#if linked}
		<div class="video-linked">
			{#if linked.thumbnail_url}
				<span class="video-thumb">
					<img src={linked.thumbnail_url} alt="" />
					<span class="video-play"><Play size={16} /></span>
				</span>
			{/if}
			<span class="video-linked-meta">
				<span class="video-linked-title">
					<CheckCircle2 size={14} /> {linked.title || linked.source_url}
				</span>
				<span class="video-linked-sub">
					{linked.source_type}{#if formatDuration(linked.duration_secs)} · {formatDuration(linked.duration_secs)}{/if}
				</span>
			</span>
		</div>
	{/if}
</div>

<style>
	.video-field {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
	}
	.video-tabs {
		display: flex;
		gap: 0.25rem;
	}
	.video-tab {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 6px 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-text-subtle);
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s, background 0.15s;
	}
	.video-tab.active {
		border-color: var(--color-primary);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
	}
	.video-row {
		display: flex;
		gap: 0.5rem;
	}
	.video-input {
		flex: 1;
		min-width: 0;
		padding: 8px 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.video-input:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.video-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		padding: 8px 14px;
		border: none;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast, #fff);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
		white-space: nowrap;
	}
	.video-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.video-error {
		font-size: 0.8125rem;
		color: var(--color-danger, #dc2626);
	}
	.video-linked {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle, transparent);
	}
	.video-thumb {
		position: relative;
		width: 96px;
		aspect-ratio: 16 / 9;
		border-radius: var(--radius);
		overflow: hidden;
		flex-shrink: 0;
		background: #000;
	}
	.video-thumb img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.video-play {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		color: #fff;
		background: rgba(0, 0, 0, 0.35);
	}
	.video-linked-meta {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
		min-width: 0;
	}
	.video-linked-title {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.video-linked-sub {
		font-size: 0.75rem;
		color: var(--color-muted);
		text-transform: capitalize;
	}
</style>
