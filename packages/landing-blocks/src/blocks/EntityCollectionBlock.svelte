<script lang="ts">
  import type { EntityCollectionProps } from '../types';
  import type { FetchContext } from '../data/types';
  import { fetchEntityCollection } from '../data/entityFetchers';
  import { getBlockContentContext } from '../shell/blockContent';

  interface Props {
    props: EntityCollectionProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
  }
  let { props, fetchCtx, geoCountry }: Props = $props();

  let items = $state<unknown[]>([]);
  let loading = $state(true);
  let error = $state(false);

  const blockContent = getBlockContentContext();

  // Use pre-fetched data if provided (server-side injection), otherwise fetch client-side.
  $effect(() => {
    // Pre-fetched data injected by the server (e.g. default seed).
    const prefetched = (props as unknown as { _items?: unknown[] })._items;
    if (prefetched) { items = prefetched; loading = false; return; }
    if (!fetchCtx) { loading = false; return; }
    loading = true;
    fetchEntityCollection(props.entity_type, props.query, fetchCtx, geoCountry)
      .then((result) => { items = result; error = false; })
      .catch(() => { error = true; })
      .finally(() => { loading = false; });
  });

  // Report content state to the wrapping BlockShell for `hide_if_empty`.
  $effect(() => {
    if (loading) return;
    blockContent?.set(!error && items.length > 0);
  });

  const colsClass = $derived(
    props.columns === 2 ? 'grid-cols-1 sm:grid-cols-2' :
    props.columns === 3 ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3' :
    props.columns === 4 ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-4' :
    'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'
  );

  // Resolve a per-entity deep link to the correct detail route.
  // Mirrors the canonical FeedCard link logic (apps/main FeedCard.svelte)
  // and the actual SvelteKit route shapes — there is NO /explore/[slug]
  // detail route, so space/thread/post must NOT link to /explore/{slug}.
  function entityHref(type: typeof props.entity_type, entity: Record<string, unknown>): string {
    const slug = (entity.slug ?? entity.id) as string | undefined;
    // Tools carry their own deep link (TOOL_REGISTRY href).
    if (type === 'tool') return (entity.href as string | undefined) ?? '#';
    if (!slug) return '#';
    switch (type) {
      case 'blog': return `/blog/${slug}`;
      case 'job': return `/jobs/${slug}`;
      case 'university': return `/universities/${slug}`;
      case 'space':
      case 'trending_space':
      case 'organization': return `/spaces/${slug}`;
      case 'thread': return `/forum/${slug}`;
      case 'post': {
        // Mixed feed items link by content_type (blog_post/forum_thread/other).
        const ct = entity.content_type as string | undefined;
        if (ct === 'blog_post') return `/blog/${slug}`;
        if (ct === 'forum_thread') return `/forum/${slug}`;
        return `/content/${slug}`;
      }
      // No per-item detail route exists yet — link to the section list.
      case 'scholarship': return `/scholarships`;
      default: return '#';
    }
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-6xl">
    {#if loading}
      <div class="grid {colsClass} gap-6">
        {#each Array(props.query.limit ?? 6) as _}
          <div class="h-48 animate-pulse rounded-[var(--radius-card)] bg-[var(--color-surface-subtle)]"></div>
        {/each}
      </div>
    {:else if error}
      <div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center text-[var(--color-muted)]">
        <p>Unable to load content. Please try again later.</p>
      </div>
    {:else if items.length === 0}
      {#if !props.query.filters?.hide_if_empty}
        <div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center text-[var(--color-muted)]">
          <p>Nothing here yet.</p>
        </div>
      {/if}
    {:else}
      <div class="grid {colsClass} gap-6">
        {#each items as item}
          {@const entity = item as Record<string, unknown>}
          <a
            href={entityHref(props.entity_type, entity)}
            class="group block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-all hover:shadow-lg hover:-translate-y-1"
          >
            {#if entity.cover_image_url || entity.featured_image_url || entity.logo_url || entity.image_url}
              <img src={(entity.cover_image_url ?? entity.featured_image_url ?? entity.logo_url ?? entity.image_url) as string} alt="" class="mb-3 h-32 w-full rounded-[var(--radius)] object-cover" loading="lazy" />
            {/if}
            {#if entity.badge}
              <span class="mb-2 inline-block rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]">{entity.badge as string}</span>
            {/if}
            <h3 class="text-base font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
              {(entity.title ?? entity.name) as string}
            </h3>
            {#if entity.excerpt || entity.description}
              <p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{(entity.excerpt ?? entity.description) as string}</p>
            {/if}
            {#if entity.country || entity.location}
              <p class="mt-2 text-xs text-[var(--color-muted)]">{(entity.country ?? entity.location) as string}</p>
            {/if}
          </a>
        {/each}
      </div>
    {/if}
  </div>
</section>
