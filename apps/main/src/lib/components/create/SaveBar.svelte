<script lang="ts">
	import { t } from '$lib/i18n';
	import { statusLabel } from './gate';
	import { Select } from 'bits-ui';
	import { Building2, User, Check, ChevronDown } from 'lucide-svelte';
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

	let { status, canPublish, publishReason, orgs, selectedOrgId, onSelectOrg, onSaveDraft, onPublish, isPublished = false }: Props = $props();

	const selectedOrg = $derived(orgs.find((o) => o.id === selectedOrgId) ?? null);
	const currentValue = $derived(selectedOrgId ?? '');

	type OrgOption = { value: string; label: string; icon: 'user' | 'org'; role?: string };
	const options = $derived<OrgOption[]>([
		{ value: '', label: t('create.post_as_yourself', 'Post as yourself'), icon: 'user' },
		...orgs.map((o) => ({ value: o.id, label: o.name, icon: 'org' as const, role: o.member_role }))
	]);
</script>

<div class="save-bar">
	<!-- Left: author selector (replaces old "Draft" status text).
	     Ghost-style trigger — no border, subtle hover, blends into the bar. -->
	{#if orgs.length > 0}
		<Select.Root
			type="single"
			value={currentValue}
			onValueChange={(v) => onSelectOrg(v === '' ? null : (v as string))}
		>
			<Select.Trigger class="author-trigger" aria-label={t('create.author_label', 'Author')}>
				{#if selectedOrg}
					<Building2 size={14} class="author-icon" />
				{:else}
					<User size={14} class="author-icon" />
				{/if}
				<span class="author-name">
					{selectedOrg ? selectedOrg.name : t('create.post_as_yourself', 'Post as yourself')}
				</span>
				<ChevronDown size={13} class="author-chevron" />
			</Select.Trigger>
			<Select.Portal>
				<Select.Content class="author-content">
					{#each options as opt (opt.value)}
						<Select.Item value={opt.value} class="author-item">
							{#snippet child({ props })}
								<div {...props}>
									<span class="author-item-label">
										{#if opt.icon === 'org'}
											<Building2 size={14} class="author-icon" />
										{:else}
											<User size={14} class="author-icon" />
										{/if}
										{opt.label}
										{#if opt.role}
											<span class="author-role">{opt.role}</span>
										{/if}
									</span>
									{#if opt.value === currentValue}
										<Check size={14} class="author-check" />
									{/if}
								</div>
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Content>
			</Select.Portal>
		</Select.Root>
	{:else}
		<span class="status-text">{t(statusLabel(status))}</span>
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

	/* ---- Author trigger: ghost/borderless, blends into the bar ---- */
	.author-trigger {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		padding: 5px 8px;
		border: none;
		border-radius: var(--radius);
		background: transparent;
		color: var(--color-text-subtle);
		font-size: 0.8125rem;
		font-weight: 500;
		cursor: pointer;
		transition: background 0.12s, color 0.12s;
	}
	.author-trigger:hover {
		background: var(--color-surface-subtle);
		color: var(--color-text);
	}
	.author-trigger:focus-visible {
		outline: 2px solid var(--color-primary);
		outline-offset: 1px;
	}
	.author-icon {
		flex-shrink: 0;
		color: var(--color-muted);
	}
	.author-name {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		max-width: 180px;
	}
	.author-chevron {
		flex-shrink: 0;
		color: var(--color-muted);
		opacity: 0.6;
	}

	/* ---- Dropdown content ---- */
	.author-content {
		z-index: 50;
		min-width: 200px;
		max-height: 260px;
		overflow-y: auto;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		padding: 4px;
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.14);
	}
	.author-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		padding: 7px 10px;
		border-radius: calc(var(--radius) - 2px);
		font-size: 0.8125rem;
		color: var(--color-text);
		cursor: pointer;
		outline: none;
	}
	.author-item[data-highlighted] {
		background: var(--color-surface-subtle);
	}
	.author-item[data-selected] {
		background: color-mix(in srgb, var(--color-primary) 8%, transparent);
		color: var(--color-primary);
		font-weight: 500;
	}
	.author-item-label {
		display: flex;
		align-items: center;
		gap: 6px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.author-role {
		flex-shrink: 0;
		border-radius: 9999px;
		background: var(--color-surface-subtle);
		padding: 1px 6px;
		font-size: 10px;
		font-weight: 500;
		text-transform: capitalize;
		color: var(--color-text-subtle);
	}
	.author-check {
		flex-shrink: 0;
		color: var(--color-primary);
	}

	/* ---- Status text fallback (no orgs) ---- */
	.status-text {
		font-size: 0.8125rem;
		color: var(--color-muted);
	}

	/* ---- Right side actions ---- */
	.actions {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.gate-reason {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin-right: 4px;
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
</style>
