# Phase 7 Implementation Patterns

**Date:** 2026-06-27
**Phase:** 7
**Purpose:** Implementation patterns learned during Phase 7 (Sections 09, 23, 26, 39, 44), reusable in future sections.

## Pattern: SvelteKit SSR Data Loading

Every page that needs server-side data follows a two-file pattern:

```typescript
// +page.server.ts — runs on the server, never in the browser
import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ locals, params, cookies }) => {
  const token = cookies.get('jc_token');
  const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

  const result = await apiFetch<{ data: DataType }>(`/api/v1/resource/${params.id}`, { headers });

  if (!result.ok) {
    return { data: null, error: result.error };
  }

  return { data: result.data.data };
};
```

```svelte
<!-- +page.svelte — receives data from server load -->
<script lang="ts">
  let { data } = $props();
</script>

{#if data}
  <div>{data.title}</div>
{:else}
  <ErrorState message="Not found" />
{/if}
```

Key rules:
- `+page.server.ts` runs only on the server; no browser APIs
- `apiFetch` wraps `fetch` with error handling and base URL
- Auth token comes from cookies, not localStorage (SSR context)
- `locals.user` is populated by `hooks.server.ts`
- Data flows one direction: server load -> page props -> components

**Trade-off:** SSR adds latency per request (server-side API call). For pages with auth-gated data, this is unavoidable. For public pages, consider caching the API response in Valkey with a short TTL.

## Pattern: Navigation Shell Composition

The root layout assembles the navigation shell from discrete components:

```svelte
<!-- +layout.svelte -->
<script lang="ts">
  import Header from '$lib/components/nav/Header.svelte';
  import MobileNav from '$lib/components/nav/MobileNav.svelte';
  import Footer from '$lib/components/nav/Footer.svelte';
  import SideNav from '$lib/components/nav/SideNav.svelte';
  import { theme } from '$lib/stores/theme';

  let { children, data } = $props();
  let sidenavOpen = $state(false);

  $effect(() => {
    theme.init(data.user?.preferences?.dark_mode ?? 'system');
  });
</script>

<div class="flex min-h-screen flex-col">
  <Header
    user={data.user}
    notificationCount={data.notificationCount}
    onsidenavToggle={() => (sidenavOpen = !sidenavOpen)}
  />
  <SideNav open={sidenavOpen} user={data.user} onclose={() => (sidenavOpen = false)} />
  <main class="flex-1 pb-16 md:pb-0">
    {@render children()}
  </main>
  <Footer />
</div>

<MobileNav notificationCount={data.notificationCount} />
```

Key rules:
- `data` from `+layout.server.ts` provides user, feature flags, notification count
- SideNav state is managed in the layout (not in individual pages)
- MobileNav is outside the flex container to sit fixed at the bottom
- `pb-16` on main compensates for MobileNav height on small screens

**Trade-off:** The layout re-renders when `sidenavOpen` changes, but Svelte's fine-grained reactivity means only the SideNav component actually re-renders. The rest of the shell is stable.

## Pattern: Dark Mode with CSS Custom Properties

Theme switching uses CSS custom properties and a class toggle:

```typescript
// src/lib/stores/theme.ts
import { browser } from '$app/environment';

type Theme = 'light' | 'dark' | 'system';

let currentTheme: Theme = $state('system');

function applyTheme(theme: Theme) {
  const html = document.documentElement;
  if (theme === 'system') {
    const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    html.classList.toggle('dark', prefersDark);
  } else {
    html.classList.toggle('dark', theme === 'dark');
  }
}

export function getTheme() {
  return {
    get current() { return currentTheme; },
    set: (theme: Theme) => {
      currentTheme = theme;
      if (browser) {
        applyTheme(theme);
        fetch('/api/v1/users/me/preferences', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ dark_mode: theme })
        }).catch(() => {});
      }
    },
    init: (serverTheme: Theme) => {
      currentTheme = serverTheme;
      if (browser) applyTheme(serverTheme);
    }
  };
}

export const theme = getTheme();
```

Key rules:
- `browser` guard prevents SSR errors (no `document` on server)
- `init()` hydrates from server-side preference (no flash of wrong theme)
- `set()` persists to the backend and applies locally
- CSS in `app.css` uses `.dark` class to swap custom property values

```css
:root {
  --color-bg: #ffffff;
  --color-text: #1a1a1a;
}
.dark {
  --color-bg: #0a0a0a;
  --color-text: #e5e5e5;
}
```

**Trade-off:** Class-based toggling requires `browser` guards and manual persistence. The alternative (CSS `prefers-color-scheme` only) doesn't allow user override. The current approach supports all three modes (light, dark, system).

## Pattern: Onboarding Wizard Step State Machine

The onboarding wizard manages step progression as a state machine:

```typescript
// +page.svelte
<script lang="ts">
  import { enhance } from '$app/forms';
  import type { PageData } from './$types';

  let { data } = $props();
  let currentStep = $state(data.onboarding.current_step);
  let stepData = $state<Record<string, unknown>>(data.onboarding.step_data);
  let submitting = $state(false);

  const steps = [
    { id: 1, label: 'Welcome', component: WelcomeStep },
    { id: 2, label: 'Destination', component: DestinationStep },
    { id: 3, label: 'Interests', component: InterestsStep },
    { id: 4, label: 'Spaces', component: SpacesStep },
    { id: 5, label: 'First Action', component: FirstActionStep }
  ];

  async function nextStep() {
    submitting = true;
    try {
      await fetch('/api/v1/onboarding/step', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ step: currentStep, data: stepData })
      });
      if (currentStep < 5) {
        currentStep++;
      } else {
        await fetch('/api/v1/onboarding/complete', { method: 'POST' });
        goto('/');
      }
    } finally {
      submitting = false;
    }
  }
</script>
```

Key rules:
- Step data is accumulated across steps (not replaced)
- Backend upserts the current step without resetting previous data
- Completion calls the `/complete` endpoint and redirects
- The wizard can be re-entered if not completed (resume from last step)

**Trade-off:** JSONB step_data is flexible but lacks schema validation at the database level. Validation happens in the service layer before upsert. If step data becomes complex, consider adding a JSON schema column or validation middleware.

## Pattern: RBAC-Gated Audience Documentation

Documentation uses URL-based audience gating with RBAC on admin endpoints:

```typescript
// [audience]/+page.server.ts
export const load: PageServerLoad = async ({ params, locals }) => {
  const { audience } = params;

  // Validate audience
  if (!['user', 'admin', 'developer'].includes(audience)) {
    throw error(404, 'Unknown audience');
  }

  // Public docs don't require auth
  const result = await apiFetch<DocPage[]>(`/docs/${audience}`);
  return { pages: result.ok ? result.data : [], audience };
};
```

```go
// Admin endpoints require RBAC
r.With(auth.RequireAuth(kr), rbac.RequirePermission(rbacResolver, "docs.admin")).
    Post("/admin/docs", docsHandlers.CreateDocPage)
```

Key rules:
- Public docs (`GET /docs/{audience}`) have no auth requirement
- Admin CRUD requires `docs.admin` permission
- Audience is validated server-side (not just client-side)
- The `[audience]` segment is a SvelteKit dynamic parameter

**Trade-off:** Audience gating at the URL level means different audiences have different routes. This is clean for SEO (each audience has its own URL space) but requires duplicate layout files if audiences need different navigation. The current approach shares a single layout with a dynamic sidebar.

## Pattern: MetadataHead Component for SEO

Each page composes its own metadata rather than using a global layout component:

```svelte
<!-- +page.svelte -->
<script lang="ts">
  import MetadataHead from '$lib/seo/MetadataHead.svelte';
  import JsonLd from '$lib/seo/JsonLd.svelte';
  import { articleSchema } from '$lib/seo/structuredData';

  let { data } = $props();
</script>

<MetadataHead
  title={data.page.title}
  description={data.page.description}
  image={data.page.og_image}
  url={`https://japaconnect.com/content/${data.page.slug}`}
  type="article"
/>

<JsonLd schema={articleSchema(data.page)} />

<article>
  <h1>{data.page.title}</h1>
  <!-- content -->
</article>
```

```svelte
<!-- MetadataHead.svelte -->
<script lang="ts">
  let { title, description, image, url, type = 'website', noindex = false } = $props();

  let robotsContent = $derived(noindex ? 'noindex, nofollow' : 'index, follow');
</script>

<svelte:head>
  <title>{title} — JapaConnect</title>
  <meta name="description" content={description} />
  <meta name="robots" content={robotsContent} />
  <meta property="og:title" content={title} />
  <meta property="og:description" content={description} />
  <meta property="og:image" content={image} />
  <meta property="og:url" content={url} />
  <meta property="og:type" content={type} />
  <meta name="twitter:card" content="summary_large_image" />
  <link rel="canonical" href={url} />
</svelte:head>
```

Key rules:
- `MetadataHead` must be at the top level (not inside conditionals or elements)
- `<svelte:head>` content is merged across components (not replaced)
- `noindex` prop is reactive via `$derived`
- Schema helpers are functions that accept dynamic data

**Trade-off:** Per-page composition means every page must import and configure `MetadataHead`. A global default could reduce boilerplate but would make per-page overrides harder. The current approach prioritizes explicit control.

## Pattern: Geo-Space as Domain Entity

Universities reuse the geo-space infrastructure rather than creating a dedicated table:

```typescript
// Universities are geo-spaces with space_type = 'university'
const university = await apiFetch<Space>(`/spaces/${slug}`);
// University data comes from custom_field_values:
// - programs (custom field)
// - ranking (custom field)
// - country (custom field)
// - acceptance_rate (custom field)
```

```svelte
<!-- UniversityCard.svelte -->
<script lang="ts">
  let { university } = $props();
  // university.name, university.slug, university custom fields
</script>

<a href="/universities/{university.slug}" class="block rounded-lg border p-4">
  <h3>{university.name}</h3>
  <p>{university.custom_fields?.country}</p>
  <p>Ranking: #{university.custom_fields?.ranking}</p>
</a>
```

Key rules:
- `space_type` distinguishes universities from other geo-spaces
- Program and admission data are stored as custom field values
- Content within the space (programs, articles) uses the content type system
- No new database tables needed — the space + custom fields + content system is sufficient

**Trade-off:** Custom fields are flexible but lack type safety at the query level. If university-specific queries become complex (e.g., "find universities with acceptance rate < 30%"), consider adding computed columns or a materialized view. For now, the custom field approach avoids schema proliferation.

## Lesson: hooks.server.ts for Auth Hydration

The `hooks.server.ts` file runs on every request and populates `event.locals`:

```typescript
export const handle: Handle = async ({ event, resolve }) => {
  const token = event.cookies.get('jc_token');

  if (token) {
    try {
      const res = await fetch(`${API_BASE_URL}/users/me`, {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (res.ok) {
        const { data } = await res.json();
        event.locals.user = {
          id: data.id,
          email: data.email,
          username: data.username,
          displayName: data.display_name,
          avatarUrl: data.avatar_url,
          profileType: data.profile_type,
          status: data.status
        };
      } else {
        event.locals.user = null;
      }
    } catch {
      event.locals.user = null;
    }
  } else {
    event.locals.user = null;
  }

  return resolve(event);
};
```

Key rules:
- `locals` is available in all `+page.server.ts` and `+layout.server.ts` load functions
- Invalid tokens set `user` to `null` (don't throw — let pages handle auth)
- The API call happens on every request (no caching). For high-traffic, cache in Valkey with a short TTL.

**Trade-off:** Fetching `/users/me` on every request adds latency. The alternative (caching in Valkey) adds complexity. For a community platform with moderate traffic, the simplicity of per-request fetching is acceptable.

## Lesson: Sidebar Collapse on Mobile

The SideNav uses CSS media queries and a state toggle:

```svelte
<!-- SideNav.svelte -->
<script lang="ts">
  let { open, user, onclose } = $props();
</script>

<!-- Backdrop -->
{#if open}
  <div class="fixed inset-0 z-40 bg-black/50 md:hidden" onclick={onclose}></div>
{/if}

<!-- Panel -->
<aside class="
  fixed inset-y-0 left-0 z-50 w-64 transform bg-white transition-transform
  {open ? 'translate-x-0' : '-translate-x-full'}
  md:relative md:translate-x-0 md:z-auto
">
  <!-- nav items -->
</aside>
```

Key rules:
- `md:` breakpoint separates mobile (slide-over) from desktop (fixed sidebar)
- Backdrop only renders on mobile (`md:hidden`)
- `translate-x` transition provides smooth open/close animation
- Desktop sidebar is always visible (`md:relative md:translate-x-0`)

**Trade-off:** The `md:` breakpoint is fixed at 768px. If users frequently resize between mobile and desktop, consider a JavaScript media query listener. For most users, the CSS breakpoint is sufficient.
