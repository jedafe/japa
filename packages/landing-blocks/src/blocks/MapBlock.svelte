<script lang="ts">
  import type { MapProps } from '../types';
  import type { FetchContext } from '../data/types';
  import { fetchEntityCollection } from '../data/entityFetchers';
  import { countryCentroid } from '../geo/countryCentroids';

  interface Props {
    props: MapProps;
    geoCountry?: string;
    fetchCtx?: FetchContext;
  }
  let { props, geoCountry, fetchCtx }: Props = $props();

  type Pin = { country: string; label: string; href?: string; x: number; y: number };

  // Project a country centroid onto the 2:1 map plane using an
  // equirectangular projection — x: lng∈[-180,180] → [0,100]%, y:
  // lat∈[90,-90] → [0,100]%. Any ISO alpha-2 code with a known centroid
  // gets a position (previously only 12 hardcoded countries could render).
  function project(pin: { country: string; label: string; href?: string }): Pin | null {
    const c = countryCentroid(pin.country);
    if (!c) return null;
    return { ...pin, x: (c[1] + 180) / 3.6, y: (90 - c[0]) / 1.8 };
  }

  // 'manual' geo_source uses only the configured pins; anything else (unset
  // included) pulls top country-level spaces so the map self-populates (Rule 16).
  // Falls back to manual pins when the API is unavailable or returns nothing.
  function loadPins(): Promise<Pin[]> {
    const manual = () => (props.pins ?? []).map(project).filter((p): p is Pin => p !== null);
    if (props.geo_source === 'manual' || !fetchCtx) {
      return Promise.resolve(manual());
    }
    return fetchEntityCollection(
      'trending_space',
      { filters: { level: 'country' }, limit: 12 },
      fetchCtx
    ).then((items) => {
      const spaces = items as Array<{ space_geo_code?: string | null; geo_code?: string | null; name: string; slug: string }>;
      const pins = spaces
        .map((s) =>
          project({
            country: (s.space_geo_code || s.geo_code || '') as string,
            label: s.name,
            href: `/spaces/${s.slug}`,
          })
        )
        .filter((p): p is Pin => p !== null);
      return pins.length > 0 ? pins : manual();
    }).catch(manual);
  }

  const pinsPromise = loadPins();

  function pinFlags(pin: Pin) {
    const isVisitor = props.highlight_visitor_country && geoCountry && pin.country.toUpperCase() === geoCountry.toUpperCase();
    return { isVisitor };
  }
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-5xl rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8">
    <div class="relative aspect-[2/1] w-full rounded-[var(--radius)] bg-[var(--color-surface-subtle)]">
      <!-- World map — soft water wash + graticule + muted land silhouettes
           in the same equirectangular projection the pins use -->
      <svg class="absolute inset-0 h-full w-full" viewBox="0 0 200 100" preserveAspectRatio="none" fill="none">
        <rect width="200" height="100" fill="#4d7fa8" opacity="0.22" />
        <g stroke="#6b8ea6" stroke-width="0.25" opacity="0.4">
          <line x1="0" y1="25" x2="200" y2="25" /><line x1="0" y1="50" x2="200" y2="50" /><line x1="0" y1="75" x2="200" y2="75" />
          <line x1="50" y1="0" x2="50" y2="100" /><line x1="100" y1="0" x2="100" y2="100" /><line x1="150" y1="0" x2="150" y2="100" />
        </g>
        <g fill="#6d9273" stroke="#6d9273" stroke-width="0.7" stroke-linejoin="round" opacity="0.8">
        <!-- North America -->
        <path d="M6.7 13.3 L10 11.1 L22.2 11.1 L30.6 10 L47.2 9.4 L55.6 10 L61.1 16.7 L67.8 18.9 L69.4 22.2 L63.3 25.6 L61.1 27.2 L58.3 30 L55.6 32.2 L55 36.1 L50 33.9 L46.1 35.6 L46.1 38.3 L47.8 40.6 L51.7 41.7 L53.9 45 L56.7 45.6 L52.8 43.3 L48.9 41.7 L46.1 39.4 L41.7 38.3 L38.9 36.7 L36.7 33.3 L32.8 30 L31.1 26.7 L31.1 23.3 L27.2 19.4 L22.2 16.7 L15.6 17.2 L10 16.7 Z" />
        <!-- Greenland -->
        <path d="M75 16.7 L88.9 11.1 L86.1 5.6 L66.7 4.4 L61.1 7.2 L67.8 14.4 L71.1 16.7 Z" />
        <!-- South America -->
        <path d="M56.7 45.6 L66.7 45.6 L71.1 47.2 L72.2 50 L75.6 51.1 L80.6 54.4 L79.4 58.3 L77.8 62.2 L73.3 65.6 L71.1 68.3 L67.8 69.4 L65.6 72.8 L63.3 76.1 L62.2 78.3 L61.1 80 L59.4 77.8 L59.4 72.2 L60 66.7 L61.1 61.1 L58.3 58.3 L56.7 52.8 L55.6 50 L56.7 47.2 Z" />
        <!-- Europe -->
        <path d="M95 29.4 L95 26.1 L99.4 24.4 L97.4 23.1 L101.1 21.7 L103.9 20 L105.6 18.1 L102.8 16.7 L105.6 14.4 L110 11.7 L116.7 11.1 L116.7 16.7 L115 20.6 L113.9 22.2 L115.6 25 L117.8 25.6 L116.7 27.8 L115 27.8 L112.8 29.4 L111.1 28.3 L109.4 27.8 L108.3 26.7 L106.7 25.6 L104.4 25.6 L101.7 26.1 L100 28.9 L97.2 30 Z" />
        <!-- Africa -->
        <path d="M96.7 30.6 L105.6 29.4 L111.1 32.2 L116.7 32.8 L119.4 34.4 L121.1 37.8 L123.9 43.3 L128.3 43.9 L126.7 47.2 L125.6 48.9 L122.8 51.1 L122.2 55.6 L120 61.1 L119.4 65 L117.8 66.7 L115.6 68.3 L111.1 69.4 L110 68.3 L108.3 65 L106.7 60 L107.2 56.7 L105 50.6 L105 47.8 L102.2 46.7 L95.6 47.2 L91.7 43.9 L90.6 41.7 L91.1 38.9 L94.4 32.2 Z" />
        <!-- Asia -->
        <path d="M115 27.8 L116.7 26.7 L122.2 25.6 L127.8 25 L133.3 23.3 L141.7 21.1 L147.2 19.4 L152.8 16.7 L158.3 12.2 L163.9 10 L172.2 10.6 L183.3 11.1 L194.4 12.2 L198.9 13.9 L200 15.6 L190 17.8 L187.2 21.7 L178.9 21.7 L178.3 25 L175.6 26.1 L170.6 27.8 L167.8 28.9 L167.2 32.8 L166.7 35.6 L163.9 37.2 L161.1 38.9 L158.9 44.4 L155.6 42.8 L154.4 41.1 L151.7 37.8 L148.3 38.3 L144.4 41.7 L142.8 45.6 L140.6 40.6 L137.8 37.2 L134.4 36.1 L131.7 35 L126.7 33.3 L128.9 36.1 L132.2 36.7 L131.1 38.9 L125 42.8 L123.9 43.3 L122.2 38.9 L119.4 36.1 L118.9 34.4 L118.9 32.8 L120 30 L116.7 29.7 Z" />
        <!-- Australia -->
        <path d="M162.2 62.2 L167.8 59.4 L173.3 56.7 L177.8 56.1 L181.1 60.6 L183.3 63.3 L185 65 L183.9 68.9 L181.7 71.1 L177.8 71.1 L175 69.4 L172.2 67.8 L168.9 68.3 L165.6 69.4 L163.9 68.3 L162.8 63.9 Z" />
        <!-- UK / Ireland / Japan / Indonesia / New Guinea / NZ / Madagascar -->
        <path d="M97.2 22.2 L98.9 18.3 L100.6 21.1 L100 22 Z" />
        <path d="M95.6 20.6 L96.7 19.4 L97 21 Z" />
        <path d="M177.8 30 L178.3 27.8 L178.9 25.6 L179.4 26.7 L178.3 31.1 Z" />
        <path d="M152.8 47.2 L155.6 50 L158.9 53.3 L162.2 53.9 L166.1 55 L165 49.4 L162.8 47.8 L161.1 50 L160 51.7 Z" />
        <path d="M172.2 50.6 L176.7 51.7 L181.7 53.9 L181.1 55 L176.7 54.4 L172.2 51.7 Z" />
        <path d="M196.7 70.6 L198.9 71.1 L196.7 73.3 Z" />
        <path d="M126.1 58.9 L127.8 63.9 L124.4 63.9 L124.4 60 Z" />
        </g>
      </svg>
      {#await pinsPromise}
        <div class="absolute inset-0 grid place-items-center text-sm text-[var(--color-muted)]">Loading map…</div>
      {:then pins}
        {#each pins as pin}
          {@const { isVisitor } = pinFlags(pin)}
          <div class="absolute flex flex-col items-center" style="top: {pin.y}%; inset-inline-start: {pin.x}%; transform: translate(-50%, -50%)">
            {#if pin.href}
              <a href={pin.href} class="flex flex-col items-center">
                <div class="rounded-full {isVisitor ? 'h-5 w-5 ring-4 ring-[var(--color-primary)] ring-opacity-30' : 'h-3 w-3'} bg-[var(--color-primary)] animate-pulse"></div>
                <span class="mt-1 whitespace-nowrap text-xs text-[var(--color-text)] bg-[var(--color-surface)] px-1.5 py-0.5 rounded shadow-sm">{pin.label}</span>
              </a>
            {:else}
              <div class="rounded-full {isVisitor ? 'h-5 w-5 ring-4 ring-[var(--color-primary)] ring-opacity-30' : 'h-3 w-3'} bg-[var(--color-primary)] animate-pulse"></div>
              <span class="mt-1 whitespace-nowrap text-xs text-[var(--color-text)] bg-[var(--color-surface)] px-1.5 py-0.5 rounded shadow-sm">{pin.label}</span>
            {/if}
          </div>
        {/each}
      {/await}
    </div>
  </div>
</section>
