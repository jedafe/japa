<script lang="ts">
	import { ChevronDown } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import type { DocAudience } from '$lib/types';
	import { AUDIENCE_LABEL_KEY } from '$lib/types';

	interface Props {
		accessibleAudiences: DocAudience[];
		activeAudience: DocAudience | null;
	}

	let { accessibleAudiences, activeAudience }: Props = $props();

	let open = $state(false);

	function select(audience: DocAudience) {
		open = false;
		// Navigation happens via the anchor href; this just closes the menu.
	}

	let currentLabel = $derived(
		activeAudience ? t(AUDIENCE_LABEL_KEY[activeAudience], activeAudience) : t('audience.pick_audience')
	);
</script>

<div class="switcher">
	<button
		type="button"
		class="trigger"
		onclick={() => (open = !open)}
		aria-haspopup="listbox"
		aria-expanded={open}
	>
		<span class="label">{t('nav.audience')}</span>
		<span class="current">{currentLabel}</span>
		<ChevronDown size={14} class={`chev ${open ? 'rot' : ''}`} />
	</button>

	{#if open}
		<button
			type="button"
			class="backdrop"
			aria-label={t('a11y.close_menu')}
			onclick={() => (open = false)}
		></button>
		<ul class="menu" role="listbox">
			{#each accessibleAudiences as audience (audience)}
				<li>
					<a
						href="/docs/{audience}"
						role="option"
						aria-selected={audience === activeAudience}
						class="item"
						class:active={audience === activeAudience}
						onclick={() => select(audience)}
					>
						{t(AUDIENCE_LABEL_KEY[audience], audience)}
					</a>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.switcher { position: relative; }
	.trigger {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.5rem 0.625rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		cursor: pointer;
		text-align: left;
	}
	.trigger:hover { border-color: var(--color-border-input-hover); }
	.label {
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-muted);
		font-weight: 500;
	}
	.current {
		flex: 1;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	:global(.chev) { color: var(--color-muted); transition: transform 0.15s; flex-shrink: 0; }
	:global(.chev.rot) { transform: rotate(180deg); }

	.backdrop {
		position: fixed;
		inset: 0;
		z-index: 40;
		background: transparent;
		border: none;
		cursor: default;
	}
	.menu {
		position: absolute;
		top: calc(100% + 4px);
		left: 0;
		right: 0;
		z-index: 50;
		list-style: none;
		margin: 0;
		padding: 0.25rem;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		box-shadow: 0 8px 24px rgb(0 0 0 / 0.12);
		max-height: 320px;
		overflow-y: auto;
	}
	.item {
		display: block;
		padding: 0.5rem 0.625rem;
		border-radius: calc(var(--radius) - 2px);
		font-size: 0.875rem;
		color: var(--color-text);
		text-decoration: none;
	}
	.item:hover { background: var(--color-surface-subtle); }
	.item.active {
		background: color-mix(in srgb, var(--color-primary) 12%, transparent);
		color: var(--color-primary);
		font-weight: 500;
	}
</style>
