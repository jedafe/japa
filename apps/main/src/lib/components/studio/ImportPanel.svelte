<script lang="ts">
	// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §5 — paste generated
	// .md output → parse frontmatter → confirm → save as a contents draft.
	// This is the web replacement for the desktop app's watched folder.
	import { t } from '$lib/i18n';
	import { enhance } from '$app/forms';
	import { invalidate } from '$app/navigation';
	import type { ContentTypeMeta, MeSpace, ContentRecord } from '$lib/types';
	import { parseFrontmatter, buildMarkdown } from '$lib/studio/frontmatter';
	import { validateFrontmatter, validateBody, isHighStakesCategory } from '$lib/studio/validation';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import { FileUp, CheckCircle2, AlertCircle, Copy, Check, Save } from 'lucide-svelte';

	const STUDIO_PROMPTS_TAG = 'app:studio-prompts';

	interface Props {
		contentTypes: ContentTypeMeta[];
		spaces: MeSpace[];
		drafts: ContentRecord[];
	}

	let { contentTypes, spaces, drafts }: Props = $props();

	let raw = $state('');
	let parsed = $state(false);
	let fm = $state<Record<string, unknown>>({});
	let body = $state('');
	let errors = $state<Record<string, string>>({});
	let bodyError = $state('');
	let submitting = $state(false);
	let copiedMd = $state(false);
	let result = $state<{ ok?: boolean; error?: string; message?: string; id?: string } | null>(null);

	const categories = $derived(
		contentTypes.find((ct) => ct.value === str('content_type'))?.categories ?? []
	);
	const highStakes = $derived(isHighStakesCategory(str('guide_category')));
	const conflictingDraft = $derived(drafts.find((d) => d.slug && d.slug === str('slug')));

	const typeItems = $derived(contentTypes.map((ct) => ({ id: ct.value, label: ct.label })));
	const categoryItems = $derived(categories.map((c) => ({ id: c.Slug, label: c.Label })));
	const spaceItems = $derived(spaces.map((s) => ({ id: s.id, label: s.name })));
	const statusItems = $derived(
		['draft', 'in_review', 'published', 'scheduled']
			.filter((s) => !highStakes || (s !== 'published' && s !== 'scheduled'))
			.map((s) => ({ id: s, label: s }))
	);
	const visibilityItems = ['public', 'members', 'private'].map((v) => ({ id: v, label: v }));

	function labelFor(items: { id: string; label: string }[], id: string): string {
		return items.find((i) => i.id === id)?.label ?? '';
	}

	function str(key: string): string {
		const v = fm[key];
		return typeof v === 'string' ? v : v == null ? '' : String(v);
	}

	function setStr(key: string, value: string) {
		fm = { ...fm, [key]: value };
	}

	function tagsText(): string {
		const v = fm.tags;
		if (Array.isArray(v)) return v.join(', ');
		return typeof v === 'string' ? v : '';
	}

	function doParse() {
		const p = parseFrontmatter(raw);
		fm = p.frontmatter;
		body = p.body;
		parsed = true;
		result = null;
		revalidate();
	}

	function revalidate() {
		const status = str('status') || 'draft';
		errors = validateFrontmatter(fm, status, 'member');
		bodyError = validateBody(body, status) ?? '';
	}

	function onFile(e: Event) {
		const file = (e.currentTarget as HTMLInputElement).files?.[0];
		if (!file) return;
		const reader = new FileReader();
		reader.onload = () => {
			raw = String(reader.result ?? '');
			doParse();
		};
		reader.readAsText(file);
	}

	function payloadJson(): string {
		const payload: Record<string, unknown> = { ...fm, body };
		if (conflictingDraft) payload.content_id = conflictingDraft.id;
		if (highStakes && (str('status') === 'published' || str('status') === 'scheduled')) {
			payload.status = 'in_review';
		}
		return JSON.stringify(payload);
	}

	async function copyAsMarkdown() {
		try {
			await navigator.clipboard.writeText(buildMarkdown(fm, body));
			copiedMd = true;
			setTimeout(() => (copiedMd = false), 2000);
		} catch {
			// Clipboard unavailable — non-fatal.
		}
	}

	const inputCls =
		'mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]';
	const labelCls = 'block text-xs font-medium text-[var(--color-muted)]';
	const errCls = 'mt-1 text-xs text-[var(--color-danger)]';
	const emptyMsg = t('common.no_results', 'No results found');
</script>

{#snippet acField(
	label: string,
	items: { id: string; label: string }[],
	key: string,
	onPick?: (id: string) => void
)}
	<div>
		<span class={labelCls}>{label}</span>
		<div class="mt-1">
			<Autocomplete
				placeholder={label}
				emptyMessage={emptyMsg}
				fetcher={staticAutocompleteFetcher(items)}
				initialFetcher={staticAutocompleteFetcher(items)}
				selectedId={str(key) || null}
				selectedLabel={labelFor(items, str(key))}
				onSelect={(item) => { setStr(key, item.id); onPick?.(item.id); revalidate(); }}
				onClear={() => { setStr(key, ''); onPick?.(''); revalidate(); }}
			/>
		</div>
		{#if errors[key]}<p class={errCls}>{errors[key]}</p>{/if}
	</div>
{/snippet}

<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
		<FileUp size={16} /> {t('studio.prompts.import_title', 'Import generated output')}
	</h2>

	{#if result?.ok}
		<div class="mb-3 flex items-center gap-2 text-sm text-[var(--color-success)]">
			<CheckCircle2 size={16} />
			{t('studio.prompts.saved', 'Draft saved.')}
			{#if result.id}
				<a href="/studio/create?draft={result.id}" class="underline">{t('studio.prompts.open_editor', 'Open in editor')}</a>
			{/if}
		</div>
	{:else if result?.error}
		<div class="mb-3 flex items-center gap-2 text-sm text-[var(--color-danger)]">
			<AlertCircle size={16} /> {result.message || result.error}
		</div>
	{/if}

	{#if !parsed}
		<p class="mb-2 text-xs text-[var(--color-muted)]">
			{t('studio.prompts.import_help', 'Paste the markdown file your assistant produced (including its YAML frontmatter), or drop a .md file.')}
		</p>
		<textarea
			bind:value={raw}
			rows="8"
			placeholder={'---\ntitle: "..."\nslug: "..."\n---\n\n# ...'}
			class="{inputCls} font-mono text-xs"
		></textarea>
		<div class="mt-3 flex flex-wrap items-center gap-2">
			<button
				type="button"
				onclick={doParse}
				disabled={!raw.trim()}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
			>
				{t('studio.prompts.parse', 'Parse output')}
			</button>
			<label class="cursor-pointer rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-elevated)]">
				{t('studio.prompts.choose_file', 'Choose .md file')}
				<input type="file" accept=".md,.markdown,.txt" class="hidden" onchange={onFile} />
			</label>
		</div>
	{:else}
		<form
			method="POST"
			action="?/save"
			use:enhance={() => {
				submitting = true;
				return async ({ result: r }) => {
					submitting = false;
					if (r.type === 'success') {
						const data = r.data as { ok?: boolean; error?: string; message?: string; content?: { id?: string } };
						result = { ...data, id: data.content?.id };
						if (data.ok) await invalidate(STUDIO_PROMPTS_TAG);
					}
				};
			}}
		>
			<input type="hidden" name="payload" value={payloadJson()} />

			{#if conflictingDraft}
				<div class="mb-3 flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-elevated)] px-3 py-2 text-xs text-[var(--color-text)]">
					<AlertCircle size={14} />
					{t('studio.prompts.slug_conflict', 'A draft with this slug exists — saving will update it.')}
				</div>
			{/if}

			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
				<label class={labelCls}>
					{t('studio.prompts.title', 'Title')}
					<input type="text" value={str('title')} oninput={(e) => { setStr('title', e.currentTarget.value); revalidate(); }} class={inputCls} />
					{#if errors.title}<p class={errCls}>{errors.title}</p>{/if}
				</label>
				<label class={labelCls}>
					{t('studio.prompts.slug', 'Slug')}
					<input type="text" value={str('slug')} oninput={(e) => { setStr('slug', e.currentTarget.value); revalidate(); }} class={inputCls} />
					{#if errors.slug}<p class={errCls}>{errors.slug}</p>{/if}
				</label>
				{@render acField(t('studio.prompts.content_type', 'Content type'), typeItems, 'content_type', () => setStr('guide_category', ''))}
				{#if categories.length > 0}
					{@render acField(t('studio.prompts.guide_category', 'Guide category'), categoryItems, 'guide_category')}
				{/if}
				{@render acField(t('studio.prompts.space', 'Space'), spaceItems, 'space_id')}
				{@render acField(t('studio.prompts.status', 'Status'), statusItems, 'status')}
				{@render acField(t('studio.prompts.visibility', 'Visibility'), visibilityItems, 'visibility')}
				<label class={labelCls}>
					{t('studio.prompts.tags', 'Tags (comma-separated)')}
					<input type="text" value={tagsText()} oninput={(e) => { fm = { ...fm, tags: e.currentTarget.value.split(',').map((x) => x.trim()).filter(Boolean) }; }} class={inputCls} />
				</label>
			</div>

			{#if highStakes}
				<p class="mt-3 text-xs text-[var(--color-warning)]">
					{t('studio.prompts.high_stakes', 'High-stakes category — saved as in_review regardless of selected status.')}
				</p>
			{/if}
			{#if bodyError}
				<p class="mt-3 text-xs text-[var(--color-danger)]">{bodyError}</p>
			{/if}

			<details class="mt-3">
				<summary class="cursor-pointer text-xs font-medium text-[var(--color-muted)]">
					{t('studio.prompts.body_preview', 'Body preview')}
				</summary>
				<pre class="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-xs text-[var(--color-text)]">{body}</pre>
			</details>

			<div class="mt-4 flex flex-wrap items-center gap-2">
				<button
					type="submit"
					disabled={submitting || Object.keys(errors).length > 0 || !!bodyError}
					class="flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
				>
					<Save size={14} />
					{submitting
						? t('common.submitting', 'Submitting…')
						: conflictingDraft
							? t('studio.prompts.update_draft', 'Update draft')
							: t('studio.prompts.save_draft', 'Save draft')}
				</button>
				<button
					type="button"
					onclick={copyAsMarkdown}
					class="flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-elevated)]"
				>
					{#if copiedMd}<Check size={14} />{:else}<Copy size={14} />{/if}
					{t('studio.prompts.copy_md', 'Copy as .md')}
				</button>
				<button
					type="button"
					onclick={() => { parsed = false; result = null; }}
					class="rounded-[var(--radius)] px-3 py-2 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
				>
					{t('studio.prompts.reset', 'Start over')}
				</button>
			</div>
		</form>
	{/if}
</div>
