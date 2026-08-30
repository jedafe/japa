<script lang="ts">
  import type { MapProps } from '../types';
  import type { FetchContext } from '../data/types';
  import { fetchEntityCollection } from '../data/entityFetchers';

  interface Props {
    props: MapProps;
    geoCountry?: string;
    fetchCtx?: FetchContext;
  }
  let { props, geoCountry, fetchCtx }: Props = $props();

  // Approximate world map positions (x%, y%) for top diaspora destinations.
  const countryPositions: Record<string, { x: number; y: number }> = {
    NG: { x: 50, y: 55 },
    GB: { x: 47, y: 32 },
    US: { x: 22, y: 38 },
    CA: { x: 22, y: 28 },
    DE: { x: 51, y: 32 },
    AU: { x: 82, y: 72 },
    IE: { x: 45, y: 32 },
    FR: { x: 48, y: 35 },
    NL: { x: 49, y: 31 },
    IT: { x: 51, y: 38 },
    ZA: { x: 54, y: 72 },
    AE: { x: 60, y: 48 },
  };

  type Pin = { country: string; label: string; href?: string };

  // When geo_source is 'trending_spaces', fetch the top country-level spaces
  // from the API so the map is not hardcoded (Rule 16). Falls back to the
  // block's manual pins if the API is unavailable or returns nothing.
  function loadTrendingPins(): Promise<Pin[]> {
    if (props.geo_source !== 'trending_spaces' || !fetchCtx) {
      return Promise.resolve(props.pins ?? []);
    }
    return fetchEntityCollection(
      'trending_space',
      { filters: { level: 'country' }, limit: 12 },
      fetchCtx
    ).then((items) => {
      const spaces = items as Array<{ space_geo_code?: string | null; geo_code?: string | null; name: string; slug: string }>;
      const pins = spaces
        .filter((s) => (s.space_geo_code || s.geo_code) && countryPositions[(s.space_geo_code || s.geo_code)!.toUpperCase()])
        .map((s) => ({
          country: (s.space_geo_code || s.geo_code) as string,
          label: s.name,
          href: `/spaces/${s.slug}`
        }));
      return pins.length > 0 ? pins : (props.pins ?? []);
    }).catch(() => props.pins ?? []);
  }

  const pinsPromise = loadTrendingPins();

  function pinsWithPosition(pinList: Pin[]) {
    return pinList.map((pin) => {
      const pos = countryPositions[pin.country.toUpperCase()] ?? { x: 50, y: 50 };
      const isVisitor = props.highlight_visitor_country && geoCountry && pin.country.toUpperCase() === geoCountry.toUpperCase();
      return { ...pin, ...pos, isVisitor };
    });
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-5xl rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8">
    <div class="relative aspect-[2/1] w-full rounded-[var(--radius)] bg-[var(--color-surface-subtle)]">
      <!-- Simplified world map background -->
      <svg class="absolute inset-0 h-full w-full opacity-20" viewBox="0 0 200 100" fill="none">
        <path d="M20 30 Q30 25 40 30 L45 40 L35 50 L25 45 Z" fill="var(--color-text)" />
        <path d="M45 25 Q55 20 65 25 L70 35 L60 45 L50 40 Z" fill="var(--color-text)" />
        <path d="M45 50 Q55 48 65 52 L70 65 L55 70 L48 60 Z" fill="var(--color-text)" />
        <path d="M75 60 Q85 58 90 62 L88 75 L80 72 Z" fill="var(--color-text)" />
      </svg>
      {#await pinsPromise}
        <div class="absolute inset-0 grid place-items-center text-sm text-[var(--color-muted)]">Loading map…</div>
      {:then loadedPins}
        {@const pins = pinsWithPosition(loadedPins)}
        {#each pins as pin}
          <div class="absolute flex flex-col items-center" style="top: {pin.y}%; left: {pin.x}%; transform: translate(-50%, -50%)">
            <div class="rounded-full {pin.isVisitor ? 'h-5 w-5 ring-4 ring-[var(--color-primary)] ring-opacity-30' : 'h-3 w-3'} bg-[var(--color-primary)] animate-pulse"></div>
            <span class="mt-1 whitespace-nowrap text-xs text-[var(--color-text)] bg-[var(--color-surface)] px-1.5 py-0.5 rounded shadow-sm">{pin.label}</span>
          </div>
        {/each}
      {:catch}
        {@const pins = pinsWithPosition(props.pins ?? [])}
        {#each pins as pin}
          <div class="absolute flex flex-col items-center" style="top: {pin.y}%; left: {pin.x}%; transform: translate(-50%, -50%)">
            <div class="rounded-full {pin.isVisitor ? 'h-5 w-5 ring-4 ring-[var(--color-primary)] ring-opacity-30' : 'h-3 w-3'} bg-[var(--color-primary)] animate-pulse"></div>
            <span class="mt-1 whitespace-nowrap text-xs text-[var(--color-text)] bg-[var(--color-surface)] px-1.5 py-0.5 rounded shadow-sm">{pin.label}</span>
          </div>
        {/each}
      {/await}
    </div>
  </div>
</section>
