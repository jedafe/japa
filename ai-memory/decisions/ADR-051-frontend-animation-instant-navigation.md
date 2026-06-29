# ADR-051: Frontend UX — Animation Strategy & Instant Navigation

**Status:** Accepted

**Context:** Two frontend UX decisions were deferred during the core architecture phases: (1) how to add motion and animation to the SvelteKit UI without adding significant bundle weight, and (2) how to make navigation feel instant (the SvelteKit equivalent of the InstantClick pattern discussed in earlier planning sessions).

---

## Animation Strategy

**Decision: AutoAnimate + Svelte native transitions. No third-party animation runtime.**

**AutoAnimate (`@formkit/auto-animate`)** — 2.5KB gzipped, zero-config, works as a Svelte action. Chosen for all list/collection animations: feed items appearing, notification lists updating, space member counts changing, search result transitions, comment threads loading.

```svelte
<!-- Feed list — items animate in/out/reorder automatically -->
<script>
  import autoAnimate from '@formkit/auto-animate';
</script>

<ul use:autoAnimate>
  {#each $feedItems as item (item.id)}
    <FeedCard {item} />
  {/each}
</ul>
```

**Svelte built-in transitions (`svelte/transition`, `svelte/animate`)** — zero bundle cost, already available. Used for: modal dialogs (fly/fade), drawer sidebars (slide), toast notifications (fly), page-level element transitions, skeleton loaders.

```svelte
<!-- Modal — native Svelte transition, zero library cost -->
{#if open}
  <div transition:fly={{ y: 20, duration: 200, easing: cubicOut }}>
    <slot />
  </div>
{/if}

<!-- Toast notifications — fly in from bottom-right -->
{#each $toasts as toast (toast.id)}
  <div animate:flip={{ duration: 250 }}
       in:fly={{ x: 40, duration: 200 }}
       out:fade={{ duration: 150 }}>
    <Toast {toast} />
  </div>
{/each}
```

**Motion (motion.dev)** — explicitly deferred. The Web Animations API-based library is available as an upgrade path if micro-interactions on specific components (e.g., card hover lift, progress rings, animated counters) require more control than Svelte's built-in easing provides. Not included in the base bundle; imported per-component only if needed, tree-shaken by Vite.

**Rejected: GSAP, Framer Motion, anime.js.** All add 50KB+ to the bundle and bring far more capability than a content-focused community platform needs. The combination of AutoAnimate (2.5KB) + Svelte native achieves all required motion design at negligible cost.

**Design principle:** Motion serves information, not spectacle. Durations: 150–250ms for micro-interactions, 200–300ms for page-level transitions. Easing: `cubicOut` for entrances, `cubicIn` for exits. `prefers-reduced-motion` media query is respected via Svelte's `prefersReducedMotion` store — all animations are skipped when the user has set this OS preference.

```svelte
<!-- Global reduced-motion guard in root layout -->
<script>
  import { prefersReducedMotion } from '$lib/stores/accessibility';
  // All animation durations multiply by: $prefersReducedMotion ? 0 : 1
</script>
```

---

## Instant Navigation

**Decision: SvelteKit built-in prefetching + View Transitions API. No external navigation library.**

SvelteKit 2.x has InstantClick behaviour built in. The two-part implementation is:

**Part 1 — Hover prefetching (data-sveltekit-preload-data).** Applied globally in the root layout. When a user hovers a link (desktop) or touches it (mobile), SvelteKit fires the server `load()` function immediately — before the click registers. By the time the click fires, the data is already loaded. Navigation is effectively instant.

```svelte
<!-- src/routes/+layout.svelte — applies to ALL links site-wide -->
<svelte:body data-sveltekit-preload-data="hover" />

<!-- For code chunks (JS) — preload on viewport entry -->
<svelte:body data-sveltekit-preload-code="viewport" />
```

**Part 2 — View Transitions API.** Applied in the root layout's `onNavigate` hook. When the browser supports the View Transitions API (Chrome 111+, Edge, growing Safari support), page navigations are wrapped in `document.startViewTransition()`, producing smooth cross-fade or slide animations between pages. Falls back silently to an instant swap on unsupported browsers.

```svelte
<!-- src/routes/+layout.svelte -->
<script>
  import { onNavigate } from '$app/navigation';

  onNavigate((navigation) => {
    if (!document.startViewTransition) return; // graceful degradation
    return new Promise((resolve) => {
      document.startViewTransition(async () => {
        resolve();              // let SvelteKit update the DOM
        await navigation.complete;
      });
    });
  });
</script>
```

```css
/* app.css — view transition names for per-element cross-fade */
.feed-card { view-transition-name: auto; }

/* Page cross-fade — default for all navigations */
::view-transition-old(root) { animation: 120ms ease-out both fade-out; }
::view-transition-new(root) { animation: 150ms ease-in both fade-in; }

/* Hero element — content detail page shares element with card thumbnail */
.hero-image { view-transition-name: hero-image; }
::view-transition-old(hero-image),
::view-transition-new(hero-image) {
  animation: none; /* Browser handles the morphing */
}
```

**Why no external library:** Libraries like `instant.page`, `quicklink`, and `swup` solve problems SvelteKit already solves natively. Adding them would either duplicate SvelteKit's router logic or fight against it. The built-in combination is zero-dependency, always in sync with SvelteKit's routing model, and benefits from framework-level maintenance.

**Consequences:** `data-sveltekit-preload-data="hover"` increases server load because `load()` functions fire on hover, not just click. This is the same tradeoff InstantClick makes. Mitigation: ensure all `load()` functions are fast (cached Valkey lookups, indexed DB queries) and use SvelteKit's `maxAge` cache option for heavily-trafficked pages. The View Transitions animation CSS lives in `app.css` and is tuned per major layout region. Progressive enhancement means zero regression on unsupported browsers.

**Bundle impact summary:**

| Concern | Solution | Bundle cost |
|---|---|---|
| List / feed animations | AutoAnimate | 2.5KB gzipped |
| UI element transitions | Svelte built-in | 0 (runtime) |
| Complex micro-interactions | Motion (per-component) | 0 base; tree-shaken on use |
| Instant navigation | SvelteKit preload built-in | 0 |
| Page transitions | View Transitions API | 0 (browser native) |
