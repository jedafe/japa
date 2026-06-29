# Section 23 — Main Site Frontend

**Status:** ✅ Locked

## Decided
Navigation: code-defined fixed slots (`primary_nav`, `mobile_bottom_tab`, `footer`, `user_dropdown`) populated by the admin-configurable Navigation Manager — structure is code, content within it is admin-managed. Bottom tab bar on mobile (Home/Feed, Spaces, Search, Notifications, Profile) over a hamburger-only pattern, given expected mobile-heavy usage. Visible Primary Space switcher in the header given relocation changes it.

Notification centre: dropdown (desktop) / full-screen (mobile), live updates via the WS `notification` event, real-time unread badge. Profile settings catalog: Account, Privacy, Notification preferences, Language, Spaces, Billing, Data & Privacy, Organization management. Shared error/empty-state component (404/403/500/empty-list), visually distinct from loading skeletons.

**UI component stack:** Tailwind CSS v4 for styling, **Bits UI** (headless Svelte primitives: Dialog, Dropdown, Tooltip, Select, Combobox, Popover, etc.) for all interactive component foundations, **Lucide Svelte** for icons. Bespoke platform components (Entity Card, Feed Card, Comparison Matrix, Ad Slot, Verification Badge indicator, Reaction Bar, Bookmark Button, etc.) are built on top of Bits UI primitives rather than a second headless library. All three SvelteKit apps share the same Bits UI version.

## Dark Mode (restored post-audit, see ADR-029)

System-default on first visit; user-selectable Light / Dark / System toggle stored in profile preferences (server-side, so it follows the user across devices rather than being a per-device CSS cookie). Implemented via CSS custom properties / Tailwind v4 dark-mode class strategy — no separate stylesheet download; theme switches without a page reload. The toggle lives in the user dropdown header area.

## Timezone-Aware Display (restored post-audit, see ADR-029)

All timestamps stored as UTC; all displayed times converted to the user's configured timezone (stored in profile preferences, defaulting to browser-detected timezone on first visit). Relative time ("2 hours ago") uses the user's timezone as the reference point. Content with explicit times — Events (V2 Content Extension), scheduled posts — show both the local time in the user's timezone and the original timezone of the content in a tooltip, since a "London event at 6pm" means different things to a viewer in Lagos vs Toronto. Timezone preference lives in the same profile settings catalog as Language.

## Timeline & Feed UI (session 5 update)

**Homepage routing (clarified):**
- Authenticated users → Primary Timeline (`/` or `/feed`)
- Unauthenticated users → Explore Timeline (`/explore`)
- Enforced at the SvelteKit root layout server load function — no client-side redirect, correct layout server-rendered directly based on session.

**Sticky Sort Filter Bar:**
All timeline views (Primary, Explore, Space, User/Profile) render a four-option sort selector that is sticky on scroll — it stays pinned at the top of the viewport beneath the main navigation header as the user scrolls:

```svelte
<!-- FeedSortBar.svelte — position: sticky; top: var(--nav-height) -->
<!-- Options: Relevant | Latest | Popular | Rising -->
<!-- Active mode stored in per-timeline session state (not server preferences) -->
<!-- On mobile: displayed as a horizontally scrollable chip row -->
```

The sort bar uses `position: sticky` with `top` set to the main nav height CSS variable so it clears the fixed navigation header correctly on both desktop and mobile.

## Animation Strategy (session 6 — ADR-051)

**Two libraries, zero runtime overhead:**

**AutoAnimate** (`@formkit/auto-animate`, 2.5KB gzipped) — applied via Svelte action to any reactive list: feed cards, notification lists, comment threads, search results, space member lists. Items animate in, out, and reorder automatically without configuration.

```svelte
<script>import autoAnimate from '@formkit/auto-animate';</script>
<ul use:autoAnimate>
  {#each $feedItems as item (item.id)}<FeedCard {item} />{/each}
</ul>
```

**Svelte built-in** (`svelte/transition`, `svelte/animate`) — used for all UI element transitions: modals (`fly` + `fade`), drawers (`slide`), toasts (`fly` + `flip`), skeleton loaders. Zero bundle cost — already part of the Svelte runtime.

**Motion (motion.dev)** — available as a per-component upgrade path for complex micro-interactions. Not in the base bundle; imported only where needed and tree-shaken by Vite.

**Reduced-motion:** All animation durations gate on a `prefersReducedMotion` store (`window.matchMedia('(prefers-reduced-motion: reduce)')`). When true, all durations are set to 0 — transitions fire but complete instantly. This applies globally.

## Instant Navigation (session 6 — ADR-051)

SvelteKit 2.x already includes the InstantClick pattern natively. No external library is needed.

**Hover prefetching** — applied globally in the root layout:
```svelte
<svelte:body data-sveltekit-preload-data="hover" />
<svelte:body data-sveltekit-preload-code="viewport" />
```
On hover (desktop) or touchstart (mobile), SvelteKit fires the page's `load()` function immediately. By the time the user clicks, the data is already loaded — navigation is instant.

**View Transitions API** — smooth page-to-page animations using the browser-native API (Chrome 111+, Edge; graceful fallback on unsupported browsers):
```svelte
<!-- +layout.svelte -->
<script>
  import { onNavigate } from '$app/navigation';
  onNavigate((nav) => {
    if (!document.startViewTransition) return;
    return new Promise((resolve) => {
      document.startViewTransition(async () => { resolve(); await nav.complete; });
    });
  });
</script>
```
Page transitions default to a 120ms cross-fade. Named `view-transition-name` elements (e.g., content hero images) morph between the list card and the detail page.
