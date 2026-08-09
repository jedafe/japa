<script lang="ts">
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { driver } from 'driver.js';
	import 'driver.js/dist/driver.css';

	// Section 09 Product Tour: a lightweight contextual walkthrough that
	// triggers immediately after the five-step wizard completes, highlighting
	// the primary nav, the Space switcher, and the post-creation entry point.
	// Dismissible at any point, never re-triggered automatically; a "Replay
	// tour" link lives in profile settings for users who want to see it again.

	let {
		/** If true, force the tour to start even if previously dismissed
		 *  (used by the "Replay tour" link in profile settings). */
		replay = false
	}: { replay?: boolean } = $props();

	let tour: ReturnType<typeof driver> | null = null;

	const tourSteps = $derived([
		{
			element: '[data-tour="primary-nav"]',
			tourTitle: { title: t('tour.nav_title', 'Navigation'), description: t('tour.nav_desc', 'Use the primary navigation to move between your feed, spaces, notifications, and profile.') }
		},
		{
			element: '[data-tour="space-switcher"]',
			tourTitle: { title: t('tour.spaces_title', 'Space Switcher'), description: t('tour.spaces_desc', 'Switch between your Spaces — geographic and topic-based communities you belong to.') }
		},
		{
			element: '[data-tour="compose"]',
			tourTitle: { title: t('tour.compose_title', 'Create a Post'), description: t('tour.compose_desc', 'Click here to compose a new post, discussion, or question in any Space you belong to.') }
		}
	]);

	function startTour() {
		if (!browser) return;

		tour = driver({
			showProgress: true,
			nextBtnText: t('tour.next', 'Next'),
			prevBtnText: t('tour.prev', 'Back'),
			doneBtnText: t('tour.done', 'Done'),
			progressText: t('tour.progress', '{current} of {total}'),
			onCloseClick: () => {
				dismissTour();
				tour?.destroy();
				tour = null;
			},
			onDestroyStarted: () => {
				dismissTour();
				tour?.destroy();
				tour = null;
			},
			steps: tourSteps.map((s) => ({
				element: s.element,
				...s.tourTitle
			}))
		});

		tour.drive();
	}

	async function dismissTour() {
		// POST /onboarding/tour/dismiss — marks tour_dismissed=true so it
		// never re-triggers automatically (Section 09).
		await api.post('/onboarding/tour/dismiss', {});
	}

	onMount(() => {
		if (!browser) return;
		if (replay) {
			// Small delay to let the app shell render the tour targets.
			setTimeout(startTour, 300);
		}
	});
</script>

{#if replay}
	<!-- Replay mode: the tour auto-starts on mount. No visible UI. -->
{/if}
