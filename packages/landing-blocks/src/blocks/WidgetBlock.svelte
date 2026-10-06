<script lang="ts">
  import { buildApiUrl } from '@japaconnect/api-url';
  import type { WidgetBlockProps } from '../types';
  import type { FetchContext } from '../data/types';
  import { getBlockContentContext } from '../shell/blockContent';

  interface Props {
    props: WidgetBlockProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
  }
  let { props, fetchCtx }: Props = $props();

  let data = $state<unknown>(null);
  let widgetType = $state<string>('');
  let cardType = $state<string>('');
  let loading = $state(true);
  let error = $state(false);

  const blockContent = getBlockContentContext();

  // Use pre-resolved data if injected by the server, otherwise fetch client-side.
  $effect(() => {
    // Server-side pre-resolved widget data (injected by GetPublishedPage).
    if (props._resolved) {
      data = props._resolved.data;
      widgetType = props._resolved.widget_type;
      cardType = props._resolved.card_type ?? '';
      loading = false;
      return;
    }
    if (!props.widget_id || !fetchCtx) {
      loading = false;
      return;
    }
    loading = true;
    const headers: Record<string, string> = {};
    if (fetchCtx.token) headers.authorization = `Bearer ${fetchCtx.token}`;
    if (fetchCtx.host) { headers.host = fetchCtx.host; headers['x-forwarded-host'] = fetchCtx.host; }
    fetch(buildApiUrl(`/widgets/${props.widget_id}`, fetchCtx.apiBase), { headers })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((json) => {
        // The API returns { data: RenderedWidget } where RenderedWidget is
        // { widget_id, widget_type, card_type, data: [...items...], cached_at }.
        // The actual items and metadata are nested under json.data.
        const rendered = json.data;
        data = rendered.data;
        widgetType = rendered.widget_type;
        cardType = rendered.card_type ?? '';
        error = false;
      })
      .catch(() => { error = true; })
      .finally(() => { loading = false; });
  });

  // Report whether this widget has content to the wrapping BlockShell.
  $effect(() => {
    if (loading) return;
    const dataObj = data && typeof data === 'object' ? (data as Record<string, unknown>) : null;
    const html = dataObj?.html;
    const hasContent = !error && (items.length > 0 || (typeof html === 'string' && html.trim().length > 0));
    blockContent?.set(hasContent);
  });

  // The card_type set on /widgets is the source of truth for how cards render.
  // Falls back to the block's layout prop, then 'grid'.
  const effectiveLayout = $derived(cardType || props.layout || 'grid');

  const colsClass = $derived(
    props.columns === 1 ? 'grid-cols-1' :
    props.columns === 2 ? 'grid-cols-1 sm:grid-cols-2' :
    props.columns === 3 ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3' :
    props.columns === 4 ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-4' :
    'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'
  );

  // Extract items array from widget data (all data-source widgets return arrays).
  const items = $derived(
    Array.isArray(data) ? data :
    (data && typeof data === 'object' && Array.isArray((data as Record<string, unknown>).items)) ?
      (data as Record<string, unknown>).items as unknown[] :
    []
  );

  // Resolve a deep link for a widget data item.
  //
  // The URL is determined by two signals:
  //   1. The widget_type (e.g. "blog_posts" → /blog/{slug}) — used when the
  //      widget is dedicated to a single content type.
  //   2. The item's content_type field (e.g. "blog_post", "discussion") —
  //      used by mixed-content widgets like latest_content and featured_content
  //      where items can be of any content type.
  //
  // Route paths match the actual SvelteKit routes in apps/main:
  //   /blog/[slug], /forum/[slug], /jobs/[slug], /content/[slug],
  //   /achievements/[slug], /spaces/[slug], /profiles/[slug],
  //   /org/[slug], /tags/[slug], /u/[username], /mentors/[username],
  //   /calendar/[id], /digest/[id], /compare/[slug]
  function itemHref(item: Record<string, unknown>): string {
    const slug = item.slug as string | undefined;
    const id = item.id as string | undefined;
    const username = item.username as string | undefined;

    // --- Items with content_type (latest_content, featured_content) ---
    // The contents table uses: blog_post, discussion, comparison, guide, etc.
    const ct = item.content_type as string | undefined;
    if (ct) {
      switch (ct) {
        case 'blog_post': return slug ? `/blog/${slug}` : '#';
        case 'discussion': return slug ? `/forum/${slug}` : '#';
        case 'comparison': return slug ? `/compare/${slug}` : '#';
        case 'guide': return slug ? `/content/${slug}` : '#';
        default: return slug ? `/content/${slug}` : '#';
      }
    }

    // --- Items without content_type — use the widget type to pick the route ---
    switch (widgetType) {
      case 'blog_posts': return slug ? `/blog/${slug}` : '#';
      case 'forum_threads': return slug ? `/forum/${slug}` : '#';
      case 'job_listings': return slug ? `/jobs/${slug}` : '#';
      case 'achievements': return slug ? `/achievements/${slug}` : '#';
      case 'academy_tracks': return slug ? `/academy/${slug}` : '#';
      case 'featured_spaces': return slug ? `/spaces/${slug}` : '#';
      case 'professional_profiles': return slug ? `/profiles/${slug}` : '#';
      case 'community_organisations': return slug ? `/org/${slug}` : '#';
      case 'trending_tags': return slug ? `/tags/${slug}` : '#';
      case 'upcoming_events': return id ? `/calendar/${id}` : '#';
      case 'user_leaderboard': return username ? `/u/${username}` : '#';
      case 'mentor_spotlight': return username ? `/mentors/${username}` : '#';
      case 'community_digest': return id ? `/digest/${id}` : '#';
      case 'reviews':
        // Reviews don't have their own detail page — link to the reviewed entity.
        return '#';
      case 'comparisons':
        // Comparisons link via the parent content slug, not their own ID.
        return '#';
      case 'space_members': return username ? `/u/${username}` : '#';
      default: return slug ? `/content/${slug}` : '#';
    }
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-6xl">
    {#if props.heading}
      <div class="mb-6 flex items-center justify-between">
        <h2 class="text-2xl font-bold text-[var(--color-text)]">{props.heading}</h2>
        {#if props.view_all_href}
          <a href={props.view_all_href} class="text-sm font-medium text-[var(--color-primary)] hover:underline">View all →</a>
        {/if}
      </div>
    {/if}

    {#if loading}
      <div class="grid {colsClass} gap-6">
        {#each Array(6) as _}
          <div class="h-48 animate-pulse rounded-[var(--radius-card)] bg-[var(--color-surface-subtle)]"></div>
        {/each}
      </div>
    {:else if error}
      <div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center text-[var(--color-muted)]">
        <p>Unable to load widget data. Please try again later.</p>
      </div>
    {:else if widgetType === 'custom_html' && data && typeof data === 'object' && 'html' in (data as Record<string, unknown>)}
      <!-- custom_html widget — HTML is sanitized server-side by content.SanitizeHTML (§5.1/§14) -->
      <div class="prose prose-sm max-w-none text-[var(--color-text)]">
        {@html (data as Record<string, unknown>).html as string}
      </div>
    {:else if items.length === 0}
      <div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center text-[var(--color-muted)]">
        <p>Nothing here yet.</p>
      </div>
    {:else if effectiveLayout === 'list'}
      <div class="grid {colsClass} gap-4">
        {#each items as item}
          {@const entity = item as Record<string, unknown>}
          <a href={itemHref(entity)} class="block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-all hover:shadow-md">
            <div class="flex items-center justify-between">
              <h3 class="text-base font-semibold text-[var(--color-text)]">{(entity.title ?? entity.name) as string}</h3>
              {#if entity.created_at || entity.published_at}
                <span class="text-xs text-[var(--color-muted)]">{new Date((entity.published_at ?? entity.created_at) as string).toLocaleDateString()}</span>
              {/if}
            </div>
            {#if entity.excerpt || entity.description || entity.body}
              <p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{(entity.excerpt ?? entity.description ?? entity.body) as string}</p>
            {/if}
          </a>
        {/each}
      </div>
    {:else if effectiveLayout === 'compact'}
      <div class="flex flex-wrap gap-2">
        {#each items as item}
          {@const entity = item as Record<string, unknown>}
          <a href={itemHref(entity)} class="rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-all hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]">
            {(entity.title ?? entity.name ?? entity.tag) as string}
          </a>
        {/each}
      </div>
    {:else if effectiveLayout === 'ghost'}
      <div class="grid {colsClass} gap-6">
        {#each items as item}
          {@const entity = item as Record<string, unknown>}
          <div class="divide-y divide-[var(--color-border)]">
            <a href={itemHref(entity)} class="group block py-5 transition-colors hover:bg-[var(--color-surface-subtle)] -mx-3 px-3 rounded-[var(--radius)]">
              <div class="flex items-baseline justify-between gap-4">
                <h3 class="text-lg font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)] transition-colors">
                  {(entity.title ?? entity.name) as string}
                </h3>
                {#if entity.created_at || entity.published_at}
                  <time class="shrink-0 text-xs text-[var(--color-muted)]">
                    {new Date((entity.published_at ?? entity.created_at) as string).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
                  </time>
                {/if}
              </div>
              {#if entity.excerpt || entity.description || entity.tagline || entity.bio}
                <p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{(entity.excerpt ?? entity.description ?? entity.tagline ?? entity.bio) as string}</p>
              {/if}
              {#if entity.author_name || entity.author_username}
                <p class="mt-1.5 text-xs text-[var(--color-muted)]">by {(entity.author_name ?? entity.author_username) as string}</p>
              {/if}
            </a>
          </div>
        {/each}
      </div>
    {:else if effectiveLayout === 'magazine'}
      <div class="grid {colsClass} gap-6">
        {#each items as item, i}
          {@const entity = item as Record<string, unknown>}
          {#if i === 0}
            <a href={itemHref(entity)} class="group lg:col-span-2 lg:row-span-2 block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] overflow-hidden transition-all hover:shadow-lg">
              {#if entity.cover_image_url || entity.featured_image_url || entity.image_url}
                <img src={(entity.cover_image_url ?? entity.featured_image_url ?? entity.image_url) as string} alt="" class="h-64 w-full object-cover lg:h-80" loading="lazy" />
              {/if}
              <div class="p-5">
                {#if entity.content_type || entity.badge}
                  <span class="mb-2 inline-block rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">{(entity.content_type ?? entity.badge) as string}</span>
                {/if}
                <h3 class="text-xl font-bold text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
                  {(entity.title ?? entity.name) as string}
                </h3>
                {#if entity.excerpt || entity.description}
                  <p class="mt-2 line-clamp-3 text-sm text-[var(--color-text-subtle)]">{(entity.excerpt ?? entity.description) as string}</p>
                {/if}
                {#if entity.author_name || entity.author_username || entity.created_at || entity.published_at}
                  <p class="mt-3 text-xs text-[var(--color-muted)]">
                    {#if entity.author_name || entity.author_username}by {(entity.author_name ?? entity.author_username) as string}{/if}
                    {#if entity.created_at || entity.published_at} · {new Date((entity.published_at ?? entity.created_at) as string).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}{/if}
                  </p>
                {/if}
              </div>
            </a>
          {:else}
            <a href={itemHref(entity)} class="group flex gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-all hover:shadow-md">
              {#if entity.cover_image_url || entity.featured_image_url || entity.image_url}
                <img src={(entity.cover_image_url ?? entity.featured_image_url ?? entity.image_url) as string} alt="" class="h-16 w-16 shrink-0 rounded-[var(--radius)] object-cover" loading="lazy" />
              {/if}
              <div class="min-w-0 flex-1">
                <h3 class="line-clamp-2 text-sm font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
                  {(entity.title ?? entity.name) as string}
                </h3>
                {#if entity.excerpt || entity.description}
                  <p class="mt-1 line-clamp-1 text-xs text-[var(--color-text-subtle)]">{(entity.excerpt ?? entity.description) as string}</p>
                {/if}
                {#if entity.created_at || entity.published_at}
                  <p class="mt-1 text-[10px] text-[var(--color-muted)]">{new Date((entity.published_at ?? entity.created_at) as string).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</p>
                {/if}
              </div>
            </a>
          {/if}
        {/each}
      </div>
    {:else if effectiveLayout === 'reddit'}
      <div class="grid {colsClass} gap-2">
        {#each items as item}
          {@const entity = item as Record<string, unknown>}
          <a href={itemHref(entity)} class="group flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-all hover:border-[var(--color-primary)] hover:shadow-sm">
            {#if entity.vote_score !== undefined || entity.view_count !== undefined}
              <div class="flex shrink-0 flex-col items-center justify-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1 text-xs font-semibold text-[var(--color-text-subtle)] min-w-[2.5rem]">
                {#if entity.vote_score !== undefined}
                  <span>{entity.vote_score as number}</span>
                  <span class="text-[9px] text-[var(--color-muted)]">votes</span>
                {:else}
                  <span>{entity.view_count as number}</span>
                  <span class="text-[9px] text-[var(--color-muted)]">views</span>
                {/if}
              </div>
            {/if}
            {#if entity.cover_image_url || entity.featured_image_url || entity.logo_url || entity.image_url}
              <img src={(entity.cover_image_url ?? entity.featured_image_url ?? entity.logo_url ?? entity.image_url) as string} alt="" class="h-10 w-10 shrink-0 rounded-[var(--radius)] object-cover" loading="lazy" />
            {/if}
            <div class="min-w-0 flex-1">
              <h3 class="truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
                {(entity.title ?? entity.name) as string}
              </h3>
              <div class="mt-0.5 flex items-center gap-2 text-xs text-[var(--color-muted)]">
                {#if entity.author_name || entity.author_username}
                  <span>{(entity.author_name ?? entity.author_username) as string}</span>
                {/if}
                {#if entity.comment_count !== undefined}
                  <span>· {entity.comment_count as number} comments</span>
                {/if}
                {#if entity.content_type}
                  <span>· {entity.content_type as string}</span>
                {/if}
              </div>
            </div>
            {#if entity.created_at || entity.published_at}
              <time class="shrink-0 text-xs text-[var(--color-muted)]">
                {new Date((entity.published_at ?? entity.created_at) as string).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
              </time>
            {/if}
          </a>
        {/each}
      </div>
    {:else}
      <div class="grid {colsClass} gap-6">
        {#each items as item}
          {@const entity = item as Record<string, unknown>}
          <a
            href={itemHref(entity)}
            class="group block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-all hover:shadow-lg hover:-translate-y-1"
          >
            {#if entity.cover_image_url || entity.featured_image_url || entity.logo_url || entity.image_url}
              <img src={(entity.cover_image_url ?? entity.featured_image_url ?? entity.logo_url ?? entity.image_url) as string} alt="" class="mb-3 h-32 w-full rounded-[var(--radius)] object-cover" loading="lazy" />
            {/if}
            {#if entity.badge || entity.difficulty || entity.org_type}
              <span class="mb-2 inline-block rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">{(entity.badge ?? entity.difficulty ?? entity.org_type) as string}</span>
            {/if}
            <h3 class="text-base font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
              {(entity.title ?? entity.name) as string}
            </h3>
            {#if entity.excerpt || entity.description || entity.tagline || entity.bio}
              <p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{(entity.excerpt ?? entity.description ?? entity.tagline ?? entity.bio) as string}</p>
            {/if}
            {#if entity.rating || entity.rating_avg}
              <div class="mt-2 flex items-center gap-1 text-sm text-[var(--color-muted)]">
                <span>★</span>
                <span>{(entity.rating ?? entity.rating_avg) as string | number}</span>
              </div>
            {/if}
            {#if entity.country || entity.location || entity.address}
              <p class="mt-2 text-xs text-[var(--color-muted)]">{(entity.country ?? entity.location ?? entity.address) as string}</p>
            {/if}
          </a>
        {/each}
      </div>
    {/if}
  </div>
</section>
