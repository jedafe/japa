<script lang="ts">
	import { t } from '$lib/i18n';
	import type { ResolvedItem } from '$lib/types/jce';
	import { ArrowRight, Sparkles, X } from 'lucide-svelte';
	import type { Component } from 'svelte';

	interface Props {
		items: ResolvedItem[];
		maxItems: number;
		authed: boolean;
		creating: boolean;
		aiEnabled: boolean;
		aiActive: boolean;
		hint?: string;
		itemLabel: string;
		itemIcon?: Component;
		onRemove: (item: ResolvedItem) => void;
		onCompare: () => void;
		onLogin: () => void;
		onToggleAi: () => void;
	}

	let {
		items,
		maxItems,
		authed,
		creating,
		aiEnabled,
		aiActive,
		hint = '',
		itemLabel,
		itemIcon: ItemIcon,
		onRemove,
		onCompare,
		onLogin,
		onToggleAi
	}: Props = $props();

	let canCompare = $derived(items.length >= 2);
</script>

<div
	class="border-t px-4 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]"
	style="background-color: var(--color-bg); border-color: var(--color-border)"
	role="region"
	aria-label={t('jce.compare.heading')}
>
	{#if hint}
		<p class="mb-2 text-xs" style="color: var(--color-muted)">{hint}</p>
	{/if}
	<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
		<div class="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
			{#if items.length === 0}
				<span class="text-sm" style="color: var(--color-muted)">
					{t('jce.compare.tray.pick_hint', 'Pick {min}–{max} {label} to compare', {
						min: '2',
						max: String(maxItems),
						label: itemLabel
					})}
				</span>
			{:else}
				{#each items as item (item.id)}
					<span
						class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm"
						style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
					>
						{#if ItemIcon}<ItemIcon size={12} />{/if}
						<span class="max-w-40 truncate">{item.title}</span>
						<button
							type="button"
							onclick={() => onRemove(item)}
							class="rounded-full p-0.5"
							style="color: var(--color-muted)"
							aria-label="{t('jce.common.delete')} {item.title}"
						>
							<X size={13} />
						</button>
					</span>
				{/each}
			{/if}
		</div>

		<div class="flex shrink-0 items-center justify-end gap-2 mr-16 sm:mr-0">
			{#if items.length > 0}
				<span class="text-xs whitespace-nowrap" style="color: var(--color-muted)">
					{items.length}/{maxItems}
				</span>
			{/if}
			{#if authed}
				{#if aiEnabled}
					<button
						type="button"
						onclick={onToggleAi}
						aria-pressed={aiActive}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-2 text-sm font-medium"
						style="background-color: {aiActive ? 'var(--color-bg)' : 'var(--color-surface)'}; border-color: var(--color-border); color: var(--color-text)"
					>
						<Sparkles size={14} />
						<span class="hidden sm:inline">{t('jce.compare.ai_quick.button')}</span>
					</button>
				{/if}
				<button
					type="button"
					disabled={!canCompare || creating}
					onclick={onCompare}
					class="inline-flex items-center gap-2 rounded-[var(--radius-button)] px-5 py-2 text-sm font-medium disabled:opacity-50"
					style="background-color: var(--color-surface); border: 1px solid var(--color-border); color: var(--color-text)"
				>
					{creating ? t('jce.compare.creating') : t('jce.compare.compare_now')}
					<ArrowRight size={15} />
				</button>
			{:else}
				<button
					type="button"
					disabled={!canCompare}
					onclick={onLogin}
					class="inline-flex items-center gap-2 rounded-[var(--radius-button)] px-5 py-2 text-sm font-medium disabled:opacity-50"
					style="background-color: var(--color-surface); border: 1px solid var(--color-border); color: var(--color-text)"
				>
					{t('jce.compare.login_to_compare')}
				</button>
			{/if}
		</div>
	</div>
</div>
