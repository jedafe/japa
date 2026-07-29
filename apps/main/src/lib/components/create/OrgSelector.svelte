<script lang="ts">
	import { t } from '$lib/i18n';
	import { Select } from 'bits-ui';
	import { Building2, User, Check } from 'lucide-svelte';
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
	<Select.Root
		type="single"
		value={currentValue}
		onValueChange={(v) => {
			onfocus?.();
			onSelect(v === '' ? null : (v as string));
		}}
	>
		<Select.Trigger
			class="flex w-full items-center justify-between gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 py-2 text-left text-sm text-[var(--color-text)] transition-colors hover:border-[var(--color-text-subtle)] focus:border-[var(--color-primary)] focus:outline-none"
		>
			<span class="flex items-center gap-1.5 overflow-hidden text-ellipsis whitespace-nowrap">
				{#if selectedOrg}
					<Building2 size={15} class="shrink-0 text-[var(--color-muted)]" />
					{selectedOrg.name}
				{:else}
					<User size={15} class="shrink-0 text-[var(--color-muted)]" />
					{t('create.post_as_yourself', 'Post as yourself')}
				{/if}
			</span>
		</Select.Trigger>
		<Select.Portal>
			<Select.Content
				class="z-40 max-h-60 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
			>
				{#each options as opt (opt.value)}
					<Select.Item
						value={opt.value}
						class="flex cursor-pointer items-center justify-between gap-2 rounded-[calc(var(--radius)-2px)] px-2.5 py-2 text-left text-sm text-[var(--color-text)] outline-none data-[highlighted]:bg-[var(--color-surface-subtle)] data-[selected]:bg-[color-mix(in_srgb,var(--color-primary)_10%,transparent)] data-[selected]:text-[var(--color-primary)] data-[selected]:font-medium"
					>
						{#snippet child({ props })}
							<div {...props}>
								<span class="flex items-center gap-1.5 overflow-hidden text-ellipsis whitespace-nowrap">
									{#if opt.icon === 'org'}
										<Building2 size={14} class="shrink-0 text-[var(--color-muted)]" />
									{:else}
										<User size={14} class="shrink-0 text-[var(--color-muted)]" />
									{/if}
									{opt.label}
									{#if opt.role}
										<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] font-medium capitalize text-[var(--color-text-subtle)]">{opt.role}</span>
									{/if}
								</span>
								{#if opt.value === currentValue}
									<Check size={14} class="shrink-0 text-[var(--color-primary)]" />
								{/if}
							</div>
						{/snippet}
					</Select.Item>
				{/each}
			</Select.Content>
		</Select.Portal>
	</Select.Root>
{/if}
