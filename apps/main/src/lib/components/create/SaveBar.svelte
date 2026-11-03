<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { statusLabel } from './gate';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { ComposerStatus, ComposerOrganization } from '$lib/types';

	interface Props {
		status: ComposerStatus;
		savedAt: number | null;
		canPublish: boolean;
		publishReason: string | null;
		/** Orgs the current user is a member of. Empty = no post-as-org available. */
		orgs: ComposerOrganization[];
		/** Selected org id, or null for "post as yourself" (personal). */
		selectedOrgId: string | null;
		/** Callback when the author/org selection changes. */
		onSelectOrg: (id: string | null) => void;
		/** Bot personas the caller may attribute content to
		 *  (content.assign_author). Empty = no persona option in the
		 *  post-as selector. */
		botAuthors?: { id: string; label: string; sublabel?: string }[];
		/** Selected bot author id, or null for "no override". */
		selectedAuthorId?: string | null;
		/** Label for the null option — "Post as yourself" on create,
		 *  "Keep current author" when editing existing content. */
		authorNoneLabel?: string;
		/** Callback when the bot-author selection changes. */
		onSelectAuthor?: (id: string | null) => void;
		/** True while the post-as selector is in "Post as a persona" mode —
		 *  the persona picker only renders in that mode. */
		personaMode?: boolean;
		/** Called when the post-as selector enters/leaves persona mode. */
		onPersonaMode?: (v: boolean) => void;
		/** When true, the persona picker's "no override" option exits
		 *  persona mode entirely (create: "Post as yourself" means leave
		 *  persona mode — staying in it would leave publishing gated).
		 *  When false (edit), the none option stays: it means "keep the
		 *  stored author". */
		noneExitsPersona?: boolean;
		onSaveDraft: () => void;
		onPublish: () => void;
		/** Server error message from the last failed save — shown next to the
		 *  error status so 400s (e.g. publish_at_in_past, missing_required_fields)
		 *  tell the user what to fix (BUG-CE-019). */
		errorMessage?: string | null;
		/** True while a publish is in flight — disables the publish button
		 *  so a double-click can't fire a second publish (BUG-CE-007). */
		publishing?: boolean;
		/** When true, the content is already published — the primary action
		 *  becomes "Save" (flush/PATCH) instead of "Publish" (POST .../publish).
		 *  Re-publishing already-published content is semantically wrong UX
		 *  even though the backend endpoint is idempotent. */
		isPublished?: boolean;
	}

	let { status, savedAt, canPublish, publishReason, orgs, selectedOrgId, onSelectOrg, botAuthors = [], selectedAuthorId = null, authorNoneLabel, onSelectAuthor, personaMode = false, onPersonaMode, noneExitsPersona = false, onSaveDraft, onPublish, errorMessage = null, publishing = false, isPublished = false }: Props = $props();

	const selectedOrg = $derived(orgs.find((o) => o.id === selectedOrgId) ?? null);
	const selectedBot = $derived(botAuthors.find((b) => b.id === selectedAuthorId) ?? null);
	const noneLabel = $derived(authorNoneLabel ?? t('create.post_as_yourself', 'Post as yourself'));

	function formatSavedAt(ts: number | null): string {
		if (!ts) return '';
		const d = new Date(ts);
		return d.toLocaleTimeString(i18n.locale, { hour: '2-digit', minute: '2-digit' });
	}

	// "Post as yourself" and "Post as a persona" are special items with
	// sentinel ids, included in the Autocomplete item list so the user can
	// search for them. Persona is a mode, not a value — selecting it swaps
	// the org list for the persona picker instead of resolving to an id.
	const SELF_ID = '__self__';
	const PERSONA_ID = '__persona__';

	const personaLabel = $derived(t('create.post_as_persona', 'Post as a persona'));

	// Autocomplete fetchers — filter the already-loaded orgs client-side.
	// The persona option is appended only when the caller can assign a bot
	// author (botAuthors non-empty).
	function fetchOrgs(q: string): Promise<AutocompleteItem[]> {
		const selfLabel = t('create.post_as_yourself', 'Post as yourself');
		const query = q.toLowerCase();
		const selfItem: AutocompleteItem = { id: SELF_ID, label: selfLabel };
		const personaItem: AutocompleteItem | null =
			botAuthors.length > 0 ? { id: PERSONA_ID, label: personaLabel } : null;
		if (query === '') {
			return Promise.resolve([
				selfItem,
				...orgs.map((o) => ({ id: o.id, label: o.name, sublabel: o.member_role })),
				...(personaItem ? [personaItem] : [])
			]);
		}
		const matchesSelf = selfLabel.toLowerCase().includes(query);
		const matchesPersona = personaLabel.toLowerCase().includes(query);
		const filtered = orgs.filter((o) => o.name.toLowerCase().includes(query));
		const items = filtered.map((o) => ({ id: o.id, label: o.name, sublabel: o.member_role }));
		return Promise.resolve([
			...(matchesSelf ? [selfItem] : []),
			...items,
			...(matchesPersona && personaItem ? [personaItem] : [])
		]);
	}
	function fetchOrgsInitial(): Promise<AutocompleteItem[]> {
		return fetchOrgs('');
	}

	const selectedLabel = $derived(
		personaMode
			? personaLabel
			: selectedOrg
				? selectedOrg.name
				: t('create.post_as_yourself', 'Post as yourself')
	);
	function handleSelect(item: AutocompleteItem) {
		if (item.id === PERSONA_ID) {
			// Persona mode replaces org attribution — the unified post-as
			// selector shows one attribution at a time.
			onPersonaMode?.(true);
			onSelectOrg(null);
			return;
		}
		onPersonaMode?.(false);
		onSelectAuthor?.(null);
		onSelectOrg(item.id === SELF_ID ? null : item.id);
	}
	function handleClearPostAs() {
		onPersonaMode?.(false);
		onSelectAuthor?.(null);
		onSelectOrg(null);
	}

	// "No override" is a special item with a sentinel id, included so the
	// caller can revert to the default author (self on create, stored on edit).
	const NONE_ID = '__no_override__';
	function fetchBots(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const noneItem: AutocompleteItem = { id: NONE_ID, label: noneLabel };
		if (query === '') {
			return Promise.resolve([
				noneItem,
				...botAuthors.map((b) => ({ id: b.id, label: b.label, sublabel: b.sublabel }))
			]);
		}
		const items = botAuthors
			.filter(
				(b) =>
					b.label.toLowerCase().includes(query) ||
					(b.sublabel ?? '').toLowerCase().includes(query)
			)
			.map((b) => ({ id: b.id, label: b.label, sublabel: b.sublabel }));
		return Promise.resolve(noneLabel.toLowerCase().includes(query) ? [noneItem, ...items] : items);
	}
	function fetchBotsInitial(): Promise<AutocompleteItem[]> {
		return fetchBots('');
	}
	const selectedBotLabel = $derived(selectedBot ? selectedBot.label : noneLabel);
	function handleSelectAuthor(item: AutocompleteItem) {
		if (item.id === NONE_ID && noneExitsPersona) {
			onPersonaMode?.(false);
			onSelectAuthor?.(null);
			return;
		}
		onSelectAuthor?.(item.id === NONE_ID ? null : item.id);
	}
	function handleClearAuthor() {
		if (noneExitsPersona) onPersonaMode?.(false);
		onSelectAuthor?.(null);
	}
</script>

<div class="save-bar">
	<!-- Left: attribution selectors (replaces old "Draft" status text).
	     Ghost-style triggers — no border, subtle hover, blend into the bar. -->
	<div class="bar-left">
	{#if orgs.length > 0 || botAuthors.length > 0}
		<div class="author-autocomplete">
			<Autocomplete
				placeholder={t('create.post_as_yourself', 'Post as yourself')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchOrgs}
				initialFetcher={fetchOrgsInitial}
				selectedId={personaMode ? PERSONA_ID : (selectedOrg ? selectedOrgId : SELF_ID)}
				selectedLabel={selectedLabel}
				onSelect={handleSelect}
				onClear={handleClearPostAs}
				openUp
			/>
		</div>
	{/if}
	{#if personaMode && botAuthors.length > 0}
		<div class="author-autocomplete">
			<Autocomplete
				placeholder={t('create.author_label', 'Author')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchBots}
				initialFetcher={fetchBotsInitial}
				selectedId={selectedBot ? selectedAuthorId : NONE_ID}
				selectedLabel={selectedBotLabel}
				onSelect={handleSelectAuthor}
				onClear={handleClearAuthor}
				openUp
			/>
		</div>
	{/if}
	{#if orgs.length === 0 && botAuthors.length === 0}
		<span class="status-text">
			{t(statusLabel(status))}
			{#if status === 'saved' && savedAt}
				<span class="saved-time"> · {formatSavedAt(savedAt)}</span>
			{/if}
		</span>
	{/if}
	</div>

	<!-- Right: publish gate reason + action buttons -->
	<div class="actions">
		{#if !canPublish && publishReason && !isPublished}
			<span class="gate-reason">{t(publishReason)}</span>
		{/if}
		{#if status === 'error' && errorMessage}
			<span class="error-message">{errorMessage}</span>
		{/if}
		{#if isPublished}
			<!-- Already-published content: the primary action is "Save" (flush/PATCH).
			     The "Save draft" secondary button is hidden because the content is
			     not a draft — showing both "Save draft" and "Save" is confusing.
			     The button is disabled when the composer status is 'saved' (nothing
			     to flush) or 'saving' (flush in flight), and re-enables when the
			     user makes an edit (status transitions to 'dirty'). -->
			<button
				type="button"
				class="btn-primary"
				disabled={status === 'saved' || status === 'saving' || status === 'pristine'}
				onclick={onSaveDraft}
			>
				{#if status === 'saving'}
					{t('create.status.saving', 'Saving…')}
				{:else if status === 'saved'}
					{t('create.status.saved', 'Saved')}
				{:else}
					{t('create.action.save')}
				{/if}
			</button>
		{:else}
			<button type="button" class="btn-secondary" onclick={onSaveDraft}>
				{t('create.action.save_draft')}
			</button>
			<button
				type="button"
				class="btn-primary"
				disabled={!canPublish || publishing}
				onclick={onPublish}
			>
				{publishing ? t('create.status.saving', 'Saving…') : t('create.action.publish')}
			</button>
		{/if}
	</div>
</div>

<style>
	.save-bar {
		position: sticky;
		bottom: 0;
		z-index: 30;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		border-top: 1px solid var(--color-border);
		background: var(--color-surface);
		padding: 10px 16px;
	}

	/* ---- Attribution selectors (post-as-org + author override) ---- */
	.bar-left {
		display: flex;
		align-items: center;
		gap: 8px;
		min-width: 0;
	}
	.author-autocomplete {
		min-width: 200px;
		max-width: 260px;
	}

	/* ---- Status text fallback (no orgs) ---- */
	.status-text {
		font-size: 0.8125rem;
		color: var(--color-muted);
	}
	.saved-time {
		color: var(--color-muted);
		font-size: 0.75rem;
	}

	/* ---- Right side actions ---- */
	.actions {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
	}
	.gate-reason {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin-inline-end: 4px;
	}
	.error-message {
		font-size: 0.75rem;
		color: var(--color-danger, #dc2626);
		margin-inline-end: 4px;
	}
	.btn-secondary {
		padding: 6px 14px;
		border: none;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-text);
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.12s;
	}
	.btn-secondary:hover {
		background: var(--color-surface-subtle);
	}
	.btn-primary {
		padding: 6px 18px;
		border: none;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: white;
		font-size: 0.8125rem;
		font-weight: 600;
		cursor: pointer;
		transition: background 0.12s, opacity 0.12s;
	}
	.btn-primary:hover:not(:disabled) {
		background: var(--color-primary-hover);
	}
	.btn-primary:disabled {
		opacity: 0.45;
		cursor: not-allowed;
	}

	/* Mobile: the author selector + action buttons do not fit side by side
	 * on narrow viewports and overflow the page horizontally. Stack them so
	 * both take the full width and the primary actions remain easy to tap. */
	@media (max-width: 767px) {
		.save-bar {
			flex-direction: column;
			align-items: stretch;
			gap: 8px;
			padding: 10px 12px;
		}
		.bar-left {
			flex-direction: column;
			align-items: stretch;
			width: 100%;
		}
		.author-autocomplete,
		.status-text {
			min-width: 0;
			max-width: none;
			width: 100%;
		}
		.actions {
			width: 100%;
			justify-content: flex-start;
			gap: 8px;
		}
		.gate-reason {
			width: 100%;
			margin: 0 0 4px 0;
			font-size: 0.75rem;
			text-align: start;
		}
		.btn-secondary,
		.btn-primary {
			flex: 1 1 0;
			min-width: 0;
			min-height: 44px;
			padding: 10px 12px;
			font-size: 0.875rem;
			overflow-wrap: break-word;
		}
	}
</style>
