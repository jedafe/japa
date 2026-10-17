<script lang="ts">
  // Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
  // World Explorer child — the map card itself: the pannable/zoomable SVG,
  // pin markers + hover cards, zoom controls + compass, loading overlay, and
  // the bottom-mode floating stats overlay. All state lives on the parent
  // ExploreMapContext.
  import type { ExploreMapCanvasProps } from '../../types';
  import type { FetchContext } from '../../data/types';
  import { getExploreMapCtx, exploreMapChildVisible, CARD_W, CARD_H } from './context.svelte';
  import type { Pin } from './context.svelte';
  import { WORLD_COUNTRIES, WORLD_VIEWBOX } from '../../geo/worldMapData';
  import * as LucideIcons from 'lucide-svelte';

  interface Props {
    props: ExploreMapCanvasProps;
    fetchCtx?: FetchContext;
    geoCountry?: string;
    id?: string;
  }
  let { props }: Props = $props();

  const ctx = getExploreMapCtx();
  const VB_W = WORLD_VIEWBOX.w;
</script>

{#if ctx && exploreMapChildVisible(ctx.P, 'explore_map_canvas')}
  <div class="relative overflow-hidden rounded-2xl border border-[var(--color-border)] shadow-sm" style="background: linear-gradient(180deg, #e8f3fb 0%, #dcedf7 100%);">
    {#if ctx.loading}
      <div class="absolute inset-0 z-10 grid place-items-center">
        <span class="text-sm text-[var(--color-muted)]">Loading map…</span>
      </div>
    {/if}
    <svg
      bind:this={ctx.svgEl}
      viewBox="{ctx.vb[0]} {ctx.vb[1]} {ctx.vb[2]} {ctx.vb[3]}"
      preserveAspectRatio={ctx.P.cover_mode ? 'xMidYMid slice' : 'xMidYMid meet'}
      class="block w-full select-none {ctx.P.cover_mode ? 'h-64 sm:h-72' : 'aspect-[2/1]'} {ctx.panning ? 'cursor-grabbing' : 'cursor-grab'}"
      style="overflow: visible"
      role="img"
      aria-label="Interactive world map of destinations"
      onpointerdown={(e) => ctx.onDown(e)}
      onpointermove={(e) => ctx.onMove(e)}
      onpointerup={() => ctx.onUp()}
      onpointercancel={() => ctx.onUp()}
      onpointerleave={() => ctx.onLeave()}
      onclickcapture={(e) => ctx.onClickCapture(e)}
    >
      <rect x={ctx.vb[0]} y={ctx.vb[1]} width={ctx.vb[2]} height={ctx.vb[3]} fill="transparent" />
      {#each WORLD_COUNTRIES as c (c.code)}
        {@const inRegion = !ctx.selRegion || c.region === ctx.selRegion}
        {@const isVisitor = ctx.visitorCode !== '' && c.code === ctx.visitorCode}
        <path
          d={c.d}
          fill={ctx.bandColorFor(c.region, c.subregion)}
          fill-opacity={isVisitor ? 1 : inRegion ? (ctx.P.focus_d ? 0.4 : 0.85) : 0.18}
          stroke={isVisitor ? 'var(--color-primary)' : '#ffffff'}
          stroke-opacity={inRegion ? 0.9 : 0.35}
          stroke-width={isVisitor ? 3 * (ctx.vb[2] / VB_W) : 1.2 * (ctx.vb[2] / VB_W)}
        />
      {/each}

      {#if ctx.P.container_d}
        <!-- Container outline: the parent unit's boundary drawn bold
             around the focus shape (state around a city breakout). -->
        <path
          d={ctx.P.container_d}
          fill="none"
          stroke="var(--color-primary)"
          stroke-opacity={0.9}
          stroke-width={2.5 * (ctx.vb[2] / VB_W)}
        />
      {/if}
      {#if ctx.P.focus_d}
        <!-- Breakout shape: the focused unit drawn over the
             country context (state/city outline inside its parent). -->
        <path
          d={ctx.P.focus_d}
          fill="var(--color-primary)"
          fill-opacity={0.7}
          stroke="#ffffff"
          stroke-opacity={0.9}
          stroke-width={1.5 * (ctx.vb[2] / VB_W)}
        />
      {/if}

      {#each ctx.visiblePins as p, i (p.country + i)}
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
      {#each ctx.visiblePins.filter((p) => p.tagline) as p (p.country)}
        {@render pinCard(p)}
      {/each}
      {#if ctx.hoverIdx >= 0 && ctx.visiblePins[ctx.hoverIdx] && !ctx.visiblePins[ctx.hoverIdx].tagline}
        {@render pinCard(ctx.visiblePins[ctx.hoverIdx])}
      {/if}
    </svg>

    <!-- Zoom controls + compass -->
    {#if ctx.P.show_zoom}
      <div class="absolute bottom-4 right-4 flex flex-col items-end gap-2">
        <div class="flex flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-sm">
          <button type="button" aria-label="Zoom in" class="grid h-9 w-9 place-items-center text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]" onclick={() => ctx.zoomBy(0.7)}>
            <LucideIcons.Plus size={15} />
          </button>
          <div class="h-px bg-[var(--color-border)]"></div>
          <button type="button" aria-label="Zoom out" class="grid h-9 w-9 place-items-center text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]" onclick={() => ctx.zoomBy(1.45)}>
            <LucideIcons.Minus size={15} />
          </button>
          <div class="h-px bg-[var(--color-border)]"></div>
          <button type="button" aria-label="Reset view" class="grid h-9 w-9 place-items-center text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]" onclick={() => ctx.resetView()}>
            <LucideIcons.LocateFixed size={15} />
          </button>
        </div>
        <button type="button" aria-label="Reset north" class="grid h-11 w-11 place-items-center" onclick={() => ctx.resetView()}>
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
    {#if ctx.P.aside_position === 'bottom' && ctx.P.show_stats && (ctx.P.stats?.length ?? 0) > 0}
      <div class="absolute bottom-4 left-4 w-56 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)]/90 p-3 shadow-sm backdrop-blur-sm">
        <h3 class="mb-2.5 text-[11px] font-semibold tracking-wide text-[var(--color-muted)]">Quick stats</h3>
        <ul class="space-y-2.5">
          {#each ctx.P.stats ?? [] as stat (stat.label)}
            {@const Icon = ctx.iconFor(stat.icon)}
            <li class="flex items-center gap-2.5">
              <span class="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
                {#if Icon}<Icon size={14} />{/if}
              </span>
              <span class="min-w-0">
                <span class="block text-sm font-bold leading-tight text-[var(--color-text)]">{ctx.statValue(stat.source, stat.value, stat.suffix)}</span>
                <span class="block truncate text-[11px] text-[var(--color-muted)]">{stat.label}</span>
              </span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>
{/if}

{#snippet marker(p: Pin)}
  {@const zf = ctx ? ctx.vb[2] / VB_W : 1}
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
  {@const zf = ctx ? ctx.vb[2] / VB_W : 1}
  {@const flip = ctx ? p.y - (CARD_H + 18) * zf < ctx.vb[1] : false}
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
