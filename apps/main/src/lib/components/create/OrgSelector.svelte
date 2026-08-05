<script lang="ts">
	import { t } from '$lib/i18n';
	import { Select } from 'bits-ui';
	import { Building2, User, Check, ChevronDown } from 'lucide-svelte';
	import type { ComposerOrganization } from '$lib/types';

	interface Props {
		/** Orgs the current user is a member of. Empty = no post-as-org available. */
		orgs: ComposerOrganization[];
		/** Selected org id, or null for "post as yourself" (personal). */
		selectedId: string | null;
		onSelect: (id: string | null) => void;
		onfocus?: () => void;
	}

	let { orgs, selectedId, onSelect, onfocus }: Props = $props();

	const selectedOrg = $derived(orgs.find((o) => o.id === selectedId) ?? null);

	// Build the options list: "Post as yourself" (value: "") + orgs.
	type OrgOption = { value: string; label: string; icon: 'user' | 'org'; role?: string };
	const options = $derived<OrgOption[]>([
		{ value: '', label: t('create.post_as_yourself', 'Post as yourself'), icon: 'user' },
		...orgs.map((o) => ({ value: o.id, label: o.name, icon: 'org' as const, role: o.member_role }))
	]);

	const currentValue = $derived(selectedId ?? '');
</script>

{#if orgs.length > 0}
	<div class="org-selector">
		<span class="org-label" id="org-selector-label">{t('create.author_label', 'Author')}</span>
		<Select.Root
			type="single"
			value={currentValue}
			onValueChange={(v) => {
				onfocus?.();
				onSelect(v === '' ? null : (v as string));
			}}
		>
			<Select.Trigger
				class="org-trigger"
				aria-labelledby="org-selector-label"
			>
				<span class="org-trigger-content">
					{#if selectedOrg}
						<Building2 size={16} class="org-icon" />
						{selectedOrg.name}
					{:else}
						<User size={16} class="org-icon" />
						{t('create.post_as_yourself', 'Post as yourself')}
					{/if}
				</span>
				<ChevronDown size={16} class="org-chevron" />
			</Select.Trigger>
			<Select.Portal>
				<Select.Content class="org-content">
					{#each options as opt (opt.value)}
						<Select.Item
							value={opt.value}
							class="org-item"
						>
							{#snippet child({ props })}
								<div {...props}>
									<span class="org-item-label">
										{#if opt.icon === 'org'}
											<Building2 size={15} class="org-icon" />
										{:else}
											<User size={15} class="org-icon" />
										{/if}
										{opt.label}
										{#if opt.role}
											<span class="org-role">{opt.role}</span>
										{/if}
									</span>
									{#if opt.value === currentValue}
										<Check size={15} class="org-check" />
									{/if}
								</div>
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Content>
			</Select.Portal>
		</Select.Root>
	</div>
{/if}

<style>
	.org-selector {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.org-label {
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--color-muted);
	}
	.org-trigger {
		display: flex;
		width: 100%;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		padding: 10px 14px;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 1rem;
		text-align: left;
		cursor: pointer;
		transition: border-color 0.15s, box-shadow 0.15s;
	}
	.org-trigger:hover {
		border-color: var(--color-text-subtle);
	}
	.org-trigger:focus-visible {
		border-color: var(--color-primary);
		box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 15%, transparent);
		outline: none;
	}
	.org-trigger-content {
		display: flex;
		align-items: center;
		gap: 6px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.org-icon {
		flex-shrink: 0;
		color: var(--color-muted);
	}
	.org-chevron {
		flex-shrink: 0;
		color: var(--color-muted);
		transition: transform 0.15s;
	}

	.org-content {
		z-index: 40;
		max-height: 240px;
		overflow-y: auto;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		padding: 4px;
		box-shadow: 0 4px 16px rgb(0 0 0 / 0.12);
	}
	.org-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		padding: 8px 10px;
		border-radius: calc(var(--radius) - 2px);
		font-size: 0.875rem;
		color: var(--color-text);
		cursor: pointer;
		outline: none;
	}
	.org-item:hover {
		background: var(--color-surface-subtle);
	}
	.org-item[data-selected] {
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
		font-weight: 500;
	}
	.org-item-label {
		display: flex;
		align-items: center;
		gap: 6px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.org-role {
		flex-shrink: 0;
		border-radius: 9999px;
		background: var(--color-surface-subtle);
		padding: 1px 6px;
		font-size: 10px;
		font-weight: 500;
		text-transform: capitalize;
		color: var(--color-text-subtle);
	}
	.org-check {
		flex-shrink: 0;
		color: var(--color-primary);
	}
</style>
