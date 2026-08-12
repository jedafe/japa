<script lang="ts">
  import type { EntityFeatureProps } from '../types';
  import type { FetchContext } from '../data/types';
  import { fetchEntityFeature } from '../data/entityFetchers';

  interface Props {
    props: EntityFeatureProps;
    fetchCtx?: FetchContext;
  }
  let { props, fetchCtx }: Props = $props();

  let entity = $state<Record<string, unknown> | null>(null);
  let loading = $state(true);
  let error = $state(false);

  $effect(() => {
    const prefetched = (props as unknown as { _entity?: Record<string, unknown> })._entity;
    if (prefetched) { entity = prefetched; loading = false; return; }
    if (!fetchCtx || !props.ref?.id) { loading = false; return; }
    loading = true;
    fetchEntityFeature(props.entity_type, props.ref, fetchCtx)
      .then((result) => { entity = result as Record<string, unknown> | null; error = false; })
      .catch(() => { error = true; })
      .finally(() => { loading = false; });
  });

  const layoutClass = $derived(
    props.layout === 'split' ? 'grid grid-cols-1 lg:grid-cols-2 gap-8 items-center' :
    props.layout === 'banner' ? 'flex flex-col items-center text-center' :
    props.layout === 'card' ? 'max-w-2xl mx-auto' :
    'max-w-4xl mx-auto'
  );
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-6xl">
    {#if loading}
      <div class="h-64 animate-pulse rounded-[var(--radius-card)] bg-[var(--color-surface-subtle)]"></div>
    {:else if error || !entity}
      <!-- No entity data available — render nothing (seed fallback) -->
    {:else}
      <div class={layoutClass}>
        {#if props.layout === 'split' || props.layout === 'hero'}
          {#if props.media_override?.url || entity.cover_image_url || entity.featured_image_url || entity.logo_url || entity.image_url}
            <div class="overflow-hidden rounded-[var(--radius-card)]">
              <img
                src={(props.media_override?.url ?? entity.cover_image_url ?? entity.featured_image_url ?? entity.logo_url ?? entity.image_url) as string}
                alt={(props.media_override?.alt ?? '') as string}
                class="w-full object-cover"
                loading="lazy"
              />
            </div>
          {/if}
        {/if}
        <div>
          {#if entity.world_ranking}
            <p class="mb-2 text-sm font-semibold text-[var(--color-primary)]">World Ranking #{entity.world_ranking as number}</p>
          {/if}
          <h2 class="text-2xl font-bold text-[var(--color-text)] sm:text-3xl">
            {(entity.title ?? entity.name) as string}
          </h2>
          {#if entity.excerpt || entity.description}
            <p class="mt-3 text-[var(--color-text-subtle)]">{(entity.excerpt ?? entity.description) as string}</p>
          {/if}
          {#if entity.country}
            <p class="mt-2 text-sm text-[var(--color-muted)]">{entity.country as string}</p>
          {/if}
          <a
            href={entity.slug ? `/${props.entity_type === 'university' ? 'universities' : props.entity_type === 'blog' ? 'blog' : 'explore'}/${entity.slug as string}` : '#'}
            class="mt-6 inline-flex items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
          >
            Learn more &#8594;
          </a>
        </div>
      </div>
    {/if}
  </div>
</section>
