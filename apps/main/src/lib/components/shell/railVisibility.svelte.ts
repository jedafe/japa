// Both shell rails (RightSidebar, LeftRail) are display:none below 1280px and
// the side nav is display:none below 768px — but all stay mounted so desktop
// hide/show toggles (e.g. the /ai rail swap, the mobile drawer) don't replay
// widget fetches and empty-state flashes. Widgets and ad units inside a rail
// read this flag to skip their mount-time fetches while the rail can never
// be visible — a mobile page view must not pay /ads/serve, /blog/featured,
// /calendar, etc. for an invisible rail.
//
// Flags are evaluated at module eval in the browser (matchMedia), before any
// consumer $effect runs; SSR defaults to true so server markup is unchanged.
// When a resized viewport crosses the breakpoint the flag flips true and the
// gated effects re-run, so a mobile→desktop transition still fetches.
//
// Rails opt their subtree in via provideRailGate(); the same components used
// in-page or in always-visible containers (article AdSlots, the mobile drawer,
// page-level SidebarWidgetRenderer) get no context and keep their
// unconditional-fetch behavior.
import { getContext, setContext } from 'svelte';

function viewportFlag(query: string): () => boolean {
	let v = $state(true);
	if (typeof window !== 'undefined') {
		const mq = window.matchMedia(query);
		v = mq.matches;
		mq.addEventListener('change', (e) => {
			v = e.matches;
		});
	}
	return () => v;
}

const railVisible = viewportFlag('(min-width: 1280px)');
const navRailVisible = viewportFlag('(min-width: 768px)');

const RAIL_CTX = Symbol('rail-visibility');

/** Called by a rail container (RightSidebar, LeftRail, the .shell-nav LeftNav)
 *  at init — marks its subtree as viewport-gated. Defaults to the 1280px
 *  sidebar breakpoint; the nav rail passes navRailVisible for its 768px one. */
export function provideRailGate(flag: () => boolean = railVisible): void {
	setContext(RAIL_CTX, flag);
}

/** The 768px flag for the side-nav rail — display:none below 768px. */
export { navRailVisible };

/** Read inside a widget/ad $effect: returns a getter for rail visibility when
 *  mounted inside a rail, undefined when mounted in-page. Call sites do
 *  `const gate = railGate();` at init, then `if (gate && !gate()) return;`
 *  inside the effect so the flag read is tracked and the effect re-fires on
 *  the hidden→visible flip. */
export function railGate(): (() => boolean) | undefined {
	return getContext<(() => boolean) | undefined>(RAIL_CTX);
}
