<script lang="ts">
	// Ref: japaconnect-engineering/sections/114-services-marketplace.md
	// §114 — package pricing-tier editor (Essential/Standard/Premium style).
	// Replace-on-save like items[]/departures[]: position follows row order.
	import { t } from '$lib/i18n';
	import { Plus, Trash2, ChevronUp, ChevronDown, Star } from 'lucide-svelte';
	import type { CurrencyInfo } from '$lib/utils/currency';
	import type { EditableTier } from './serviceForm';
	import {
		marketplaceIcon,
		MARKETPLACE_ICON_NAMES
	} from '$lib/components/marketplace/marketplaceIcon';

	interface Props {
		tiers: EditableTier[];
		/** Shared field class from the parent form. */
		inputClass: string;
		/** Currency options for the per-tier override select ('' = inherit). */
		currencies: CurrencyInfo[];
	}

	let { tiers = $bindable(), inputClass, currencies }: Props = $props();

	// Client cap mirrors the seeded default service_packages.tiers.max_count=5;
	// the backend re-validates against the live setting.
	const MAX_TIERS = 5;

	function add() {
		tiers = [
			...tiers,
			{ name: '', badge: '', description: '', icon: '', priceAmount: '', currency: '', featuresText: '', isFeatured: false }
		];
	}

	function move(idx: number, dir: -1 | 1) {
		const j = idx + dir;
		if (j < 0 || j >= tiers.length) return;
		const next = [...tiers];
		[next[idx], next[j]] = [next[j], next[idx]];
		tiers = next;
	}

	// §114: at most one tier may be featured — radio semantics.
	function setFeatured(idx: number, on: boolean) {
		tiers = tiers.map((tr, i) => ({ ...tr, isFeatured: i === idx ? on : false }));
	}
</script>

{#snippet iconRow(value: string, onpick: (name: string) => void)}
	{@const Preview = marketplaceIcon(value)}
	<div class="flex items-center gap-2">
		<span
			class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)]"
			aria-hidden="true"
		>
			<Preview size={14} class="text-[var(--color-primary)]" />
		</span>
		<select
			value={value}
			onchange={(e) => onpick(e.currentTarget.value)}
			class="{inputClass} w-auto min-w-0 flex-1 sm:flex-none"
			aria-label={t('services.icon_pick', 'Icon')}
		>
			<option value="">{t('services.icon_none', 'No icon')}</option>
			{#each MARKETPLACE_ICON_NAMES as name (name)}
				<option value={name}>{name}</option>
			{/each}
		</select>
	</div>
{/snippet}

<div>
	<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
		{t('services.tier_items', 'Pricing tiers')}
	</span>
	<p class="mb-2 text-xs text-[var(--color-muted)]">
		{t('services.tier_hint', 'Optional Good / Better / Best pricing — travellers pick a tier at booking. Rows render in this order.')}
	</p>
	<div class="space-y-2">
		{#each tiers as tier, idx (idx)}
			<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
				<div class="flex items-start gap-2">
					<div class="flex flex-col">
						<button
							type="button"
							onclick={() => move(idx, -1)}
							disabled={idx === 0}
							class="text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-30"
							aria-label={t('common.move_up', 'Move up')}
						>
							<ChevronUp size={16} />
						</button>
						<button
							type="button"
							onclick={() => move(idx, 1)}
							disabled={idx === tiers.length - 1}
							class="text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-30"
							aria-label={t('common.move_down', 'Move down')}
						>
							<ChevronDown size={16} />
						</button>
					</div>
					<div class="flex-1 space-y-2">
						<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
							<input
								type="text"
								bind:value={tier.name}
								placeholder={t('services.tier_name_ph', 'Tier name (e.g. Standard)')}
								class={inputClass}
								aria-label={t('services.tier_name_ph', 'Tier name')}
							/>
							<input
								type="text"
								bind:value={tier.badge}
								placeholder={t('services.tier_badge_ph', 'Badge (e.g. Most popular — optional)')}
								class={inputClass}
								aria-label={t('services.tier_badge_ph', 'Tier badge')}
							/>
						</div>
						<input
							type="text"
							bind:value={tier.description}
							placeholder={t('services.tier_desc_ph', 'Short description (optional)')}
							class={inputClass}
							aria-label={t('services.tier_desc_ph', 'Tier description')}
						/>
						{@render iconRow(tier.icon, (v) => (tier.icon = v))}
						<div class="flex gap-2">
							<input
								type="number"
								min="0"
								step="0.01"
								bind:value={tier.priceAmount}
								placeholder={t('services.tier_price_ph', 'Price')}
								class="{inputClass} sm:w-32"
								aria-label={t('services.tier_price_ph', 'Tier price')}
							/>
							<select
								bind:value={tier.currency}
								class="{inputClass} sm:w-40"
								aria-label={t('services.tier_currency', 'Tier currency')}
							>
								<option value="">{t('services.tier_currency_inherit', 'Package currency')}</option>
								{#each currencies as c (c.code)}
									<option value={c.code}>{c.code}</option>
								{/each}
							</select>
						</div>
						<textarea
							bind:value={tier.featuresText}
							rows={3}
							placeholder={t('services.tier_features_ph', 'One feature per line (e.g. Airport pickup)')}
							class="{inputClass} resize-y"
							aria-label={t('services.tier_features_ph', 'Tier features')}
						></textarea>
						<label class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
							<input
								type="checkbox"
								checked={tier.isFeatured}
								onchange={(e) => setFeatured(idx, e.currentTarget.checked)}
								class="h-3.5 w-3.5 rounded border-[var(--color-border)]"
							/>
							<Star size={12} class={tier.isFeatured ? 'text-amber-500' : ''} />
							{t('services.tier_featured', 'Highlight this tier')}
						</label>
					</div>
					<button
						type="button"
						onclick={() => (tiers = tiers.filter((_, i) => i !== idx))}
						class="mt-1 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
						aria-label={t('common.remove', 'Remove')}
					>
						<Trash2 size={16} />
					</button>
				</div>
			</div>
		{/each}
	</div>
	{#if tiers.length < MAX_TIERS}
		<button
			type="button"
			onclick={add}
			class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
		>
			<Plus size={14} /> {t('services.tier_add', 'Add tier')}
		</button>
	{/if}
</div>
