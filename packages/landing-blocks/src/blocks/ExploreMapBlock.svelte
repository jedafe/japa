<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  import type { ExploreMapProps } from '../types';
  import type { FetchContext } from '../data/types';
  import { countryCentroid } from '../geo/countryCentroids';
  import { WORLD_COUNTRIES, WORLD_VIEWBOX, REGION_BOUNDS } from '../geo/worldMapData';
  import { EXPLORE_MAP_DEFAULTS } from './exploreMapDefaults';
  import { buildApiUrl } from '@japaconnect/api-url';
  import * as LucideIcons from 'lucide-svelte';
  import type { Component } from 'svelte';

  interface Props {
    props: ExploreMapProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props, fetchCtx, geoCountry, id }: Props = $props();

  const P = $derived({ ...EXPLORE_MAP_DEFAULTS, ...props });
  const VB_W = WORLD_VIEWBOX.w;
  const VB_H = WORLD_VIEWBOX.h;

  const countryByCode = new Map(WORLD_COUNTRIES.map((c) => [c.code, c]));
  const nameToCode = new Map(WORLD_COUNTRIES.map((c) => [c.name.toLowerCase(), c.code]));
  const FALLBACK_FILL = '#ccdde8';
  const CARD_W = 150;
  const CARD_H = 60;

  interface GeoSpaceLite {
    id: string; name: string; slug: string; level: string;
    geo_code?: string; description?: string; currency_code?: string | null;
    member_count?: number; follower_count?: number;
  }
  interface FxCurrency { code: string; name: string; symbol: string }
  interface Pin {
    country: string; label: string; tagline?: string; category?: string; href?: string;
    x: number; y: number; color: string; flag: string;
  }

  // ── Remote data ─────────────────────────────────────────────
  let apiCountries = $state<GeoSpaceLite[]>([]);
  let apiRegions = $state<string[]>([]);
  let currencies = $state<FxCurrency[]>([]);
  let fxRates = $state<Record<string, number>>({});
  let statCounts = $state<Record<string, number>>({});
  let viewPins = $state<Pin[]>([]);
  let loading = $state(true);

  // ── UI state ────────────────────────────────────────────────
  let selRegion = $state('');
  let vbFit = $state(false);
  let selCategory = $state('');
  let selView = $state('');
  let currency = $state('');
  let vb = $state<[number, number, number, number]>([0, 0, VB_W, VB_H]);
  let hoverIdx = $state(-1);
  let svgEl = $state<SVGSVGElement | null>(null);

  // Initialize selections from props once (don't clobber on later edits).
  $effect(() => {
    if (!selView) selView = P.views?.[0]?.key ?? '';
    if (!currency) currency = P.default_currency ?? 'USD';
    // initial_bounds takes precedence over initial_region; vbFit ensures the
    // fit runs once even though this effect re-fires when P changes.
    if (!selRegion && !vbFit && P.initial_bounds) {
      vbFit = true;
      fitBounds(P.initial_bounds);
    } else if (!selRegion && !vbFit && P.initial_region && REGION_BOUNDS[P.initial_region]) {
      vbFit = true;
      selectRegion(P.initial_region);
    }
  });

  function headers(): Record<string, string> {
    const h: Record<string, string> = {};
    if (!fetchCtx) return h;
    if (fetchCtx.token) h.authorization = `Bearer ${fetchCtx.token}`;
    if (fetchCtx.host) { h.host = fetchCtx.host; h['x-forwarded-host'] = fetchCtx.host; }
    if (fetchCtx.surface) h['x-jc-surface'] = fetchCtx.surface;
    return h;
  }

  async function api<T>(path: string): Promise<T | null> {
    if (!fetchCtx) return null;
    try {
      const res = await fetchCtx.fetch(buildApiUrl(path, fetchCtx.apiBase), { headers: headers() });
      if (!res.ok) return null;
      return (await res.json()) as T;
    } catch {
      return null;
    }
  }

  function flag(code: string): string {
    if (!code || code.length !== 2) return '';
    return String.fromCodePoint(...[...code.toUpperCase()].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
  }

  function centroid(code: string): [number, number] | null {
    const c = countryCentroid(code);
    if (!c) return null;
    return [((c[1] + 180) / 360) * VB_W, ((90 - c[0]) / 180) * VB_H];
  }

  function bandColorFor(region: string, subregion: string): string {
    const key = P.region_bands?.[subregion] ?? P.region_bands?.[region];
    return P.col_bands?.find((b) => b.key === key)?.color ?? FALLBACK_FILL;
  }

  function categoryColor(key?: string): string | null {
    return P.categories?.find((c) => c.key === key)?.color ?? null;
  }

  function firstSentence(s?: string): string | undefined {
    if (!s) return undefined;
    const cut = s.split(/[.!?]/)[0]?.trim();
    if (!cut) return undefined;
    return cut.length > 64 ? cut.slice(0, 61).trimEnd() + '…' : cut;
  }

  function toPin(country: string, label: string, tagline?: string, category?: string, href?: string, pos?: [number, number]): Pin | null {
    const code = country.toUpperCase();
    const c = pos ?? centroid(code);
    if (!c) return null;
    const meta = countryByCode.get(code);
    return {
      country: code, label, tagline, category, href,
      x: c[0], y: c[1],
      color: categoryColor(category) ?? bandColorFor(meta?.region ?? '', meta?.subregion ?? ''),
      flag: flag(code),
    };
  }

  // ── Data loading ────────────────────────────────────────────
  // Mount fetch: countries, regions, stats. Reads no block props so it only
  // re-runs if fetchCtx changes — not on every inspector keystroke.
  $effect(() => {
    loading = true;
    void (async () => {
      const [countries, regions] = await Promise.all([
        api<{ data: GeoSpaceLite[] }>('/geo/countries'),
        api<{ data: string[] }>('/spaces/regions'),
      ]);
      apiCountries = (countries?.data ?? []).filter((c) => c.geo_code);
      apiRegions = regions?.data ?? [];

      // Stat totals — independent best-effort fetches.
      const counts: Record<string, number> = { countries: apiCountries.length };
      const [col, blog, svc] = await Promise.all([
        api<{ data: { total: number } }>('/calculator/cost-of-living/cities?page_size=1'),
        api<{ meta: { total: number } }>('/blog?page_size=1'),
        api<{ meta: { total: number } }>('/services?limit=1'),
      ]);
      if (col?.data?.total != null) counts.col_cities = col.data.total;
      if (blog?.meta?.total != null) counts.blog = blog.meta.total;
      if (svc?.meta?.total != null) counts.services = svc.meta.total;
      statCounts = counts;
      loading = false;
    })();
  });

  // FX data — keyed only on the configured base currency.
  $effect(() => {
    const base = P.default_currency ?? 'USD';
    void (async () => {
      const [fxCur, fx] = await Promise.all([
        api<{ data: FxCurrency[] }>('/fx/currencies'),
        api<{ data: { rates: Record<string, number> } }>(`/fx/rates?base=${encodeURIComponent(base)}`),
      ]);
      currencies = fxCur?.data ?? [];
      fxRates = fx?.data?.rates ?? {};
    })();
  });

  // View-specific pin sets (universities / COL cities), fetched on demand.
  $effect(() => {
    const view = P.views?.find((v) => v.key === selView);
    if (!view || view.source === 'countries') { viewPins = []; return; }
    void (async () => {
      if (view.source === 'universities') {
        const json = await api<{ data: Array<{ country?: string; country_code?: string }> }>('/universities?page_size=100');
        const byCountry = new Map<string, number>();
        for (const u of json?.data ?? []) {
          const code = (u.country_code ?? nameToCode.get((u.country ?? '').toLowerCase()) ?? '').toUpperCase();
          if (code) byCountry.set(code, (byCountry.get(code) ?? 0) + 1);
        }
        viewPins = [...byCountry.entries()]
          .map(([code, n]) => toPin(code, countryByCode.get(code)?.name ?? code, `${n} ${n === 1 ? 'university' : 'universities'}`, 'study', '/universities'))
          .filter((p): p is Pin => p !== null);
      } else {
        const json = await api<{ data: { cities: Array<{ description?: string }> } }>('/calculator/cost-of-living/cities?page_size=60');
        const byCountry = new Map<string, number>();
        for (const c of json?.data?.cities ?? []) {
          // Descriptions are seeded as "City, Country — …" — the tail resolves the country.
          const name = (c.description ?? '').split(/[—–-]/)[0]?.split(',').pop()?.trim().toLowerCase() ?? '';
          const code = nameToCode.get(name);
          if (code) byCountry.set(code, (byCountry.get(code) ?? 0) + 1);
        }
        viewPins = [...byCountry.entries()]
          .map(([code, n]) => toPin(code, countryByCode.get(code)?.name ?? code, `${n} ${n === 1 ? 'city' : 'cities'} tracked`, 'affordable', '/tools/cost-of-living'))
          .filter((p): p is Pin => p !== null);
      }
    })();
  });

  // ── Pins ────────────────────────────────────────────────────
  const basePins = $derived.by<Pin[]>(() => {
    const view = P.views?.find((v) => v.key === selView);
    if (view && view.source !== 'countries') return viewPins;
    if (P.geo_source === 'manual' || apiCountries.length === 0) {
      return (P.pins ?? [])
        .map((p) =>
          toPin(
            p.country,
            p.label ?? p.country,
            p.tagline,
            p.category,
            p.href,
            p.x != null && p.y != null ? [p.x, p.y] : undefined,
          ),
        )
        .filter((p): p is Pin => p !== null);
    }
    return [...apiCountries]
      .sort((a, b) => (b.member_count ?? 0) + (b.follower_count ?? 0) - ((a.member_count ?? 0) + (a.follower_count ?? 0)))
      .slice(0, P.pin_limit ?? 8)
      .map((s) => toPin(s.geo_code ?? '', s.name, firstSentence(s.description), undefined, `/spaces/${s.slug}`))
      .filter((p): p is Pin => p !== null);
  });

  const visiblePins = $derived.by<Pin[]>(() =>
    basePins.filter((p) => {
      if (selRegion && countryByCode.get(p.country)?.region !== selRegion) return false;
      if (selCategory && p.category !== selCategory) return false;
      return true;
    })
  );

  // ── Regions ─────────────────────────────────────────────────
  const regionOptions = $derived.by<string[]>(() => {
    const known = apiRegions.filter((r) => REGION_BOUNDS[r]);
    return known.length ? known : Object.keys(REGION_BOUNDS);
  });

  // ── Zoom / pan ──────────────────────────────────────────────
  function clampVb(v: [number, number, number, number]): [number, number, number, number] {
    const w = Math.min(Math.max(v[2], 80), VB_W);
    const h = Math.min(Math.max(v[3], 40), VB_H);
    // Re-center on the requested midpoint — otherwise small bounds that hit
    // the min-size clamp (city-level fits) end up pinned to the top-left.
    const cx = v[0] + v[2] / 2;
    const cy = v[1] + v[3] / 2;
    return [
      Math.min(Math.max(cx - w / 2, 0), VB_W - w),
      Math.min(Math.max(cy - h / 2, 0), VB_H - h),
      w, h,
    ];
  }

  const reducedMotion = typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
  let tweenId = 0;
  function setVb(target: [number, number, number, number]) {
    const t = clampVb(target);
    if (reducedMotion) { vb = t; return; }
    const id = ++tweenId;
    const from = [...vb] as [number, number, number, number];
    const start = performance.now();
    const dur = 260;
    const step = (now: number) => {
      if (id !== tweenId) return;
      const k = Math.min(1, (now - start) / dur);
      const e = 1 - Math.pow(1 - k, 3);
      vb = [from[0] + (t[0] - from[0]) * e, from[1] + (t[1] - from[1]) * e, from[2] + (t[2] - from[2]) * e, from[3] + (t[3] - from[3]) * e];
      if (k < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }

  function zoomBy(f: number) {
    const [x, y, w, h] = vb;
    const nw = w * f;
    const nh = h * f;
    setVb([x + (w - nw) / 2, y + (h - nh) / 2, nw, nh]);
  }

  function fitBounds(b: [number, number, number, number]) {
    const [x0, y0, x1, y1] = b;
    let bw = x1 - x0;
    let bh = y1 - y0;
    if (bw / bh > 2) bh = bw / 2; else bw = bh * 2;
    setVb([x0 + (x1 - x0 - bw) / 2, y0 + (y1 - y0 - bh) / 2, bw, bh]);
  }

  function fitRegion(region: string) {
    const b = REGION_BOUNDS[region];
    if (!b) { setVb([0, 0, VB_W, VB_H]); return; }
    fitBounds(b);
  }

  function selectRegion(name: string) {
    selRegion = selRegion === name ? '' : name;
    if (selRegion) fitRegion(selRegion); else setVb([0, 0, VB_W, VB_H]);
  }

  function resetView() {
    selRegion = '';
    setVb([0, 0, VB_W, VB_H]);
  }

  // Pointer drag → pan; pointer proximity → pin hover (screen-space, so the
  // hit radius stays a constant ~14px at any zoom).
  let panning = $state<{ px: number; py: number; vb: [number, number, number, number]; moved: boolean } | null>(null);
  let justDragged = false;
  function onDown(e: PointerEvent) {
    panning = { px: e.clientX, py: e.clientY, vb: [...vb] as [number, number, number, number], moved: false };
    (e.currentTarget as Element).setPointerCapture(e.pointerId);
    // Touch taps never fire a pre-click pointermove — surface the pin card on down.
    if (svgEl) hoverAt(e, svgEl.getBoundingClientRect());
  }
  /** Screen-space position of a pin under preserveAspectRatio — "meet"
   * letterboxes (min scale), cover_mode "slice" crops (max scale). */
  function pinScreen(p: Pin, rect: DOMRect): { x: number; y: number } {
    const s = P.cover_mode
      ? Math.max(rect.width / vb[2], rect.height / vb[3])
      : Math.min(rect.width / vb[2], rect.height / vb[3]);
    const offX = (rect.width - vb[2] * s) / 2;
    const offY = (rect.height - vb[3] * s) / 2;
    return { x: (p.x - vb[0]) * s + offX + rect.left, y: (p.y - vb[1]) * s + offY + rect.top };
  }

  function hoverAt(e: PointerEvent, rect: DOMRect) {
    let best = -1;
    let bestD = 18; // px hit radius
    for (let i = 0; i < visiblePins.length; i++) {
      const pt = pinScreen(visiblePins[i], rect);
      const d = Math.hypot(pt.x - e.clientX, pt.y - e.clientY);
      if (d < bestD) { bestD = d; best = i; }
    }
    hoverIdx = best;
  }

  function onMove(e: PointerEvent) {
    if (!svgEl) return;
    const rect = svgEl.getBoundingClientRect();
    if (panning) {
      const scale = panning.vb[2] / rect.width;
      if (Math.abs(e.clientX - panning.px) + Math.abs(e.clientY - panning.py) > 4) panning.moved = true;
      if (panning.moved) {
        tweenId++; // cancel any in-flight zoom tween while dragging
        vb = clampVb([panning.vb[0] - (e.clientX - panning.px) * scale, panning.vb[1] - (e.clientY - panning.py) * scale, panning.vb[2], panning.vb[3]]);
        return;
      }
    }
    hoverAt(e, rect);
  }
  function onUp() {
    if (panning?.moved) {
      justDragged = true;
      setTimeout(() => (justDragged = false), 60);
    }
    panning = null;
  }
  function onLeave() { panning = null; hoverIdx = -1; }
  // A drag shouldn't fire pin-link clicks on release.
  function onClickCapture(e: MouseEvent) {
    if (justDragged) {
      justDragged = false;
      e.preventDefault();
      e.stopPropagation();
    }
  }

  // ── Currency ────────────────────────────────────────────────
  const rate = $derived(fxRates[currency] ?? 1);
  const symbol = $derived(currencies.find((c) => c.code === currency)?.symbol ?? (currency === 'USD' ? '$' : ''));

  function fmtMoney(n: number): string {
    const v = n * rate;
    return v >= 10000
      ? new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 }).format(v)
      : new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(v);
  }
  function bandRange(min: number, max: number | null): string {
    return max === null ? `${symbol}${fmtMoney(min)}+` : `${symbol}${fmtMoney(min)} – ${symbol}${fmtMoney(max)}`;
  }

  function statValue(source: string, value?: string, suffix?: string): string {
    const n = statCounts[source];
    if (n != null) return `${n.toLocaleString()}${suffix ?? ''}`;
    return value ?? '—';
  }

  function iconFor(name?: string): Component | null {
    if (!name) return null;
    return ((LucideIcons as Record<string, unknown>)[name] as Component) ?? null;
  }

  const visitorCode = $derived(geoCountry?.toUpperCase() ?? '');
</script>

<section class="explore-map {P.cover_mode ? '' : 'px-4 py-10 sm:px-6'}" id={id}>
  <div class="mx-auto max-w-7xl">
    <div class="flex flex-col gap-5 {P.aside_position === 'bottom' ? '' : 'lg:flex-row'}">
      <!-- ══ Panel — beside the map ('left') or stacked below ('bottom') ══ -->
      {#if (P.show_legend && (P.categories?.length ?? 0) > 0) || P.show_region_filter || (P.aside_position !== 'bottom' && P.show_stats && (P.stats?.length ?? 0) > 0) || (P.aside_position === 'bottom' && P.show_col_index && (P.col_bands?.length ?? 0) > 0)}
      <aside class="order-2 w-full shrink-0 {P.aside_position === 'bottom' ? 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3' : 'flex flex-col gap-4 lg:order-1 lg:w-60'}"> <!-- bottom mode: below map+stats -->
        {#if P.show_legend && (P.categories?.length ?? 0) > 0}
          <div class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-sm">
            <h3 class="mb-3 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Map legend</h3>
            <ul class="space-y-2.5">
              {#each P.categories ?? [] as cat (cat.key)}
                {@const Icon = iconFor(cat.icon)}
                <li>
                  <button
                    type="button"
                    class="flex w-full items-center gap-2.5 rounded-[var(--radius)] px-1.5 py-1 text-left transition-colors hover:bg-[var(--color-surface-subtle)] {selCategory === cat.key ? 'bg-[var(--color-surface-subtle)] ring-1 ring-[var(--color-border)]' : ''}"
                    onclick={() => (selCategory = selCategory === cat.key ? '' : cat.key)}
                    aria-pressed={selCategory === cat.key}
                  >
                    <span class="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-white" style="background:{cat.color}">
                      {#if Icon}<Icon size={15} />{/if}
                    </span>
                    <span class="min-w-0">
                      <span class="block truncate text-xs font-semibold text-[var(--color-text)]">{cat.label}</span>
                      {#if cat.sublabel}<span class="block truncate text-[11px] text-[var(--color-muted)]">{cat.sublabel}</span>{/if}
                    </span>
                  </button>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if P.show_region_filter}
          <div class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-sm">
            <h3 class="mb-2.5 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Region filter</h3>
            <ul class="space-y-1">
              <li>
                <button
                  type="button"
                  class="flex w-full items-center gap-2 rounded-[var(--radius)] px-2 py-1.5 text-sm transition-colors {selRegion === '' ? 'bg-[var(--color-primary)]/10 font-medium text-[var(--color-primary)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
                  onclick={resetView}
                >
                  <LucideIcons.Globe size={14} /> All Regions
                </button>
              </li>
              {#each regionOptions as region (region)}
                <li>
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 rounded-[var(--radius)] px-2 py-1.5 text-sm transition-colors {selRegion === region ? 'bg-[var(--color-primary)]/10 font-medium text-[var(--color-primary)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'}"
                    onclick={() => selectRegion(region)}
                    aria-pressed={selRegion === region}
                  >
                    <LucideIcons.MapPin size={14} class="opacity-60" /> {region}
                  </button>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        {#if P.aside_position !== 'bottom' && P.show_stats && (P.stats?.length ?? 0) > 0}
          <div class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-sm">
            <h3 class="mb-3 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Quick stats</h3>
            <ul class="space-y-3">
              {#each P.stats ?? [] as stat (stat.label)}
                {@const Icon = iconFor(stat.icon)}
                <li class="flex items-center gap-2.5">
                  <span class="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
                    {#if Icon}<Icon size={15} />{/if}
                  </span>
                  <span class="min-w-0">
                    <span class="block text-sm font-bold leading-tight text-[var(--color-text)]">{statValue(stat.source, stat.value, stat.suffix)}</span>
                    <span class="block truncate text-[11px] text-[var(--color-muted)]">{stat.label}</span>
                  </span>
                </li>
              {/each}
            </ul>
          </div>
        {/if}

        <!-- COL index — in the aside row only when the aside is bottomed -->
        {#if P.aside_position === 'bottom' && P.show_col_index && (P.col_bands?.length ?? 0) > 0}
          <div class="rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-sm">
            <h3 class="mb-3 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Cost of living index (Monthly)</h3>
            <ul class="space-y-2.5">
              {#each P.col_bands ?? [] as band (band.key)}
                <li class="flex items-center gap-2.5 rounded-[var(--radius)] px-1.5 py-1 transition-colors hover:bg-[var(--color-surface-subtle)]">
                  <span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background:{band.color}"></span>
                  <span class="text-xs font-semibold text-[var(--color-text)]">{bandRange(band.min, band.max)}</span>
                  <span class="truncate text-xs text-[var(--color-muted)]">{band.label}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      </aside>
      {/if}

      <!-- ══ Main column (map first on mobile/tablet) ════════ -->
      <div class="order-1 min-w-0 flex-1 {P.aside_position === 'bottom' ? '' : 'lg:order-2'}">
        <div class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0">
            {#if P.headline}<h2 class="text-2xl font-bold tracking-tight text-[var(--color-text)] sm:text-[28px]">{P.headline}</h2>{/if}
            {#if P.subtitle}<p class="mt-1 text-sm text-[var(--color-muted)]">{P.subtitle}</p>{/if}
          </div>
          {#if P.show_toolbar}
            <div class="flex shrink-0 divide-x divide-[var(--color-border)] overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-sm">
              <label class="flex items-center gap-2 px-3 py-2">
                <LucideIcons.MapPin size={15} class="text-[var(--color-primary)]" />
                <span class="flex flex-col">
                  <span class="text-[10px] leading-tight text-[var(--color-muted)]">View</span>
                  <span class="em-select-wrap"><select
                    bind:value={selView}
                    class="em-select"
                  >
                    {#each P.views ?? [] as v (v.key)}<option value={v.key}>{v.label}</option>{/each}
                  </select><LucideIcons.ChevronDown size={11} class="em-chevron" /></span>
                </span>
              </label>
              <label class="flex items-center gap-2 px-3 py-2">
                <LucideIcons.LayoutGrid size={15} class="text-[var(--color-primary)]" />
                <span class="flex flex-col">
                  <span class="text-[10px] leading-tight text-[var(--color-muted)]">Show</span>
                  <span class="em-select-wrap"><select
                    bind:value={selCategory}
                    class="em-select"
                  >
                    <option value="">All Categories</option>
                    {#each P.categories ?? [] as c (c.key)}<option value={c.key}>{c.label}</option>{/each}
                  </select><LucideIcons.ChevronDown size={11} class="em-chevron" /></span>
                </span>
              </label>
              {#if currencies.length > 0}
                <label class="flex items-center gap-2 px-3 py-2">
                  <LucideIcons.CircleDollarSign size={15} class="text-[var(--color-primary)]" />
                  <span class="flex flex-col">
                    <span class="text-[10px] leading-tight text-[var(--color-muted)]">Currency</span>
                    <span class="em-select-wrap"><select
                      bind:value={currency}
                      class="em-select"
                    >
                      {#each currencies as c (c.code)}<option value={c.code}>{c.code}</option>{/each}
                    </select><LucideIcons.ChevronDown size={11} class="em-chevron" /></span>
                  </span>
                </label>
              {/if}
            </div>
          {/if}
        </div>

        <!-- Map card -->
        <div class="relative overflow-hidden rounded-2xl border border-[var(--color-border)] shadow-sm" style="background: linear-gradient(180deg, #e8f3fb 0%, #dcedf7 100%);">
          {#if loading}
            <div class="absolute inset-0 z-10 grid place-items-center">
              <span class="text-sm text-[var(--color-muted)]">Loading map…</span>
            </div>
          {/if}
          <svg
            bind:this={svgEl}
            viewBox="{vb[0]} {vb[1]} {vb[2]} {vb[3]}"
            preserveAspectRatio={P.cover_mode ? 'xMidYMid slice' : 'xMidYMid meet'}
            class="block w-full select-none {P.cover_mode ? 'h-64 sm:h-72' : 'aspect-[2/1]'} {panning ? 'cursor-grabbing' : 'cursor-grab'}"
            style="overflow: visible"
            role="img"
            aria-label="Interactive world map of destinations"
            onpointerdown={onDown}
            onpointermove={onMove}
            onpointerup={onUp}
            onpointercancel={onUp}
            onpointerleave={onLeave}
            onclickcapture={onClickCapture}
          >
            <rect x={vb[0]} y={vb[1]} width={vb[2]} height={vb[3]} fill="transparent" />
            {#each WORLD_COUNTRIES as c (c.code)}
              {@const inRegion = !selRegion || c.region === selRegion}
              {@const isVisitor = visitorCode !== '' && c.code === visitorCode}
              <path
                d={c.d}
                fill={bandColorFor(c.region, c.subregion)}
                fill-opacity={isVisitor ? 1 : inRegion ? (P.focus_d ? 0.4 : 0.85) : 0.18}
                stroke={isVisitor ? 'var(--color-primary)' : '#ffffff'}
                stroke-opacity={inRegion ? 0.9 : 0.35}
                stroke-width={isVisitor ? 3 * (vb[2] / VB_W) : 1.2 * (vb[2] / VB_W)}
              />
            {/each}

            {#if P.container_d}
              <!-- Container outline: the parent unit's boundary drawn bold
                   around the focus shape (state around a city breakout). -->
              <path
                d={P.container_d}
                fill="none"
                stroke="var(--color-primary)"
                stroke-opacity={0.9}
                stroke-width={2.5 * (vb[2] / VB_W)}
              />
            {/if}
            {#if P.focus_d}
              <!-- Breakout shape: the focused unit drawn over the
                   country context (state/city outline inside its parent). -->
              <path
                d={P.focus_d}
                fill="var(--color-primary)"
                fill-opacity={0.7}
                stroke="#ffffff"
                stroke-opacity={0.9}
                stroke-width={1.5 * (vb[2] / VB_W)}
              />
            {/if}

            {#each visiblePins as p, i (p.country + i)}
              <g transform="translate({p.x} {p.y})">
                {#if p.href}
                  <a href={p.href} aria-label={p.label} class="cursor-pointer">
                    {@render marker(p)}
                  </a>
                {:else}
                  {@render marker(p)}
                {/if}
              </g>
            {/each}
            <!-- Cards render in a top pass: all taglined pins' cards above the
                 markers, then the hovered pin's card last (highest z-order). -->
            {#each visiblePins.filter((p) => p.tagline) as p (p.country)}
              {@render pinCard(p)}
            {/each}
            {#if hoverIdx >= 0 && visiblePins[hoverIdx] && !visiblePins[hoverIdx].tagline}
              {@render pinCard(visiblePins[hoverIdx])}
            {/if}
          </svg>

          <!-- Zoom controls + compass -->
          {#if P.show_zoom}
            <div class="absolute bottom-4 right-4 flex flex-col items-end gap-2">
              <div class="flex flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-sm">
                <button type="button" aria-label="Zoom in" class="grid h-9 w-9 place-items-center text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]" onclick={() => zoomBy(0.7)}>
                  <LucideIcons.Plus size={15} />
                </button>
                <div class="h-px bg-[var(--color-border)]"></div>
                <button type="button" aria-label="Zoom out" class="grid h-9 w-9 place-items-center text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]" onclick={() => zoomBy(1.45)}>
                  <LucideIcons.Minus size={15} />
                </button>
                <div class="h-px bg-[var(--color-border)]"></div>
                <button type="button" aria-label="Reset view" class="grid h-9 w-9 place-items-center text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]" onclick={resetView}>
                  <LucideIcons.LocateFixed size={15} />
                </button>
              </div>
              <button type="button" aria-label="Reset north" class="grid h-11 w-11 place-items-center" onclick={resetView}>
                <svg viewBox="0 0 48 48" class="h-10 w-10 drop-shadow-sm">
                  <circle cx="24" cy="24" r="22" fill="var(--color-surface)" stroke="var(--color-border)" />
                  <text x="24" y="11" text-anchor="middle" font-size="9" font-weight="700" fill="var(--color-text)">N</text>
                  <path d="M24 13 L28 27 L24 24 L20 27 Z" fill="var(--color-primary)" />
                  <path d="M24 35 L20 21 L24 24 L28 21 Z" fill="var(--color-muted)" opacity="0.45" />
                </svg>
              </button>
            </div>
          {/if}

          <!-- Quick stats — floating card at the map's bottom-left in
               bottom mode (mirrors the zoom controls on the right). -->
          {#if P.aside_position === 'bottom' && P.show_stats && (P.stats?.length ?? 0) > 0}
            <div class="absolute bottom-4 left-4 w-56 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]/90 p-3 shadow-sm backdrop-blur-sm">
              <h3 class="mb-2.5 text-[11px] font-semibold tracking-wide text-[var(--color-muted)]">Quick stats</h3>
              <ul class="space-y-2.5">
                {#each P.stats ?? [] as stat (stat.label)}
                  {@const Icon = iconFor(stat.icon)}
                  <li class="flex items-center gap-2.5">
                    <span class="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
                      {#if Icon}<Icon size={14} />{/if}
                    </span>
                    <span class="min-w-0">
                      <span class="block text-sm font-bold leading-tight text-[var(--color-text)]">{statValue(stat.source, stat.value, stat.suffix)}</span>
                      <span class="block truncate text-[11px] text-[var(--color-muted)]">{stat.label}</span>
                    </span>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
        </div>

        <!-- COL index bar (default 'left' layout — under the map) -->
        {#if P.aside_position !== 'bottom' && P.show_col_index && (P.col_bands?.length ?? 0) > 0}
          <div class="mt-4 rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] px-5 py-4 shadow-sm">
            <h3 class="mb-3 text-xs font-semibold tracking-wide text-[var(--color-muted)]">Cost of living index (Monthly)</h3>
            <ul class="flex flex-wrap items-center gap-x-8 gap-y-2">
              {#each P.col_bands ?? [] as band (band.key)}
                <li class="flex items-center gap-2">
                  <span class="h-2.5 w-2.5 rounded-full" style="background:{band.color}"></span>
                  <span class="text-xs font-semibold text-[var(--color-text)]">{bandRange(band.min, band.max)}</span>
                  <span class="text-xs text-[var(--color-muted)]">{band.label}</span>
                </li>
              {/each}
            </ul>
          </div>
        {/if}
      </div>
    </div>
  </div>
</section>

{#snippet marker(p: Pin)}
  {@const zf = vb[2] / VB_W}
  <!-- pin marker — scales WITH zoom so it stays a constant screen size -->
  <g transform="scale({zf * 1.2})">
    <path
      d="M0 0 C-4.5 -7 -6 -10 -6 -13 A6 6 0 1 1 6 -13 C6 -10 4.5 -7 0 0 Z"
      fill={p.color}
      stroke="#ffffff"
      stroke-width="1.4"
    />
    <circle cy="-13" r="2.2" fill="#ffffff" />
  </g>
{/snippet}

{#snippet pinCard(p: Pin)}
  {@const zf = vb[2] / VB_W}
  {@const flip = p.y - (CARD_H + 18) * zf < vb[1]}
  <g transform="translate({p.x} {p.y}) scale({zf})">
    <foreignObject x={-CARD_W / 2} y={flip ? 14 : -CARD_H - 14} width={CARD_W} height={CARD_H} style="overflow: visible">
      <div class="em-card">
        <div class="em-card-title">{#if p.flag}<span>{p.flag}</span>{/if}<span class="truncate">{p.label}</span></div>
        {#if p.tagline}<div class="em-card-tag">{p.tagline}</div>{/if}
      </div>
    </foreignObject>
  </g>
{/snippet}

<style>
  .em-select-wrap {
    position: relative;
    display: inline-flex;
    align-items: center;
  }
  .em-select {
    appearance: none;
    -webkit-appearance: none;
    margin: -2px;
    padding: 2px 16px 2px 2px;
    background: transparent;
    border: none;
    color: var(--color-text);
    font-size: 0.75rem;
    font-weight: 600;
    line-height: 1.2;
    cursor: pointer;
    outline: none;
  }
  .em-select option {
    background-color: var(--color-surface);
    color: var(--color-text);
  }
  :global([data-theme="dark"]) .em-select {
    color-scheme: dark;
  }
  .em-select-wrap :global(.em-chevron) {
    position: absolute;
    right: 0;
    pointer-events: none;
    color: var(--color-muted);
  }
  .em-card {
    display: inline-block;
    width: 100%;
    border-radius: 10px;
    border: 1px solid var(--color-border);
    background: var(--color-surface);
    padding: 7px 10px;
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.12);
    pointer-events: none;
  }
  .em-card-title {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 11px;
    font-weight: 700;
    color: var(--color-text);
    line-height: 1.2;
  }
  .em-card-tag {
    margin-top: 2px;
    font-size: 10px;
    line-height: 1.25;
    color: var(--color-muted);
  }
</style>
