<script lang="ts">
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { api, docsApi } from '#lib/api.js';
	import { beforeNavigate, goto } from '$app/navigation';
	import { Select } from '#lib/components/ui/select/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import Autocomplete from '#lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '#lib/components/ui/Autocomplete.svelte';
	import MediaPickerModal from '#lib/components/ui/MediaPickerModal.svelte';
	import { RichTextEditor } from '@japaconnect/ui';
	import { Image as ImageIcon } from 'lucide-svelte';
	import { onMount } from 'svelte';

	type DocPage = {
		id: string;
		title: string;
		slug: string;
		audience: string;
		status?: string;
		review_status?: string;
		category?: string | null;
		subcategory?: string | null;
		parent_id?: string | null;
		position?: number;
		content?: unknown;
		feature_image_id?: string | null;
		feature_image_url?: string | null;
		updated_at?: string;
	};

	let { docId = null, onsaved, oncancel }: {
		docId?: string | null;
		onsaved?: (id: string) => void;
		oncancel?: () => void;
	} = $props();

	const canPublish = $derived(hasPermission('docs.admin'));

	let saving = $state(false);
	let errorMessage = $state('');

	let form = $state({
		title: '', slug: '', audience: 'users', status: 'draft',
		category: '', subcategory: '', parentId: '', position: 0,
		featureImageId: '', featureImageUrl: '', summary: ''
	});

	// Edit-mode state — the detail fetch populates the body + fresh lock token.
	let editingOriginalStatus = $state('');
	// Optimistic lock token — sent as expected_updated_at so a stale admin
	// form can't silently overwrite a concurrent docs-site save.
	let editingUpdatedAt = $state('');
	// True once the detail fetch populated the body — guards submit from
	// writing an empty doc over stored content when the fetch failed.
	let editContentReady = $state(false);
	let editSeed = $state<unknown>(null);
	let editorJson = $state<unknown>(null);
	// The editor fires onjson on its initial seed — skip it so the dirty
	// guard doesn't flag an untouched form as edited.
	let seedJsonSeen = $state(false);
	let rte: RichTextEditor | undefined = $state();
	let loadError = $state('');

	let pendingImageInsert: ((src: string, alt?: string) => void) | null = null;
	let bodyPickerOpen = $state(false);
	let featurePickerOpen = $state(false);

	// DOC-009: audience/status options are loaded from the API.
	let audiences = $state<string[]>(['users', 'premium', 'organizations', 'moderators', 'api', 'admin', 'superadmin']);
	let statuses = $state<string[]>(['draft', 'in_review', 'approved', 'published']);

	// §44 amendment 2: PATCH may only set draft/in_review — approve/publish are
	// dedicated endpoints (and record the reviewer). docs.admin holders can
	// still *select* approved/published here — submit() chains the dedicated
	// endpoints after the write. Backward transitions are not offered.
	const statusOptions = $derived.by(() => {
		const current = docId ? editingOriginalStatus : '';
		const forward: string[] = [];
		if (canPublish) {
			if (current !== 'published' && current !== 'approved') forward.push('approved');
			if (current !== 'published') forward.push('published');
		}
		const all = [...new Set([...statuses.filter((s) => s === 'draft' || s === 'in_review'), ...(current ? [current] : []), ...forward])];
		return all.map((s) => ({
			value: s,
			label: t(`docs.status.${s}`, s.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()))
		}));
	});

	const audienceOptions = $derived(audiences.map((a) => ({ value: a, label: a })));

	// Category/subcategory suggestions — the managed catalog (docs.categories,
	// public settings) merged with the taxonomy already used by sibling pages
	// in the selected audience. Fields stay free-text; these are hints only.
	let managedCats = $state<{ name: string; subcategories: string[] }[]>([]);

	const categorySuggestions = $derived.by(() => {
		const seen = new Map<string, string>();
		for (const c of managedCats.map((m) => m.name)) if (c.trim()) seen.set(c.toLowerCase(), c);
		for (const d of parentCandidates) if (d.category?.trim()) seen.set(d.category.toLowerCase(), d.category.trim());
		return [...seen.values()];
	});

	function subSuggestionsFor(category: string): string[] {
		const cat = category.trim().toLowerCase();
		const seen = new Map<string, string>();
		const managedSubs = cat
			? (managedCats.find((c) => c.name.toLowerCase() === cat)?.subcategories ?? [])
			: managedCats.flatMap((c) => c.subcategories);
		for (const s of managedSubs) if (s.trim()) seen.set(s.toLowerCase(), s);
		for (const d of parentCandidates) {
			if (cat && d.category?.trim().toLowerCase() !== cat) continue;
			if (d.subcategory?.trim()) seen.set(d.subcategory.toLowerCase(), d.subcategory.trim());
		}
		return [...seen.values()];
	}
	const subcategorySuggestions = $derived(subSuggestionsFor(form.category));

	const toItems = (list: string[]): AutocompleteItem[] => list.map((s) => ({ id: s, label: s }));
	const filterItems = (list: string[], q: string) =>
		Promise.resolve(toItems(list.filter((s) => s.toLowerCase().includes(q.toLowerCase()))));

	const categoryFetcher = (q: string) => filterItems(categorySuggestions, q);
	const categoryInitial = () => Promise.resolve(toItems(categorySuggestions));
	const subcategoryFetcher = (q: string) => filterItems(subcategorySuggestions, q);
	const subcategoryInitial = () => Promise.resolve(toItems(subcategorySuggestions));

	// Parent page select — siblings in the same audience (the backend enforces
	// audience match in validateParent), minus the page being edited. Fetched
	// unfiltered so no list filter can silently remove valid parents.
	let parentCandidates = $state<DocPage[]>([]);
	// Audience the current parentCandidates list was fetched for — the
	// invalidation effect must not clear form.parentId on a stale list.
	let parentLoadedFor = $state('');

	async function loadParentCandidates(audience: string) {
		parentLoadedFor = '';
		const res = await docsApi.list(`audience=${encodeURIComponent(audience)}&limit=0`);
		if (res.ok) {
			parentCandidates = res.data?.data ?? [];
		}
		parentLoadedFor = audience;
	}

	$effect(() => {
		const aud = form.audience;
		if (aud) loadParentCandidates(aud);
	});

	const parentPages = $derived(parentCandidates.filter((d) => d.id !== docId));
	const parentFetcher = (q: string) =>
		Promise.resolve(
			parentPages
				.filter((d) => d.title.toLowerCase().includes(q.toLowerCase()))
				.map((d) => ({ id: d.id, label: d.title, sublabel: d.slug }))
		);
	const parentInitial = () =>
		Promise.resolve(parentPages.map((d) => ({ id: d.id, label: d.title, sublabel: d.slug })));

	// Changing the audience invalidates the parent choice — the backend
	// requires the parent to live in the same audience (validateParent).
	// Gated on the candidate fetch completing so an in-flight load can't
	// clear a legit parent during the empty-list window.
	$effect(() => {
		if (form.parentId && parentLoadedFor === form.audience && !parentPages.some((p) => p.id === form.parentId)) {
			form.parentId = '';
		}
	});

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	async function submit() {
		if (!form.title.trim()) return;
		saving = true;
		errorMessage = '';
		// Flush any pending editor transaction so the JSON reflects the latest
		// keystroke before it ships.
		rte?.flush();
		const slug = form.slug.trim() || slugify(form.title);
		// §44 amendment 2: only draft/in_review are writable via create/PATCH.
		// Selecting approved/published omits review_status from the write and
		// chains the dedicated approve/publish endpoints afterwards so reviewer
		// metadata stays truthful. Empty category/subcategory/parent_id clear
		// the stored value (preserve-on-absent → send "" to clear).
		const desired = form.status;
		const writable = desired === 'approved' || desired === 'published' ? undefined : desired;
		// editorJson tracks every edit (the editor also fires it on seed) —
		// falling back to editSeed covers the untouched case.
		const content = docId
			? editContentReady
				? (editorJson ?? editSeed ?? '')
				: undefined
			: (editorJson ?? '');
		const payload = {
			title: form.title.trim(),
			slug,
			audience: form.audience,
			// On update without a successful detail fetch, omit content so the
			// stored document is preserved rather than overwritten with an
			// empty doc.
			content,
			review_status: writable,
			category: form.category.trim(),
			subcategory: form.subcategory.trim(),
			parent_id: form.parentId,
			position: Math.max(0, Math.trunc(form.position) || 0),
			feature_image_id: form.featureImageId,
			...(form.summary.trim() ? { summary: form.summary.trim() } : {}),
			// Optimistic lock — the docs-site editor uses the same contract.
			...(docId && editingUpdatedAt ? { expected_updated_at: editingUpdatedAt } : {})
		};
		const result = docId ? await docsApi.update(docId, payload) : await docsApi.create(payload);
		if (result.ok) {
			// Chain the dedicated status endpoints for forward transitions.
			// Both endpoints are idempotent for their target state, so call only
			// when the transition is actually needed.
			const newId = docId ?? (result.data as { id?: string })?.id;
			const prior = docId ? editingOriginalStatus : '';
			if (newId && desired === 'approved' && prior !== 'approved' && prior !== 'published') {
				const r = await docsApi.approve(newId);
				if (!r.ok) errorMessage = t('docs.saved_transition_failed', `Saved, but approve failed: ${r.error}`);
			} else if (newId && desired === 'published' && prior !== 'published') {
				if (prior !== 'approved') {
					const r = await docsApi.approve(newId);
					if (!r.ok) errorMessage = t('docs.saved_transition_failed', `Saved, but approve failed: ${r.error}`);
				}
				if (!errorMessage) {
					const r = await docsApi.publish(newId);
					if (!r.ok) errorMessage = t('docs.saved_transition_failed', `Saved, but publish failed: ${r.error}`);
				}
			}
			saving = false;
			if (newId && !errorMessage) {
				markClean();
				onsaved?.(newId);
			}
			return;
		}
		errorMessage = result.error;
		saving = false;
	}

	// Dirty guard — a real page (unlike the old modal) loses work on any
	// accidental navigation, so warn before leaving with unsaved edits.
	// dirty is DERIVED from a diff against the seeded snapshot: the seed
	// itself (empty create form or the fetched page) is clean by definition.
	let seededForm = $state<typeof form | null>(null);
	let editorDirty = $state(false);
	const dirty = $derived.by(() => {
		if (editorDirty) return true;
		const s = seededForm;
		if (!s) return false;
		return form.title !== s.title || form.slug !== s.slug || form.audience !== s.audience
			|| form.status !== s.status || form.category !== s.category || form.subcategory !== s.subcategory
			|| form.parentId !== s.parentId || form.position !== s.position
			|| form.featureImageId !== s.featureImageId;
	});
	function markClean() {
		seededForm = { ...form };
		editorDirty = false;
	}
	beforeNavigate((nav) => {
		if (!dirty || saving) return;
		if (!confirm(t('docs.discard_changes', 'Discard unsaved changes?'))) nav.cancel();
	});

	onMount(async () => {
		const [audResult, statusResult, pub] = await Promise.all([
			docsApi.audiences(),
			docsApi.statuses(),
			api<Record<string, unknown>>('/site/settings/public')
		]);
		if (audResult.ok && Array.isArray(audResult.data) && audResult.data.length > 0) {
			audiences = audResult.data;
		}
		if (statusResult.ok && Array.isArray(statusResult.data) && statusResult.data.length > 0) {
			statuses = statusResult.data;
		}
		if (pub.ok) {
			const raw = pub.data?.['docs.categories'];
			if (Array.isArray(raw)) {
				managedCats = raw
					.filter((c): c is { name: string; subcategories?: unknown[] } => !!c && typeof c.name === 'string')
					.map((c) => ({
						name: c.name,
						subcategories: Array.isArray(c.subcategories) ? c.subcategories.filter((s): s is string => typeof s === 'string') : []
					}));
			}
		}

		if (!docId) {
			// Create mode — the empty form is the clean baseline.
			seededForm = { ...form };
			return;
		}
		// Edit mode — one detail fetch seeds every field plus the body editor.
		const detail = await docsApi.get(docId);
		if (detail.ok && detail.data) {
			const d = detail.data as DocPage & { review_status?: string };
			const status = d.review_status ?? d.status ?? 'draft';
			form = {
				title: d.title,
				slug: d.slug,
				audience: d.audience,
				status,
				category: d.category ?? '',
				subcategory: d.subcategory ?? '',
				parentId: d.parent_id ?? '',
				position: d.position ?? 0,
				featureImageId: d.feature_image_id ?? '',
				featureImageUrl: d.feature_image_url ?? '',
				summary: ''
			};
			editingOriginalStatus = status;
			editingUpdatedAt = d.updated_at ?? '';
			editSeed = d.content ?? null;
			editContentReady = true;
			// The fetched values are the clean baseline for the dirty guard.
			seededForm = { ...form };
		} else {
			loadError = ('error' in detail ? detail.error : '') || t('docs.load_failed', 'Failed to load the page.');
		}
	});
</script>

<svelte:window onbeforeunload={(e) => { if (dirty) e.preventDefault(); }} />

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
{#if loadError}
	<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{loadError}
	</div>
{:else}
	<div class="space-y-3">
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-title" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.title_2')}</label>
				<input id="doc-title" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.title} oninput={() => { if (!docId) form.slug = slugify(form.title); }} />
			</div>
			<div>
				<label for="doc-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.slug')}</label>
				<input id="doc-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.slug} />
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-audience" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.audience')}</label>
				<Select options={audienceOptions} bind:value={form.audience} />
			</div>
			<div>
				<label for="doc-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.status')}</label>
				<Select options={statusOptions} bind:value={form.status} />
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-category" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.category', 'Category')}</label>
				<Autocomplete
					id="doc-category"
					placeholder={t('docs.category_ph', 'Category (optional)')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={categoryFetcher}
					initialFetcher={categoryInitial}
					selectedId={form.category || null}
					selectedLabel={form.category || null}
					freeText
					allowFreeText
					onSelect={(item) => { form.category = item.label; }}
					onInput={(q) => { form.category = q; }}
					onFreeTextBlur={(q) => { form.category = q.trim(); }}
				/>
			</div>
			<div>
				<label for="doc-subcategory" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.subcategory', 'Subcategory')}</label>
				<Autocomplete
					id="doc-subcategory"
					disabled={!form.category.trim()}
					placeholder={form.category.trim() ? t('docs.subcategory_ph', 'Subcategory (optional)') : t('docs.subcategory_pick_cat', 'Pick a category first')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={subcategoryFetcher}
					initialFetcher={subcategoryInitial}
					selectedId={form.subcategory || null}
					selectedLabel={form.subcategory || null}
					freeText
					allowFreeText
					onSelect={(item) => { form.subcategory = item.label; }}
					onInput={(q) => { form.subcategory = q; }}
					onFreeTextBlur={(q) => { form.subcategory = q.trim(); }}
				/>
			</div>
		</div>
		<div class="grid gap-3 sm:grid-cols-2">
			<div>
				<label for="doc-parent" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.parent_page', 'Parent page')}</label>
				<Autocomplete
					id="doc-parent"
					placeholder={t('docs.parent_ph', '— Top level —')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={parentFetcher}
					initialFetcher={parentInitial}
					selectedId={form.parentId || null}
					selectedLabel={parentPages.find((p) => p.id === form.parentId)?.title ?? null}
					onSelect={(item) => { form.parentId = item.id; }}
					onClear={() => { form.parentId = ''; }}
				/>
			</div>
			<div>
				<label for="doc-position" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.position', 'Position')}</label>
				<input id="doc-position" type="number" min="0" step="1" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.position} />
			</div>
		</div>
		<!-- Feature image (§44 amendment 5) — media-library backed, URL is
		     computed server-side. -->
		<div>
			<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.feature_image', 'Feature image')}</span>
			<div class="flex items-center gap-3">
				{#if form.featureImageUrl}
					<img src={form.featureImageUrl} alt="" class="h-14 w-24 rounded-md border object-cover" style="border-color: var(--color-border)" />
				{/if}
				<Button variant="outline" onclick={() => { featurePickerOpen = true; }}>
					<ImageIcon size={14} class="mr-1 inline-block" />
					{form.featureImageId ? t('docs.change_image', 'Change image') : t('docs.choose_image', 'Choose image')}
				</Button>
				{#if form.featureImageId}
					<button type="button" class="text-xs" style="color: var(--color-muted)" onclick={() => { form.featureImageId = ''; form.featureImageUrl = ''; }}>
						{t('common.clear', 'Clear')}
					</button>
				{/if}
			</div>
		</div>
		<div>
			<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.body_label', 'Body')}</span>
			{#if docId && !editContentReady}
				<div class="rounded-md border px-3 py-8 text-center text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
					{t('docs.body_loading', 'Loading page content…')}
				</div>
			{:else}
				<RichTextEditor
					bind:this={rte}
					content={editSeed}
					{t}
					placeholder={t('docs.body_placeholder', 'Write the documentation…')}
					ariaLabel={t('docs.body_label', 'Body')}
					defaultMode="wysiwyg"
					modes={['wysiwyg', 'markdown', 'block']}
					minHeight={420}
					features={{
						image: true,
						youtube: true,
						markdown: true,
						suggestions: false,
						coupon: false,
						linkPreview: false,
						video: false
					}}
					onjson={(json) => {
						editorJson = json;
						if (seedJsonSeen) editorDirty = true;
						else seedJsonSeen = true;
					}}
					onPickImage={(insert) => {
						pendingImageInsert = insert;
						bodyPickerOpen = true;
					}}
				/>
			{/if}
		</div>
		{#if docId}
			<div>
				<label for="doc-summary" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('docs.summary_label', 'Edit summary (optional)')}</label>
				<input id="doc-summary" type="text" maxlength="200" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.summary} placeholder={t('docs.summary_placeholder', 'Briefly describe your changes')} />
			</div>
		{/if}
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" onclick={() => { markClean(); oncancel ? oncancel() : goto('/docs'); }}>{t('docs.cancel')}</Button>
			<Button onclick={submit} disabled={saving}>
				{saving ? t('docs.saving') : (docId ? t('docs.update') : t('docs.create'))}
			</Button>
		</div>
	</div>
{/if}

<MediaPickerModal
	bind:open={bodyPickerOpen}
	onselect={(media) => {
		pendingImageInsert?.(media.url, media.filename);
		pendingImageInsert = null;
		bodyPickerOpen = false;
	}}
/>
<MediaPickerModal
	bind:open={featurePickerOpen}
	onselect={(media) => {
		form.featureImageId = media.id;
		form.featureImageUrl = media.url;
		featurePickerOpen = false;
	}}
/>
