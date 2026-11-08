<script lang="ts">
	/**
	 * GuideSlideOver — right-side "How it works" drawer for directory pages.
	 *
	 * Body is a single markdown document rendered to HTML; a CTA button
	 * repeater sits at the bottom. When `canManage` and `onSave` are set,
	 * staff get a full-height inline editor covering ALL drawer text —
	 * title, body markdown, and the CTA list — persisted by the parent via
	 * `onSave` (typically PUT /admin/settings/{category} → public JSON blob).
	 */
	import { Dialog } from 'bits-ui';
	import { X, Pencil, BookOpen, Check, Loader2, ChevronRight, Plus, Trash2 } from 'lucide-svelte';
	import { t } from '#lib/i18n/index.js';
	import { markdownToHtml } from '#lib/studio/markdown-html.js';

	export interface GuideCta {
		title: string;
		url: string;
	}

	export interface GuideContent {
		title: string;
		/** Markdown source — rendered via markdownToHtml (sanitized). */
		body: string;
		ctas: GuideCta[];
	}

	interface Props {
		open?: boolean;
		/** Drawer heading (resolved value — settings override or i18n default). */
		title: string;
		/** Markdown body (resolved value — settings override or i18n default). */
		body?: string;
		/** Resolved CTA buttons — rendered stacked at the bottom. Absolute
		 *  http(s) URLs open in a new tab. */
		ctas?: GuideCta[];
		/** Shows the inline pencil editor (admin gate, resolved server-side). */
		canManage?: boolean;
		/** Persistence hook — receives the edited content, returns an error
		 *  message to display or null on success. */
		onSave?: (v: GuideContent) => Promise<string | null>;
		/** Max CTA buttons in the repeater (default 3). */
		maxCtas?: number;
	}

	let {
		open = $bindable(false),
		title,
		body = '',
		ctas = [],
		canManage = false,
		onSave,
		maxCtas = 3
	}: Props = $props();

	let editOpen = $state(false);
	let saving = $state(false);
	let error = $state('');
	let editTitle = $state('');
	let editBody = $state('');
	let editCtas = $state<GuideCta[]>([]);

	const bodyHtml = $derived(body ? markdownToHtml(body, { sanitize: true }) : '');
	const isExternal = (url: string) => /^https?:\/\//i.test(url);

	function openEdit() {
		editTitle = title;
		editBody = body;
		editCtas = ctas.map((c) => ({ ...c }));
		if (editCtas.length === 0) editCtas = [{ title: '', url: '' }];
		error = '';
		editOpen = true;
	}

	function addCta() {
		if (editCtas.length < maxCtas) editCtas = [...editCtas, { title: '', url: '' }];
	}

	function removeCta(i: number) {
		editCtas = editCtas.filter((_, idx) => idx !== i);
	}

	async function save() {
		if (!onSave) return;
		saving = true;
		error = '';
		const payload: GuideContent = {
			title: editTitle.trim(),
			body: editBody,
			ctas: editCtas
				.map((c) => ({ title: c.title.trim(), url: c.url.trim() }))
				.filter((c) => c.title && c.url)
		};
		const err = await onSave(payload);
		saving = false;
		if (err) {
			error = err;
			return;
		}
		editOpen = false;
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay
			class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=open]:animate-in data-[state=closed]:animate-out"
		/>
		<Dialog.Content
			class="fixed inset-y-0 end-0 z-50 flex w-full max-w-md flex-col border-s border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl transition-transform duration-300 ease-out data-[state=open]:translate-x-0 data-[state=closed]:translate-x-full rtl:data-[state=closed]:-translate-x-full"
		>
			{#if editOpen && canManage && onSave}
				<!-- Full-height edit mode — every text in the drawer is editable:
				     title, markdown body, and the CTA repeater. -->
				<div class="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-4">
					<Dialog.Title class="flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
						<Pencil size={16} class="text-[var(--color-primary-text)]" />
						{t('guide_panel.edit_title', 'Edit panel')}
					</Dialog.Title>
					<Dialog.Close
						class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						aria-label={t('common.close', 'Close')}
					>
						<X size={16} />
					</Dialog.Close>
				</div>

				<div class="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-5 py-4">
					<label class="text-xs font-medium text-[var(--color-muted)]" for="guide-title">
						{t('guide_panel.title_label', 'Title')}
					</label>
					<input
						id="guide-title"
						type="text"
						bind:value={editTitle}
						class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
					/>

					<label class="text-xs font-medium text-[var(--color-muted)]" for="guide-body">
						{t('guide_panel.body_label', 'Content (Markdown)')}
					</label>
					<textarea
						id="guide-body"
						bind:value={editBody}
						spellcheck="false"
						class="h-64 shrink-0 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 font-mono text-xs leading-relaxed text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
					></textarea>

					<p class="mt-1 text-xs font-medium text-[var(--color-muted)]">
						{t('guide_panel.ctas_label', 'Buttons')}
					</p>
					{#each editCtas as cta, i (i)}
						<div class="flex items-start gap-2">
							<div class="min-w-0 flex-1">
								<input
									type="text"
									bind:value={cta.title}
									placeholder={t('guide_panel.cta_title_label', 'CTA title')}
									aria-label={t('guide_panel.cta_title_label', 'CTA title')}
									class="mb-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								/>
								<input
									type="text"
									bind:value={cta.url}
									placeholder={t('guide_panel.cta_url_label', 'CTA link')}
									aria-label={t('guide_panel.cta_url_label', 'CTA link')}
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
								/>
							</div>
							{#if editCtas.length > 1}
								<button
									type="button"
									onclick={() => removeCta(i)}
									aria-label={t('common.remove', 'Remove')}
									class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] text-[var(--color-muted)] hover:text-[var(--color-danger)]"
								>
									<Trash2 size={14} />
								</button>
							{/if}
						</div>
					{/each}
					{#if editCtas.length < maxCtas}
						<button
							type="button"
							onclick={addCta}
							class="inline-flex items-center gap-1.5 self-start rounded-[var(--radius)] border border-dashed border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-muted)] hover:text-[var(--color-text)]"
						>
							<Plus size={13} /> {t('guide_panel.cta_add', 'Add button')}
						</button>
					{/if}

					{#if error}
						<p class="text-xs text-[var(--color-danger)]">{error}</p>
					{/if}
					<div class="mt-1 flex justify-end gap-2 pb-2">
						<button
							type="button"
							onclick={() => (editOpen = false)}
							class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.cancel', 'Cancel')}
						</button>
						<button
							type="button"
							onclick={save}
							disabled={saving}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] disabled:opacity-50"
						>
							{#if saving}<Loader2 size={14} class="animate-spin" />{:else}<Check size={14} />{/if}
							{t('common.save', 'Save')}
						</button>
					</div>
				</div>
			{:else}
				<div class="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-4">
					<Dialog.Title class="flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
						<BookOpen size={18} class="text-[var(--color-primary-text)]" />
						{title}
					</Dialog.Title>
					<Dialog.Close
						class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						aria-label={t('common.close', 'Close')}
					>
						<X size={16} />
					</Dialog.Close>
				</div>

				<div class="guide-body flex-1 overflow-y-auto px-5 py-5 text-sm leading-relaxed text-[var(--color-text-subtle)]">
					{#if bodyHtml}
						<!-- sanitized via DOMPurify in markdownToHtml -->
						<!-- eslint-disable-next-line svelte/no-at-html-tags -->
						{@html bodyHtml}
					{/if}
				</div>

				<!-- Bottom CTA repeater — staff with canManage get the pencil editor. -->
				<div class="border-t border-[var(--color-border)] px-5 py-4">
					<div class="flex flex-col gap-2">
						{#each ctas as cta (cta.url + cta.title)}
							<a
								href={cta.url}
								target={isExternal(cta.url) ? '_blank' : undefined}
								rel={isExternal(cta.url) ? 'noopener noreferrer' : undefined}
								class="inline-flex w-full items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
							>
								{cta.title}
								<ChevronRight size={15} class="rtl:-scale-x-100" />
							</a>
						{/each}
						{#if canManage && onSave}
							<button
								type="button"
								onclick={openEdit}
								title={t('guide_panel.cta_edit', 'Edit panel')}
								aria-label={t('guide_panel.cta_edit', 'Edit panel')}
								class="grid h-9 w-9 self-end place-items-center rounded-[var(--radius)] border border-[var(--color-border)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
							>
								<Pencil size={14} />
							</button>
						{/if}
					</div>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	/* Rendered markdown inherits theme tokens inside the drawer. */
	.guide-body :global(p) {
		margin-block: 0.5rem;
	}
	.guide-body :global(a) {
		color: var(--color-primary-text);
	}
	.guide-body :global(ol),
	.guide-body :global(ul) {
		padding-inline-start: 1.25rem;
		margin-block: 0.5rem;
	}
	.guide-body :global(ol) {
		list-style: decimal;
	}
	.guide-body :global(ul) {
		list-style: disc;
	}
	.guide-body :global(li) {
		margin-block: 0.25rem;
	}
	.guide-body :global(h1),
	.guide-body :global(h2),
	.guide-body :global(h3),
	.guide-body :global(h4) {
		color: var(--color-text);
		font-weight: 600;
		margin-block: 0.75rem 0.25rem;
	}
	.guide-body :global(strong) {
		color: var(--color-text);
	}
	.guide-body :global(code) {
		background: var(--color-surface-subtle);
		border-radius: var(--radius);
		padding: 0.1rem 0.3rem;
		font-size: 0.8em;
	}
	.guide-body :global(blockquote) {
		border-inline-start: 3px solid var(--color-border);
		padding-inline-start: 0.75rem;
		color: var(--color-muted);
	}
</style>
