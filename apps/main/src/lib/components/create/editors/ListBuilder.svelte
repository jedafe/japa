<script lang="ts">
	// Ref: docs/superpowers/specs/2026-09-28-list-content-type-design.md (§14 list amendment)
	// List (listicle) composer — numbered/countdown/unnumbered item lists with
	// a per-item visual slot (image XOR internal-content/link/video embed).
	// field_values.items is the editing source; serializeListBodyHtml produces
	// the canonical body_html persisted on the contents row.
	import { onMount, untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import {
		Plus,
		X,
		ChevronUp,
		ChevronDown,
		AlertCircle,
		Image as ImageIcon,
		Link as LinkIcon,
		Video as VideoIcon,
		Loader2
	} from 'lucide-svelte';
	import PostEditor, { type EditorMode } from './PostEditor.svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import Autocomplete, { type AutocompleteItem } from '../Autocomplete.svelte';
	import { api } from '$lib/api/client';
	import { resolveVideo } from '$lib/api/videos';
	import { displayNumber, isEmptyHtml, serializeListBodyHtml } from './listSerialize';
	import type { ListFieldValues, ListItem, ListMode, MediaAsset } from '$lib/types';
	import type { ContentArticle, GuideListItem } from '$lib/types/content';

	type VisualTab = 'image' | 'content' | 'link' | 'video';

	interface LinkPreviewData {
		url: string;
		title: string;
		description: string;
		image: string;
		site_name: string;
		favicon: string;
	}

	/** Internal row wrapper — `key` + per-row UI state are client-only; emit
	 *  maps rows back to plain ListItem[] so nothing leaks into field_values. */
	interface Row {
		key: string;
		item: ListItem;
		/** Which visual-slot tab is open (UI-only). */
		tab: VisualTab;
		/** URL inputs for the link/video tabs (UI-only). */
		linkUrl: string;
		videoUrl: string;
		embedBusy: boolean;
		embedError: string;
	}

	interface Props {
		/** Existing field_values when restoring a list draft. */
		initial: ListFieldValues | null;
		/** Default editor mode (block | wysiwyg | markdown). */
		defaultEditor: EditorMode;
		/** Pushes the field_values payload ({list_mode, intro_html, items}). */
		onvalues: (fv: ListFieldValues) => void;
		/** Pushes the serialized body_html for the contents row. */
		onhtml: (html: string) => void;
		/** Item count for the publish gate (list requires >= 1 item). */
		oncounts: (n: number) => void;
		/** Marks the composer dirty (drives autosave). */
		onedit: () => void;
		/** Feature flags — the video embed tab is hidden when video.enabled
		 *  is off (the resolver endpoint is gated the same way). */
		features?: Record<string, boolean>;
	}

	let { initial, defaultEditor, onvalues, onhtml, oncounts, onedit, features = {} }: Props = $props();

	function toRow(item: ListItem): Row {
		const tab: VisualTab = item.media_id
			? 'image'
			: item.embed?.kind === 'content'
				? 'content'
				: item.embed?.kind === 'url'
					? 'link'
					: item.embed?.kind === 'video'
						? 'video'
						: 'image';
		return {
			key: crypto.randomUUID(),
			item,
			tab,
			linkUrl: item.embed?.kind === 'url' ? (item.embed.url ?? '') : '',
			videoUrl: '',
			embedBusy: false,
			embedError: ''
		};
	}

	let mode = $state<ListMode>(untrack(() => initial?.list_mode ?? 'numbered'));
	let introHtml = $state(untrack(() => initial?.intro_html ?? ''));
	let rows = $state<Row[]>(untrack(() => (initial?.items ?? []).map(toRow)));
	// media_id → public URL, used by the serializer to emit <img src>.
	let mediaUrls = $state<Record<string, string>>({});
	// Row key whose image tab is picking from the media library (modal open).
	let mediaPickerKey = $state<string | null>(null);
	// Autocomplete result cache: item id → row so onSelect can fetch the
	// detail for the snapshot. Plain Map — results are transient, not state.
	let contentPicks = new Map<string, GuideListItem>();

	function emit(markDirty = true) {
		// Untouched TipTap editors serialize as '<p></p>' — strip markup-free
		// HTML so field_values and body_html never store empty paragraphs.
		const items = rows.map((r) => ({
			...r.item,
			body_html: isEmptyHtml(r.item.body_html) ? undefined : r.item.body_html
		}));
		onvalues({ list_mode: mode, intro_html: isEmptyHtml(introHtml) ? '' : introHtml, items });
		onhtml(serializeListBodyHtml(mode, introHtml, items, mediaUrls));
		oncounts(items.length);
		if (markDirty) onedit();
	}

	onMount(() => {
		// The mount-time emit must NOT mark dirty — restoring a draft would
		// otherwise schedule an autosave PATCH identical to stored data.
		emit(false);
		// Resolve media URLs for restored items' media_id so the serializer
		// can emit <img src>; re-emit (still clean) once they're in.
		const ids = [
			...new Set(rows.map((r) => r.item.media_id).filter((v): v is string => !!v))
		];
		if (ids.length > 0) {
			void Promise.all(
				ids.map(async (id) => {
					const res = await api.get<MediaAsset>(`/media/${id}`);
					if (res.data?.url) mediaUrls[id] = res.data.url;
				})
			).then(() => emit(false));
		}
	});

	function setMode(m: ListMode) {
		mode = m;
		emit();
	}

	function addRow() {
		rows = [...rows, toRow({})];
		emit();
	}

	function removeRow(key: string) {
		rows = rows.filter((r) => r.key !== key);
		emit();
	}

	function moveRow(i: number, dir: -1 | 1) {
		const j = i + dir;
		if (j < 0 || j >= rows.length) return;
		const next = rows.slice();
		[next[i], next[j]] = [next[j], next[i]];
		rows = next;
		emit();
	}

	/** media_id XOR embed — setting one clears the other. */
	function clearVisual(row: Row) {
		row.item.media_id = null;
		row.item.embed = null;
		emit();
	}

	function handleMediaSelect(m: MediaAsset) {
		const row = rows.find((r) => r.key === mediaPickerKey);
		if (row) {
			row.item.media_id = m.id;
			mediaUrls[m.id] = m.url;
			row.item.embed = null;
			emit();
		}
		mediaPickerKey = null;
	}

	// Internal-content picker — GET /content?q= only (no type= param: the
	// backend ListContents `type` branch swallows `q`, so search goes out bare).
	async function contentFetcher(q: string): Promise<AutocompleteItem[]> {
		const res = await api.get<GuideListItem[]>(
			`/content?q=${encodeURIComponent(q)}&page_size=10`
		);
		const items = res.data ?? [];
		contentPicks = new Map(items.map((c) => [c.id, c]));
		return items.map((c) => ({ id: c.id, label: c.title, sublabel: c.slug }));
	}

	async function pickContent(row: Row, sel: AutocompleteItem) {
		const hit = contentPicks.get(sel.id);
		if (!hit) return;
		row.embedError = '';
		// Fetch the detail for a durable snapshot (title/excerpt/image) stored
		// in field_values — the placeholder hydrates from slug at render time,
		// the snapshot is the no-JS fallback card baked into body_html.
		const res = await api.get<ContentArticle>(`/content/${hit.slug}`);
		if (res.error || !res.data) {
			row.embedError = res.error?.message ?? t('content.embed.unavailable');
			return;
		}
		const dto = res.data;
		row.item.embed = {
			kind: 'content',
			content_id: dto.id,
			content_slug: dto.slug,
			snapshot: {
				title: dto.title,
				excerpt: dto.excerpt ?? undefined,
				image_url: dto.featured_image_url ?? undefined,
				content_type: dto.content_type
			}
		};
		row.item.media_id = null;
		emit();
	}

	async function fetchLinkPreview(row: Row) {
		const url = row.linkUrl.trim();
		if (!url || row.embedBusy) return;
		row.embedBusy = true;
		row.embedError = '';
		const res = await api.get<LinkPreviewData>(`/link-preview?url=${encodeURIComponent(url)}`);
		row.embedBusy = false;
		if (res.error || !res.data) {
			row.embedError = res.error?.message ?? t('content.embed.unavailable');
			return;
		}
		const p = res.data;
		// GET /link-preview returns {url,title,description,image,site_name,
		// favicon} — image maps to resolved.image_url; no embed_url exists
		// (spec reconciliation 4), so url embeds always serialize as
		// link-preview placeholders in v1.
		row.item.embed = {
			kind: 'url',
			url,
			resolved: {
				title: p.title,
				description: p.description,
				image_url: p.image,
				site_name: p.site_name
			}
		};
		row.item.media_id = null;
		emit();
	}

	async function resolveVideoEmbed(row: Row) {
		const url = row.videoUrl.trim();
		if (!url || row.embedBusy) return;
		row.embedBusy = true;
		row.embedError = '';
		const res = await resolveVideo(url);
		row.embedBusy = false;
		if (res.error || !res.data) {
			// 403 = caller lacks video.import.single (auth-gated endpoint).
			row.embedError =
				res.status === 403
					? t('create.list.embed.video_denied')
					: (res.error?.message ?? t('content.embed.unavailable'));
			return;
		}
		row.item.embed = {
			kind: 'video',
			video_source_id: res.data.source.id,
			resolved: {
				title: res.data.source.title ?? undefined,
				image_url: res.data.source.thumbnail_url ?? undefined
			}
		};
		row.item.media_id = null;
		emit();
	}

	const MODE_OPTIONS: ListMode[] = ['numbered', 'countdown', 'unnumbered'];
	const VISUAL_TABS = $derived.by(() => {
		const tabs: { value: VisualTab; labelKey: string }[] = [
			{ value: 'image', labelKey: 'create.list.embed.image' },
			{ value: 'content', labelKey: 'create.list.embed.content' },
			{ value: 'link', labelKey: 'create.list.embed.link' }
		];
		// video.enabled gates both the resolver endpoint and the Videos
		// studio tab — hide the embed tab to match rather than offer a
		// picker that can only fail.
		if (features['video.enabled'] !== false) {
			tabs.push({ value: 'video', labelKey: 'create.list.embed.video' });
		}
		return tabs;
	});

	/** Short label for the current visual chip. */
	function visualLabel(row: Row): string {
		const e = row.item.embed;
		if (row.item.media_id) return mediaUrls[row.item.media_id] ?? row.item.media_id;
		if (e?.kind === 'content') return e.snapshot?.title ?? e.content_slug ?? '';
		if (e?.kind === 'url') return e.resolved?.title ?? e.url ?? '';
		if (e?.kind === 'video') return e.resolved?.title ?? e.video_source_id ?? '';
		return '';
	}
</script>

<div class="list-builder">
	<!-- Introduction — rich text above the list items (intro_html). -->
	<div class="intro-section">
		<span class="field-label">{t('create.list.intro_label')}</span>
		<PostEditor
			html={introHtml}
			defaultMode={defaultEditor}
			onhtml={(v) => {
				introHtml = v;
				emit(false);
			}}
			onedit={() => emit(true)}
		/>
	</div>

	<!-- Mode segmented control — numbered / countdown / unnumbered. -->
	<div class="mode-row">
		<span class="field-label">{t('create.list.mode')}</span>
		<div class="mode-tabs" role="group" aria-label={t('create.list.mode')}>
			{#each MODE_OPTIONS as m (m)}
				<button
					type="button"
					class="mode-btn"
					class:active={mode === m}
					aria-pressed={mode === m}
					onclick={() => setMode(m)}
				>
					{t(`create.list.mode.${m}`)}
				</button>
			{/each}
		</div>
	</div>

	{#each rows as row, i (row.key)}
		<div class="list-item-card">
			<div class="item-head">
				<span class="item-badge" aria-hidden="true">{displayNumber(mode, i, rows.length)}</span>
				<input
					type="text"
					class="item-heading"
					placeholder={t('create.list.item_heading')}
					value={row.item.heading ?? ''}
					oninput={(e) => {
						row.item.heading = e.currentTarget.value;
						emit();
					}}
				/>
				<div class="item-actions">
					<button
						type="button"
						class="icon-btn"
						title={t('create.list.move_up', 'Move up')}
						aria-label={t('create.list.move_up', 'Move up')}
						disabled={i === 0}
						onclick={() => moveRow(i, -1)}
					>
						<ChevronUp size={14} />
					</button>
					<button
						type="button"
						class="icon-btn"
						title={t('create.list.move_down', 'Move down')}
						aria-label={t('create.list.move_down', 'Move down')}
						disabled={i === rows.length - 1}
						onclick={() => moveRow(i, 1)}
					>
						<ChevronDown size={14} />
					</button>
					<button
						type="button"
						class="icon-btn danger"
						title={t('create.list.remove_item')}
						aria-label={t('create.list.remove_item')}
						onclick={() => removeRow(row.key)}
					>
						<X size={14} />
					</button>
				</div>
			</div>

			<div class="item-body">
				<span class="field-label">{t('create.list.item_body')}</span>
				<PostEditor
					html={row.item.body_html ?? ''}
					defaultMode={defaultEditor}
					onhtml={(v) => {
						row.item.body_html = v;
						emit(false);
					}}
					onedit={() => emit(true)}
				/>
			</div>

			<!-- One visual slot per item: image XOR embed. -->
			<div class="visual-slot">
				<div class="visual-tabs" role="group">
					{#each VISUAL_TABS as tab (tab.value)}
						<button
							type="button"
							class="mode-btn"
							class:active={row.tab === tab.value}
							aria-pressed={row.tab === tab.value}
							onclick={() => (row.tab = tab.value)}
						>
							{t(tab.labelKey)}
						</button>
					{/each}
				</div>

				{#if row.item.media_id || row.item.embed}
					<div class="visual-chip">
						{#if row.item.media_id && mediaUrls[row.item.media_id]}
							<img class="visual-thumb" src={mediaUrls[row.item.media_id]} alt="" />
						{:else if row.item.embed?.kind === 'video' && row.item.embed.resolved?.image_url}
							<img class="visual-thumb" src={row.item.embed.resolved.image_url} alt="" />
						{:else if row.item.embed?.kind === 'content' && row.item.embed.snapshot?.image_url}
							<img class="visual-thumb" src={row.item.embed.snapshot.image_url} alt="" />
						{:else}
							<LinkIcon size={14} />
						{/if}
						<span class="visual-label">{visualLabel(row)}</span>
						<button
							type="button"
							class="icon-btn"
							title={t('common.remove')}
							aria-label={t('common.remove')}
							onclick={() => clearVisual(row)}
						>
							<X size={13} />
						</button>
					</div>
				{/if}

				{#if row.embedError}
					<div class="api-error" role="alert">
						<AlertCircle size={14} />
						<span>{row.embedError}</span>
					</div>
				{/if}

				{#if row.tab === 'image'}
					<button type="button" class="add-btn" onclick={() => (mediaPickerKey = row.key)}>
						<ImageIcon size={14} />
						{t('create.list.embed.image')}
					</button>
				{:else if row.tab === 'content'}
					<Autocomplete
						placeholder={t('create.list.embed.search')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={contentFetcher}
						selectedId={row.item.embed?.kind === 'content' ? (row.item.embed.content_id ?? null) : null}
						selectedLabel={row.item.embed?.kind === 'content' ? (row.item.embed.snapshot?.title ?? null) : null}
						onSelect={(sel) => void pickContent(row, sel)}
						onClear={() => {
							if (row.item.embed?.kind === 'content') {
								row.item.embed = null;
								emit();
							}
						}}
					/>
				{:else if row.tab === 'link'}
					<div class="url-row">
						<LinkIcon size={14} class="url-icon" />
						<input
							type="url"
							class="url-input"
							placeholder="https://…"
							bind:value={row.linkUrl}
							onkeydown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault();
									void fetchLinkPreview(row);
								}
							}}
						/>
						<button
							type="button"
							class="add-btn"
							disabled={row.embedBusy || !row.linkUrl.trim()}
							onclick={() => void fetchLinkPreview(row)}
						>
							{#if row.embedBusy}<Loader2 size={14} class="spin" />{/if}
							{t('create.list.embed.fetch')}
						</button>
					</div>
				{:else if row.tab === 'video'}
					<div class="url-row">
						<VideoIcon size={14} class="url-icon" />
						<input
							type="url"
							class="url-input"
							placeholder="https://…"
							bind:value={row.videoUrl}
							onkeydown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault();
									void resolveVideoEmbed(row);
								}
							}}
						/>
						<button
							type="button"
							class="add-btn"
							disabled={row.embedBusy || !row.videoUrl.trim()}
							onclick={() => void resolveVideoEmbed(row)}
						>
							{#if row.embedBusy}<Loader2 size={14} class="spin" />{/if}
							{t('create.list.embed.fetch')}
						</button>
					</div>
				{/if}
			</div>

			<div class="item-meta">
				<input
					type="url"
					class="meta-input"
					placeholder={t('create.list.source_url')}
					value={row.item.source_url ?? ''}
					oninput={(e) => {
						row.item.source_url = e.currentTarget.value;
						emit();
					}}
				/>
				<input
					type="text"
					class="meta-input"
					placeholder={t('create.list.credit')}
					value={row.item.credit ?? ''}
					oninput={(e) => {
						row.item.credit = e.currentTarget.value;
						emit();
					}}
				/>
			</div>
		</div>
	{/each}

	<button type="button" class="add-btn add-item-btn" onclick={addRow}>
		<Plus size={14} />
		{t('create.list.add_item')}
	</button>
</div>

{#if mediaPickerKey}
	<MediaLibraryModal
		allowedType="image"
		onSelect={handleMediaSelect}
		onClose={() => (mediaPickerKey = null)}
	/>
{/if}

<style>
	.list-builder {
		display: flex;
		flex-direction: column;
		gap: 12px;
	}

	.field-label {
		font-size: 0.6875rem;
		font-weight: 500;
		color: var(--color-muted);
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.intro-section,
	.item-body {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.mode-row {
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.mode-tabs,
	.visual-tabs {
		display: inline-flex;
		gap: 2px;
		padding: 2px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
	}
	.mode-btn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 5px 10px;
		border: none;
		border-radius: calc(var(--radius) - 2px);
		background: transparent;
		color: var(--color-text-subtle);
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.12s;
	}
	.mode-btn:hover {
		color: var(--color-text);
	}
	.mode-btn.active {
		background: var(--color-surface);
		color: var(--color-primary);
		box-shadow: 0 1px 2px rgb(0 0 0 / 0.08);
	}

	.list-item-card {
		display: flex;
		flex-direction: column;
		gap: 10px;
		padding: 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
	}

	.item-head {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.item-badge {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 26px;
		height: 26px;
		padding: 0 6px;
		border-radius: 999px;
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-primary);
		font-size: 0.75rem;
		font-weight: 700;
		flex-shrink: 0;
	}
	.item-heading {
		flex: 1;
		min-width: 0;
		height: 2rem;
		padding: 0 10px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		font-weight: 600;
		outline: none;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.item-heading:focus {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}

	.item-actions {
		display: flex;
		align-items: center;
		gap: 2px;
		flex-shrink: 0;
	}
	.icon-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 26px;
		height: 26px;
		border: none;
		border-radius: 6px;
		background: transparent;
		color: var(--color-text-subtle);
		cursor: pointer;
		transition: background 0.12s, color 0.12s;
	}
	.icon-btn:hover:not(:disabled) {
		background: var(--color-surface-subtle);
		color: var(--color-text);
	}
	.icon-btn.danger:hover:not(:disabled) {
		color: var(--color-danger);
	}
	.icon-btn:disabled {
		opacity: 0.35;
		cursor: not-allowed;
	}

	.visual-slot {
		display: flex;
		flex-direction: column;
		gap: 8px;
		padding-top: 4px;
		border-top: 1px dashed var(--color-border);
	}

	.visual-chip {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 6px 8px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface-subtle);
		font-size: 0.8125rem;
		color: var(--color-text);
	}
	.visual-thumb {
		width: 40px;
		height: 28px;
		object-fit: cover;
		border-radius: 4px;
		flex-shrink: 0;
	}
	.visual-label {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.url-row {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 0 8px 0 12px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		transition: border-color 0.15s;
	}
	.url-row:focus-within {
		border-color: var(--color-primary);
	}
	.url-input {
		flex: 1;
		min-width: 0;
		height: 2.25rem;
		border: none;
		background: transparent;
		color: var(--color-text);
		font-size: 0.875rem;
		outline: none;
	}
	.url-input::placeholder {
		color: var(--color-text-subtle);
	}
	.url-row :global(.url-icon) {
		flex-shrink: 0;
		color: var(--color-text-subtle);
	}

	.item-meta {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 8px;
	}
	.meta-input {
		height: 2rem;
		padding: 0 10px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.8125rem;
		outline: none;
		transition: border-color 0.15s;
	}
	.meta-input:focus {
		border-color: var(--color-primary);
	}
	.meta-input::placeholder {
		color: var(--color-text-subtle);
	}

	.add-btn {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		padding: 6px 10px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text-subtle);
		font-size: 0.75rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.12s;
	}
	.add-btn:hover:not(:disabled) {
		background: var(--color-surface-subtle);
		border-color: var(--color-primary);
		color: var(--color-primary);
	}
	.add-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.add-item-btn {
		align-self: flex-start;
		padding: 8px 14px;
	}

	.api-error {
		display: flex;
		align-items: center;
		gap: 6px;
		padding: 8px 12px;
		border-radius: var(--radius);
		background: color-mix(in srgb, #ef4444 10%, transparent);
		border: 1px solid color-mix(in srgb, #ef4444 30%, transparent);
		color: #dc2626;
		font-size: 0.8125rem;
	}

	.list-builder :global(.spin) {
		animation: spin 0.8s linear infinite;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	@media (max-width: 640px) {
		.item-meta {
			grid-template-columns: 1fr;
		}
	}
</style>
