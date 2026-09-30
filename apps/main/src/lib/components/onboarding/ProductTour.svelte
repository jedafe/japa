<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { driver, type DriveStep } from 'driver.js';
	import 'driver.js/dist/driver.css';

	// Section 09 Product Tour: a lightweight contextual walkthrough that
	// triggers immediately after the five-step wizard completes, highlighting
	// the primary nav, the Space switcher, and the post-creation entry point.
	// Dismissible at any point, never re-triggered automatically; a "Replay
	// tour" link lives in profile settings for users who want to see it again.

	let {
		/** If true, force the tour to start even if previously dismissed
		 *  (used by the "Replay tour" link in profile settings). */
		replay = false,
		/** Ordered tour target keys (data-tour attribute values) from
		 *  onboarding.tour_steps — managed in Control → Settings. `null`/
		 *  undefined falls back to the built-in defaults; an explicit empty
		 *  array disables the tour for this surface. */
		steps = null
	}: { replay?: boolean; steps?: string[] | null } = $props();

	let tour: ReturnType<typeof driver> | null = null;

	// Built-in default step order (mirrors the seeded onboarding.tour_steps
	// value): nav overview, the key feature entries top-to-bottom, then the
	// bottom blocks (spaces, compose), the right sidebar, and the account
	// entry. Steps whose target isn't rendered (feature-flagged menus,
	// hidden mobile/desktop elements) are skipped automatically.
	const defaultStepKeys = [
		'primary-nav',
		'home',
		'notifications',
		'messages',
		'stories',
		'forum',
		'discover',
		'explore',
		'learn',
		'jobs',
		'achievements',
		'bookmarks',
		'search',
		'space-switcher',
		'compose',
		'sidebar',
		'account'
	];

	// i18n prefix overrides for targets whose tour.<key>_title/_desc keys
	// don't follow the kebab→snake convention.
	const i18nPrefix: Record<string, string> = {
		'primary-nav': 'nav',
		'space-switcher': 'spaces'
	};

	const tourSteps = $derived(
		(steps == null ? defaultStepKeys : steps)
			.map((key) => {
				const prefix = i18nPrefix[key] ?? key.replaceAll('-', '_');
				return {
					selector: `[data-tour="${key}"]`,
					title: t(`tour.${prefix}_title`, ''),
					description: t(`tour.${prefix}_desc`, '')
				};
			})
			// Skip keys with no translation — admin-added targets without
			// i18n entries would otherwise render an empty popover.
			.filter((s) => s.title !== '')
	);

	// driver.js resolves string selectors via querySelector, which always
	// returns the first DOM match — but the shell mounts [data-tour] targets
	// twice (hidden desktop nav + mobile drawer / bottom tab bar). Resolve to
	// the first *visible* match instead so steps track the rendered shell.
	function firstVisible(selector: string): Element | null {
		for (const el of document.querySelectorAll(selector)) {
			const rect = el.getBoundingClientRect();
			if (rect.width > 0 && rect.height > 0) return el;
		}
		return null;
	}

	// Strip the ?tour trigger param once the tour has started — otherwise a
	// page refresh re-runs it, and ?tour=replay would loop forever since it
	// intentionally bypasses the dismissed guard.
	function cleanupUrl() {
		const url = new URL(page.url);
		if (!url.searchParams.has('tour')) return;
		url.searchParams.delete('tour');
		replaceState(url, page.state);
	}

	// Auto-retry: when a mounted tour finds no visible targets (exception
	// pages render no shell chrome), stay armed and re-attempt once per
	// navigation until it can actually run. A skip never records dismissal.
	let pendingStart = false;
	let retryTimer: ReturnType<typeof setTimeout> | null = null;

	function startTour() {
		if (!browser || tour) return; // already running

		cleanupUrl();

		const steps: DriveStep[] = tourSteps
			.map((s) => ({ el: firstVisible(s.selector), def: s }))
			.filter((x): x is { el: Element; def: (typeof tourSteps)[number] } => x.el !== null)
			.map(({ el, def }) => ({
				element: el,
				popover: { title: def.title, description: def.description }
			}));

		// No visible targets on this page (hidden nav on small screens,
		// exception routes) — skip without dismissing so the tour can still
		// run on a page that renders the shell targets.
		if (steps.length === 0) {
			pendingStart = true;
			return;
		}
		pendingStart = false;

		tour = driver({
			showProgress: true,
			popoverClass: 'jc-tour',
			nextBtnText: t('tour.next', 'Next'),
			prevBtnText: t('tour.prev', 'Back'),
			doneBtnText: t('tour.done', 'Done'),
			progressText: t('tour.progress', '{{current}} of {{total}}'),
			onCloseClick: () => {
				void dismissTour();
				tour?.destroy();
				tour = null;
			},
			onDestroyStarted: () => {
				void dismissTour();
				tour?.destroy();
				tour = null;
			},
			steps
		});

		tour.drive();
	}

	async function dismissTour() {
		// POST /onboarding/tour/dismiss — records the "main" surface dismissal
		// in onboarding_state.tour_dismissals so it never re-triggers (§09).
		await api.post('/onboarding/tour/dismiss', { surface: 'main' });
	}

	onMount(() => {
		if (!browser) return;
		if (replay) {
			// Small delay to let the app shell render the tour targets.
			setTimeout(startTour, 300);
		}
	});

	// Stop the tour on real navigation — driver.js caches the resolved
	// Elements, which detach when SvelteKit swaps the page DOM. The ?tour
	// cleanup uses replaceState (same pathname), so it can't trip this.
	let initialPath = '';
	$effect(() => {
		const path = page.url.pathname;
		if (!initialPath) {
			initialPath = path;
			return;
		}
		if (path !== initialPath) {
			initialPath = path;
			if (retryTimer) {
				clearTimeout(retryTimer);
				retryTimer = null;
			}
			if (tour) {
				// destroy() (not the close callback) — no dismissal is
				// recorded, so an undismissed tour can run again later.
				tour.destroy();
				tour = null;
			} else if (pendingStart) {
				// The tour was armed but this page had no targets — retry on
				// the destination route.
				retryTimer = setTimeout(startTour, 300);
			}
		}
	});

	// Re-trigger: the layouts latch this component mounted, so a second
	// ?tour=replay in the same session re-appears the param without
	// re-running onMount — restart the tour when the param transitions.
	let seenTourParam: string | null = null;
	$effect(() => {
		const p = page.url.searchParams.get('tour');
		if (p && p !== seenTourParam) {
			seenTourParam = p;
			setTimeout(startTour, 300);
		} else if (!p) {
			seenTourParam = null;
		}
	});
</script>

{#if replay}
	<!-- Replay mode: the tour auto-starts on mount. No visible UI. -->
{/if}

<style>
	/* Theme driver.js popovers onto the platform tokens (driver.css ships
	   hardcoded light-theme colors). Scoped via the jc-tour popoverClass. */
	:global(.driver-popover.jc-tour) {
		background-color: var(--color-surface);
		color: var(--color-text);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card, 12px);
		font-family: var(--font-sans, inherit);
		box-shadow: 0 8px 30px rgb(0 0 0 / 0.18);
	}
	:global(.jc-tour .driver-popover-title) {
		color: var(--color-text);
	}
	:global(.jc-tour .driver-popover-description) {
		color: var(--color-text-subtle);
	}
	:global(.jc-tour .driver-popover-progress-text) {
		color: var(--color-muted);
	}
	:global(.jc-tour .driver-popover-close-btn) {
		color: var(--color-muted);
	}
	:global(.jc-tour .driver-popover-close-btn:hover),
	:global(.jc-tour .driver-popover-close-btn:focus) {
		color: var(--color-text);
	}
	:global(.jc-tour .driver-popover-footer-btn) {
		background-color: var(--color-surface);
		color: var(--color-text);
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
	}
	:global(.jc-tour .driver-popover-footer-btn:hover),
	:global(.jc-tour .driver-popover-footer-btn:focus) {
		background-color: var(--color-surface-subtle);
	}
	:global(.jc-tour .driver-popover-next-btn) {
		background-color: var(--color-primary);
		color: var(--color-primary-contrast);
		border-color: var(--color-primary);
	}
	:global(.jc-tour .driver-popover-next-btn:hover),
	:global(.jc-tour .driver-popover-next-btn:focus) {
		background-color: var(--color-primary-hover, var(--color-primary));
	}
	:global(.jc-tour .driver-popover-arrow-side-left) {
		border-left-color: var(--color-surface);
	}
	:global(.jc-tour .driver-popover-arrow-side-right) {
		border-right-color: var(--color-surface);
	}
	:global(.jc-tour .driver-popover-arrow-side-top) {
		border-top-color: var(--color-surface);
	}
	:global(.jc-tour .driver-popover-arrow-side-bottom) {
		border-bottom-color: var(--color-surface);
	}
</style>
