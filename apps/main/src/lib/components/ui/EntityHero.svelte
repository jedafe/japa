<script lang="ts">
	/**
	 * EntityHero — rich directory-list header: accent-tinted gradient panel
	 * with a breadcrumb row, icon badge, display title, description, real-data
	 * stat chips, a CTA row and a per-entity decorative `art` scene on the
	 * right (desktop only).
	 *
	 * The `actions` snippet receives the hero's button class trio so pages can
	 * compose CTAs that share the panel's accent without re-declaring styles.
	 */
	import { page } from '$app/state';
	import { t } from '#lib/i18n/index.js';
	import { Check, ChevronRight, Share2 } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import type { IconComponent } from './icons';

	export interface HeroStat {
		icon: IconComponent;
		value: string;
		label: string;
		/** Per-stat tint (defaults to the hero accent). */
		tint?: string;
	}

	export interface HeroCrumb {
		label: string;
		href?: string;
	}

	export interface HeroActionClasses {
		primary: string;
		secondary: string;
		ghost: string;
	}

	interface Props {
		icon: IconComponent;
		heading: string;
		description?: string;
		/** Entity accent — drives gradient, badge, primary CTA and art tint. */
		accent?: string;
		crumbs?: HeroCrumb[];
		stats?: HeroStat[];
		/** Renders the share pill (native share → clipboard fallback). */
		share?: boolean;
		art?: Snippet;
		actions?: Snippet<[HeroActionClasses]>;
		class?: string;
	}

	let {
		icon: Icon,
		heading,
		description = '',
		accent = '#3b49df',
		crumbs = [],
		stats = [],
		share = true,
		art,
		actions,
		class: classProp = ''
	}: Props = $props();

	const actionClasses: HeroActionClasses = {
		primary:
			'inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--hero-accent)] px-4 py-2.5 text-sm font-medium text-[var(--hero-contrast)] shadow-sm transition-colors hover:bg-[color-mix(in_srgb,var(--hero-accent)_85%,var(--hero-contrast))]',
		secondary:
			'inline-flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]',
		ghost:
			'inline-flex items-center gap-1 px-1 py-2.5 text-sm font-medium text-[var(--hero-accent)] transition-opacity hover:opacity-75'
	};

	let shared = $state(false);
	let shareTimer: ReturnType<typeof setTimeout> | undefined;

	async function sharePage() {
		const url = page.url.href;
		try {
			if (navigator.share) {
				await navigator.share({ title: heading, url });
			} else {
				await navigator.clipboard.writeText(url);
			}
			shared = true;
			clearTimeout(shareTimer);
			shareTimer = setTimeout(() => (shared = false), 2000);
		} catch {
			// dismissed or unavailable — leave the button idle
		}
	}
</script>

<section
	class="entity-hero relative overflow-hidden rounded-[20px] border {classProp}"
	style="--hero-accent: {accent}; margin-inline: var(--page-gutter, 0.75rem)"
	aria-labelledby="entity-hero-heading"
>
	<div class="relative px-5 py-6 sm:px-8 sm:py-8">
		<!-- Breadcrumb + share row -->
		<div class="flex items-center justify-between gap-3">
			<nav aria-label="Breadcrumb" class="flex min-w-0 items-center gap-1.5 text-xs text-[var(--color-muted)]">
				{#each crumbs as crumb, i (crumb.label)}
					{#if i > 0}
						<ChevronRight size={12} class="shrink-0 rtl:-scale-x-100" />
					{/if}
					{#if crumb.href}
						<a href={crumb.href} class="truncate transition-colors hover:text-[var(--color-text)]">
							{crumb.label}
						</a>
					{:else}
						<span class="truncate font-medium text-[var(--color-text-subtle)]" aria-current="page">
							{crumb.label}
						</span>
					{/if}
				{/each}
			</nav>
			{#if share}
				<button
					type="button"
					onclick={sharePage}
					class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3.5 py-1.5 text-xs font-medium text-[var(--color-text)] shadow-sm transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{#if shared}
						<Check size={13} class="text-[var(--color-success)]" />
						{t('common.copied', 'Copied')}
					{:else}
						<Share2 size={13} />
						{t('common.share', 'Share')}
					{/if}
				</button>
			{/if}
		</div>

		<div class="mt-5 grid items-center gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,380px)]">
			<div class="min-w-0">
				<div class="flex items-center gap-4">
					<span
						class="grid h-14 w-14 shrink-0 place-items-center rounded-2xl text-[var(--hero-contrast)] shadow-sm sm:h-16 sm:w-16"
						style="background: linear-gradient(135deg, var(--hero-accent), color-mix(in srgb, var(--hero-accent) 72%, var(--hero-contrast)))"
					>
						<Icon size={30} strokeWidth={1.8} />
					</span>
					<div class="min-w-0">
						<h1
							id="entity-hero-heading"
							class="break-words text-2xl font-bold tracking-tight text-[var(--color-text)] sm:whitespace-nowrap sm:text-[2rem]"
						>
							{heading}
						</h1>
						{#if description}
							<p class="mt-1 max-w-md text-sm leading-relaxed text-[var(--color-text-subtle)]">
								{description}
							</p>
						{/if}
					</div>
				</div>

				{#if stats.length > 0}
					<div class="mt-6 flex flex-wrap items-center gap-x-7 gap-y-3">
						{#each stats as stat (stat.label)}
							{@const StatIcon = stat.icon}
							<div class="flex items-center gap-2.5">
								<span
									class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)]"
									style="background: color-mix(in srgb, {stat.tint ?? 'var(--hero-accent)'} 12%, transparent); color: {stat.tint ?? 'var(--hero-accent)'}"
								>
									<StatIcon size={17} />
								</span>
								<span class="min-w-0">
									<span class="block text-base font-bold leading-tight text-[var(--color-text)]">
										{stat.value}
									</span>
									<span class="block text-xs text-[var(--color-muted)]">{stat.label}</span>
								</span>
							</div>
						{/each}
					</div>
				{/if}

				{#if actions}
					<div class="mt-6 flex flex-wrap items-center gap-3">
						{@render actions(actionClasses)}
					</div>
				{/if}
			</div>

			{#if art}
				<!-- Decorative scene — desktop only. pointer-events-none: the
				     art can overlap the left-column CTA row and swallow clicks. -->
				<div class="pointer-events-none relative hidden h-56 lg:block" aria-hidden="true">
					{@render art()}
				</div>
			{/if}
		</div>
	</div>
</section>

<style>
	.entity-hero {
		--hero-contrast: #ffffff;
		border-color: color-mix(in srgb, var(--hero-accent) 18%, var(--color-border));
		background:
			radial-gradient(
				130% 170% at 88% 0%,
				color-mix(in srgb, var(--hero-accent) 16%, transparent),
				transparent 55%
			),
			radial-gradient(
				110% 140% at -4% 112%,
				color-mix(in srgb, var(--hero-accent) 8%, transparent),
				transparent 55%
			),
			linear-gradient(
				135deg,
				color-mix(in srgb, var(--hero-accent) 7%, var(--color-surface)) 0%,
				color-mix(in srgb, var(--hero-accent) 13%, var(--color-surface)) 100%
			);
	}

	/* Dark theme — the panel reads as a tinted surface, not a colour card;
	   brightened accents need a dark foreground on the primary CTA. */
	:global([data-theme='dark']) .entity-hero {
		--hero-contrast: color-mix(in srgb, var(--hero-accent) 18%, #0a0a0a);
		border-color: color-mix(in srgb, var(--hero-accent) 24%, var(--color-border));
		background:
			radial-gradient(
				130% 170% at 88% 0%,
				color-mix(in srgb, var(--hero-accent) 10%, transparent),
				transparent 55%
			),
			radial-gradient(
				110% 140% at -4% 112%,
				color-mix(in srgb, var(--hero-accent) 5%, transparent),
				transparent 55%
			),
			linear-gradient(
				135deg,
				color-mix(in srgb, var(--hero-accent) 4%, var(--color-surface)) 0%,
				color-mix(in srgb, var(--hero-accent) 8%, var(--color-surface)) 100%
			);
	}
</style>
