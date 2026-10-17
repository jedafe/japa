<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  import type {
    ExploreMapProps, ExploreMapView, ExploreMapCategory,
    ExploreMapStat, ExploreMapPin, ExploreMapBand, GeoContext,
  } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { EXPLORE_MAP_DEFAULTS } from '../../blocks/exploreMapDefaults';
  import { WORLD_COUNTRIES } from '../../geo/worldMapData';
  import type { Component } from 'svelte';

  interface Props {
    props: ExploreMapProps;
    onchange: (v: ExploreMapProps) => void;
    mediaPickerComponent?: Component;
    geoCtx?: GeoContext;
    fetchCtx?: FetchContext;
  }
  let { props, onchange, mediaPickerComponent, geoCtx, fetchCtx }: Props = $props();

  const P = $derived<ExploreMapProps>({ ...EXPLORE_MAP_DEFAULTS, ...props });

  function update<K extends keyof ExploreMapProps>(field: K, v: ExploreMapProps[K]) {
    onchange({ ...P, [field]: v });
  }

  function listUpdate<T extends object>(field: keyof ExploreMapProps, list: T[], idx: number, patch: Partial<T>) {
    update(field, list.map((it, i) => (i === idx ? { ...it, ...patch } : it)) as never);
  }
  function listRemove<T>(field: keyof ExploreMapProps, list: T[], idx: number) {
    update(field, list.filter((_, i) => i !== idx) as never);
  }
  function listAdd<T>(field: keyof ExploreMapProps, list: T[], item: T) {
    update(field, [...list, item] as never);
  }

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
  const labelClass = 'mb-1 block text-xs font-medium text-[var(--color-muted)]';
  const regionBandRegions = [...new Set(WORLD_COUNTRIES.flatMap((c) => [c.region, c.subregion].filter(Boolean)))].sort();
</script>

<div class="space-y-4">
  <!-- Copy -->
  <div class="space-y-2">
    <label class="block"><span class={labelClass}>Headline</span>
      <input type="text" value={P.headline ?? ''} oninput={(e) => update('headline', e.currentTarget.value)} class={inputClass} /></label>
    <label class="block"><span class={labelClass}>Subtitle</span>
      <input type="text" value={P.subtitle ?? ''} oninput={(e) => update('subtitle', e.currentTarget.value)} class={inputClass} /></label>
  </div>

  <!-- Toggles -->
  <div>
    <span class={labelClass}>Panels &amp; controls</span>
    <div class="grid grid-cols-2 gap-1.5">
      {#each [
        ['show_toolbar', 'Toolbar'],
        ['show_legend', 'Map legend'],
        ['show_region_filter', 'Region filter'],
        ['show_stats', 'Quick stats'],
        ['show_col_index', 'COL index bar'],
        ['show_zoom', 'Zoom controls'],
      ] as [field, label]}
        <label class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
          <input type="checkbox" checked={P[field as keyof ExploreMapProps] as boolean} onchange={(e) => update(field as keyof ExploreMapProps, e.currentTarget.checked as never)} /> {label}
        </label>
      {/each}
    </div>
  </div>

  <!-- Pins -->
  <div class="space-y-2">
    <div class="grid grid-cols-2 gap-2">
      <label class="block"><span class={labelClass}>Pin source</span>
        <select value={P.geo_source} onchange={(e) => update('geo_source', e.currentTarget.value as ExploreMapProps['geo_source'])} class={inputClass}>
          <option value="api">Top country spaces</option>
          <option value="manual">Manual pins</option>
        </select></label>
      <label class="block"><span class={labelClass}>Pin limit</span>
        <input type="number" min="1" max="40" value={P.pin_limit ?? 8} oninput={(e) => update('pin_limit', Number(e.currentTarget.value))} class={inputClass} /></label>
    </div>
    {#if P.geo_source === 'manual'}
      {#each P.pins ?? [] as pin, i (i)}
        <div class="space-y-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
          <div class="flex items-center justify-between">
            <span class="text-xs text-[var(--color-muted)]">Pin {i + 1}</span>
            <button type="button" onclick={() => listRemove('pins', P.pins ?? [], i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
          </div>
          <div class="grid grid-cols-2 gap-1.5">
            <input type="text" value={pin.country} placeholder="country code (CA)" oninput={(e) => listUpdate<ExploreMapPin>('pins', P.pins ?? [], i, { country: e.currentTarget.value })} class={inputClass} />
            <input type="text" value={pin.label ?? ''} placeholder="label" oninput={(e) => listUpdate<ExploreMapPin>('pins', P.pins ?? [], i, { label: e.currentTarget.value })} class={inputClass} />
          </div>
          <input type="text" value={pin.tagline ?? ''} placeholder="tagline (shown on pin card)" oninput={(e) => listUpdate<ExploreMapPin>('pins', P.pins ?? [], i, { tagline: e.currentTarget.value })} class={inputClass} />
          <div class="grid grid-cols-2 gap-1.5">
            <select value={pin.category ?? ''} onchange={(e) => listUpdate<ExploreMapPin>('pins', P.pins ?? [], i, { category: e.currentTarget.value || undefined })} class={inputClass}>
              <option value="">No category</option>
              {#each P.categories ?? [] as c (c.key)}<option value={c.key}>{c.label}</option>{/each}
            </select>
            <input type="text" value={pin.href ?? ''} placeholder="href" oninput={(e) => listUpdate<ExploreMapPin>('pins', P.pins ?? [], i, { href: e.currentTarget.value })} class={inputClass} />
          </div>
        </div>
      {/each}
      <button type="button" onclick={() => listAdd<ExploreMapPin>('pins', P.pins ?? [], { country: '', label: '' })} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add pin</button>
    {/if}
  </div>

  <!-- Categories -->
  <div class="space-y-2">
    <span class={labelClass}>Legend categories</span>
    {#each P.categories ?? [] as cat, i (i)}
      <div class="space-y-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">{cat.label || `Category ${i + 1}`}</span>
          <button type="button" onclick={() => listRemove('categories', P.categories ?? [], i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        <div class="grid grid-cols-2 gap-1.5">
          <input type="text" value={cat.key} placeholder="key" oninput={(e) => listUpdate<ExploreMapCategory>('categories', P.categories ?? [], i, { key: e.currentTarget.value })} class={inputClass} />
          <input type="text" value={cat.icon ?? ''} placeholder="icon (Lucide)" oninput={(e) => listUpdate<ExploreMapCategory>('categories', P.categories ?? [], i, { icon: e.currentTarget.value })} class={inputClass} />
        </div>
        <input type="text" value={cat.label} placeholder="label" oninput={(e) => listUpdate<ExploreMapCategory>('categories', P.categories ?? [], i, { label: e.currentTarget.value })} class={inputClass} />
        <input type="text" value={cat.sublabel ?? ''} placeholder="sublabel" oninput={(e) => listUpdate<ExploreMapCategory>('categories', P.categories ?? [], i, { sublabel: e.currentTarget.value })} class={inputClass} />
        <label class="flex items-center gap-2 text-xs text-[var(--color-muted)]">Color
          <input type="color" value={cat.color} oninput={(e) => listUpdate<ExploreMapCategory>('categories', P.categories ?? [], i, { color: e.currentTarget.value })} class="h-6 w-10 cursor-pointer rounded border border-[var(--color-border)]" />
        </label>
      </div>
    {/each}
    <button type="button" onclick={() => listAdd<ExploreMapCategory>('categories', P.categories ?? [], { key: `cat_${Date.now() % 1e5}`, label: '', color: '#4a8df6' })} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add category</button>
  </div>

  <!-- Stats -->
  <div class="space-y-2">
    <span class={labelClass}>Quick stats</span>
    {#each P.stats ?? [] as stat, i (i)}
      <div class="space-y-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">{stat.label || `Stat ${i + 1}`}</span>
          <button type="button" onclick={() => listRemove('stats', P.stats ?? [], i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        <div class="grid grid-cols-2 gap-1.5">
          <input type="text" value={stat.icon ?? ''} placeholder="icon (Lucide)" oninput={(e) => listUpdate<ExploreMapStat>('stats', P.stats ?? [], i, { icon: e.currentTarget.value })} class={inputClass} />
          <input type="text" value={stat.label} placeholder="label" oninput={(e) => listUpdate<ExploreMapStat>('stats', P.stats ?? [], i, { label: e.currentTarget.value })} class={inputClass} />
        </div>
        <div class="grid grid-cols-3 gap-1.5">
          <select value={stat.source} onchange={(e) => listUpdate<ExploreMapStat>('stats', P.stats ?? [], i, { source: e.currentTarget.value as ExploreMapStat['source'] })} class={inputClass}>
            <option value="countries">Countries</option>
            <option value="col_cities">COL cities</option>
            <option value="blog">Blog posts</option>
            <option value="services">Services</option>
            <option value="manual">Manual</option>
          </select>
          <input type="text" value={stat.value ?? ''} placeholder="manual value" oninput={(e) => listUpdate<ExploreMapStat>('stats', P.stats ?? [], i, { value: e.currentTarget.value })} class={inputClass} />
          <input type="text" value={stat.suffix ?? ''} placeholder="suffix (+)" oninput={(e) => listUpdate<ExploreMapStat>('stats', P.stats ?? [], i, { suffix: e.currentTarget.value })} class={inputClass} />
        </div>
      </div>
    {/each}
    <button type="button" onclick={() => listAdd<ExploreMapStat>('stats', P.stats ?? [], { icon: 'Globe', label: '', source: 'manual', value: '' })} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add stat</button>
  </div>

  <!-- COL bands -->
  <div class="space-y-2">
    <span class={labelClass}>Cost of living bands</span>
    {#each P.col_bands ?? [] as band, i (i)}
      <div class="space-y-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2">
        <div class="flex items-center justify-between">
          <span class="text-xs text-[var(--color-muted)]">{band.label || `Band ${i + 1}`}</span>
          <button type="button" onclick={() => listRemove('col_bands', P.col_bands ?? [], i)} class="text-xs text-[var(--color-danger)] hover:underline">Remove</button>
        </div>
        <div class="grid grid-cols-2 gap-1.5">
          <input type="text" value={band.key} placeholder="key" oninput={(e) => listUpdate<ExploreMapBand>('col_bands', P.col_bands ?? [], i, { key: e.currentTarget.value })} class={inputClass} />
          <input type="text" value={band.label} placeholder="label" oninput={(e) => listUpdate<ExploreMapBand>('col_bands', P.col_bands ?? [], i, { label: e.currentTarget.value })} class={inputClass} />
        </div>
        <div class="grid grid-cols-[1fr_1fr_auto] items-center gap-1.5">
          <input type="number" value={band.min} oninput={(e) => listUpdate<ExploreMapBand>('col_bands', P.col_bands ?? [], i, { min: Number(e.currentTarget.value) })} class={inputClass} />
          <input type="number" value={band.max ?? ''} placeholder="max (blank = +)" oninput={(e) => listUpdate<ExploreMapBand>('col_bands', P.col_bands ?? [], i, { max: e.currentTarget.value === '' ? null : Number(e.currentTarget.value) })} class={inputClass} />
          <input type="color" value={band.color} oninput={(e) => listUpdate<ExploreMapBand>('col_bands', P.col_bands ?? [], i, { color: e.currentTarget.value })} class="h-7 w-10 cursor-pointer rounded border border-[var(--color-border)]" />
        </div>
      </div>
    {/each}
    <button type="button" onclick={() => listAdd<ExploreMapBand>('col_bands', P.col_bands ?? [], { key: `band_${Date.now() % 1e5}`, label: '', min: 0, max: null, color: '#6abf69' })} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add band</button>
  </div>

  <!-- Region → band map -->
  <div class="space-y-2">
    <span class={labelClass}>Region → band coloring</span>
    <p class="text-[11px] text-[var(--color-muted)]">Country fills resolve by subregion first, then region.</p>
    {#each regionBandRegions as region (region)}
      {@const bandKey = P.region_bands?.[region] ?? ''}
      <label class="flex items-center justify-between gap-2 text-xs">
        <span class="text-[var(--color-text)]">{region}</span>
        <select
          value={bandKey}
          onchange={(e) => {
            const next = { ...(P.region_bands ?? {}) };
            if (e.currentTarget.value) next[region] = e.currentTarget.value;
            else delete next[region];
            update('region_bands', next);
          }}
          class="w-40 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs text-[var(--color-text)]"
        >
          <option value="">— default —</option>
          {#each P.col_bands ?? [] as b (b.key)}<option value={b.key}>{b.label}</option>{/each}
        </select>
      </label>
    {/each}
  </div>

  <!-- Views + currency -->
  <div class="space-y-2">
    <span class={labelClass}>Toolbar views</span>
    {#each P.views ?? [] as view, i (i)}
      <div class="grid grid-cols-[1fr_1fr_auto] items-center gap-1.5">
        <input type="text" value={view.label} placeholder="label" oninput={(e) => listUpdate<ExploreMapView>('views', P.views ?? [], i, { label: e.currentTarget.value })} class={inputClass} />
        <select value={view.source} onchange={(e) => listUpdate<ExploreMapView>('views', P.views ?? [], i, { source: e.currentTarget.value as ExploreMapView['source'] })} class={inputClass}>
          <option value="countries">Countries</option>
          <option value="col_cities">COL cities</option>
          <option value="universities">Universities</option>
        </select>
        <button type="button" onclick={() => listRemove('views', P.views ?? [], i)} class="text-xs text-[var(--color-danger)] hover:underline">✕</button>
      </div>
    {/each}
    <button type="button" onclick={() => listAdd<ExploreMapView>('views', P.views ?? [], { key: `view_${Date.now() % 1e5}`, label: '', source: 'countries' })} class="w-full rounded-[var(--radius)] border border-dashed border-[var(--color-border)] py-1.5 text-sm text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]">+ Add view</button>

    <label class="block pt-1"><span class={labelClass}>Base currency (band ranges stored in)</span>
      <input type="text" value={P.default_currency ?? 'USD'} oninput={(e) => update('default_currency', e.currentTarget.value.toUpperCase())} class={inputClass} /></label>
  </div>
</div>
