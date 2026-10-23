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
		onSaveDraft: () => void;
		onPublish: () => void;
		/** When true, the content is already published — the primary action
		 *  becomes "Save" (flush/PATCH) instead of "Publish" (POST .../publish).
		 *  Re-publishing already-published content is semantically wrong UX
		 *  even though the backend endpoint is idempotent. */
		isPublished?: boolean;
	}

	let { status, savedAt, canPublish, publishReason, orgs, selectedOrgId, onSelectOrg, onSaveDraft, onPublish, isPublished = false }: Props = $props();

	const selectedOrg = $derived(orgs.find((o) => o.id === selectedOrgId) ?? null);

	function formatSavedAt(ts: number | null): string {
		if (!ts) return '';
		const d = new Date(ts);
		return d.toLocaleTimeString(i18n.locale, { hour: '2-digit', minute: '2-digit' });
	}

	// "Post as yourself" is a special item with a sentinel id. It is
	// included in the Autocomplete item list so the user can search for it.
	const SELF_ID = '__self__';

	// Autocomplete fetchers — filter the already-loaded orgs client-side.
	function fetchOrgs(q: string): Promise<AutocompleteItem[]> {
		const selfLabel = t('create.post_as_yourself', 'Post as yourself');
		const query = q.toLowerCase();
		const selfItem: AutocompleteItem = { id: SELF_ID, label: selfLabel };
		if (query === '') {
			return Promise.resolve([
				selfItem,
				...orgs.map((o) => ({ id: o.id, label: o.name, sublabel: o.member_role }))
			]);
		}
		const matchesSelf = selfLabel.toLowerCase().includes(query);
		const filtered = orgs.filter((o) => o.name.toLowerCase().includes(query));
		const items = filtered.map((o) => ({ id: o.id, label: o.name, sublabel: o.member_role }));
		return Promise.resolve(matchesSelf ? [selfItem, ...items] : items);
	}
	function fetchOrgsInitial(): Promise<AutocompleteItem[]> {
		return fetchOrgs('');
	}

	const selectedLabel = $derived(
		selectedOrg
			? selectedOrg.name
			: t('create.post_as_yourself', 'Post as yourself')
	);
	function handleSelect(item: AutocompleteItem) {
		onSelectOrg(item.id === SELF_ID ? null : item.id);
	}
</script>

<div class="save-bar">
	<!-- Left: author selector (replaces old "Draft" status text).
	     Ghost-style trigger — no border, subtle hover, blends into the bar. -->
	{#if orgs.length > 0}
		<div class="author-autocomplete">
			<Autocomplete
				placeholder={t('create.post_as_yourself', 'Post as yourself')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchOrgs}
				initialFetcher={fetchOrgsInitial}
				selectedId={selectedOrg ? selectedOrgId : SELF_ID}
				selectedLabel={selectedLabel}
				onSelect={handleSelect}
				onClear={() => onSelectOrg(null)}
				openUp
			/>
		</div>
	{:else}
		<span class="status-text">
			{t(statusLabel(status))}
			{#if status === 'saved' && savedAt}
				<span class="saved-time"> · {formatSavedAt(savedAt)}</span>
			{/if}
		</span>
	{/if}

	<!-- Right: publish gate reason + action buttons -->
	<div class="actions">
		{#if !canPublish && publishReason && !isPublished}
			<span class="gate-reason">{t(publishReason)}</span>
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
				disabled={!canPublish}
				onclick={onPublish}
			>
				{t('create.action.publish')}
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

	/* ---- Author autocomplete wrapper ---- */
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
