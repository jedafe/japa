// packages/landing-blocks/src/data/entityFetchers.ts
import { buildApiUrl } from '@japaconnect/api-url';
import type { EntityType } from '../types';
import type { FetchContext, ToolSummary, RemittanceRate } from './types';

/** Curated tool registry — deep links to live tool routes (no hardcoded content data). */
export const TOOL_REGISTRY: ToolSummary[] = [
  { id: 'visa-checklist', name: 'Visa Checklist', slug: 'visa-checklist', description: 'Generate your visa document checklist by destination.', href: '/tools/visa-checklist', icon: 'FileCheck', accent: 'var(--chart-1)' },
  { id: 'readiness-assessment', name: 'Japa Readiness Assessment', slug: 'readiness-assessment', description: 'Assess your readiness to relocate abroad.', href: '/tools/readiness', icon: 'ClipboardCheck', accent: 'var(--chart-2)' },
  { id: 'cost-of-living', name: 'Cost of Living Calculator', slug: 'cost-of-living', description: 'Compare living costs across destination cities.', href: '/tools/cost-of-living', icon: 'Calculator', accent: 'var(--chart-3)' },
  { id: 'scholarship-finder', name: 'Scholarship Finder', slug: 'scholarship-finder', description: 'Find scholarships for your destination and level.', href: '/scholarships', icon: 'GraduationCap', accent: 'var(--chart-4)' },
  { id: 'remittance-tracker', name: 'Remittance Rate Tracker', slug: 'remittance-tracker', description: 'Track live remittance rates across corridors.', href: '/tools/remittance', icon: 'TrendingUp', accent: 'var(--chart-5)' },
  { id: 'japabot', name: 'JapaBot', slug: 'japabot', description: 'AI assistant for your relocation questions.', href: '/ai/chat', icon: 'Bot', badge: 'AI', accent: 'var(--color-primary)' },
  { id: 'jobs', name: 'Jobs Board', slug: 'jobs', description: 'Browse jobs with visa sponsorship options.', href: '/jobs', icon: 'Briefcase', accent: 'var(--chart-1)' },
  { id: 'travel-search', name: 'Travel Search', slug: 'travel-search', description: 'Search and compare flights and hotels.', href: '/tools/travel', icon: 'Plane', accent: 'var(--chart-2)' },
];

/** Shared request headers for backend fetches: auth, host forwarding, and
 * the consuming app's surface (X-JC-Surface) for scoped feature-flag
 * resolution. */
function ctxHeaders(ctx: FetchContext): Record<string, string> {
  const headers: Record<string, string> = {};
  if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;
  if (ctx.host) { headers.host = ctx.host; headers['x-forwarded-host'] = ctx.host; }
  if (ctx.surface) headers['x-jc-surface'] = ctx.surface;
  return headers;
}

/** Build query string from a filters object. */
function qs(filters: Record<string, string> | undefined, extra: Record<string, string | number | undefined> = {}): string {
  const params = new URLSearchParams();
  if (filters) for (const [k, v] of Object.entries(filters)) if (v) params.set(k, v);
  for (const [k, v] of Object.entries(extra)) if (v !== undefined && v !== '') params.set(k, String(v));
  const s = params.toString();
  return s ? `?${s}` : '';
}

/** Fetch a collection of entities by type. */
export async function fetchEntityCollection(
  type: EntityType,
  query: { filters?: Record<string, string>; sort?: string; limit?: number; geo?: boolean },
  ctx: FetchContext,
  geoCountry?: string
): Promise<unknown[]> {
  const limit = query.limit ?? 6;
  const geoParam = query.geo && geoCountry ? { geo: geoCountry } : {};
  const sortParam = query.sort ? { sort: query.sort } : {};
  const headers = ctxHeaders(ctx);

  // Tool cards are now configurable via the public site settings endpoint
  // (landing.tools). Rule 16: no hardcoded data in production pages.
  if (type === 'tool') {
    try {
      const res = await ctx.fetch(buildApiUrl('/site/settings/public', ctx.apiBase), { headers });
      if (res.ok) {
        const json = (await res.json()) as { data?: Record<string, unknown> };
        const tools = json.data?.['landing.tools'] as unknown[] | undefined;
        if (Array.isArray(tools) && tools.length > 0) return tools.slice(0, limit);
      }
    } catch (err) {
      console.error('[entityFetchers] tool settings fetch failed', err);
    }
    // Fallback only when the public setting is absent or the request fails.
    return TOOL_REGISTRY.slice(0, limit);
  }

  const pathMap: Partial<Record<EntityType, string>> = {
    post: `/explore${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    blog: `/blog${qs(query.filters, { page_size: limit, ...sortParam })}`,
    // thread → /forum (forum threads), NOT /explore (mixed feed). The forum
    // endpoint lists threads by created_at desc; sort/geo params are ignored.
    thread: `/forum${qs(query.filters, { page_size: limit })}`,
    // space/organization → /spaces (returns real destination data with
    // geo-filtering support; /trending/spaces is also available but returns
    // trending snapshots which may be empty before the first compute cycle).
    space: `/spaces${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    trending_space: `/trending/spaces${qs(query.filters, { limit, ...geoParam })}`,
    organization: `/spaces${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    university: `/universities${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    job: `/jobs${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    scholarship: `/scholarships${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    remittance_rate: `/remittance/rates${qs(query.filters, { limit })}`,
  };

  const path = pathMap[type];
  if (!path) return [];

  try {
    const res = await ctx.fetch(buildApiUrl(path, ctx.apiBase), { headers });
    if (!res.ok) {
      console.error(`[entityFetchers] fetchEntityCollection(${type}) ${res.status} ${res.statusText} — ${path}`);
      return [];
    }
    const json = await res.json();
    return (json.data ?? json) as unknown[];
  } catch (err) {
    console.error(`[entityFetchers] fetchEntityCollection(${type}) network error — ${path}`, err);
    return [];
  }
}

/** Fetch a single entity by ref. */
export async function fetchEntityFeature(
  type: EntityType,
  ref: { id?: string; slug?: string } | undefined,
  ctx: FetchContext
): Promise<unknown | null> {
  if (!ref?.id && !ref?.slug) return null;
  const headers = ctxHeaders(ctx);

  const id = ref.id ?? ref.slug;
  const pathMap: Partial<Record<EntityType, string>> = {
    blog: `/blog/${id}`,
    space: `/spaces/${id}`,
    organization: `/spaces/${id}`,
    university: `/universities/${id}`,
    job: `/jobs/${id}`,
    scholarship: `/scholarships/${id}`,
  };

  const path = pathMap[type];
  if (!path) return null;

  try {
    const res = await ctx.fetch(buildApiUrl(path, ctx.apiBase), { headers });
    if (!res.ok) {
      console.error(`[entityFetchers] fetchEntityFeature(${type}) ${res.status} ${res.statusText} — ${path}`);
      return null;
    }
    const json = await res.json();
    return (json.data ?? json) as unknown;
  } catch (err) {
    console.error(`[entityFetchers] fetchEntityFeature(${type}) network error — ${path}`, err);
    return null;
  }
}

/** Fetch remittance rates (used by LiveRateTickerBlock in Plan 2). */
export async function fetchRemittanceRates(
  ctx: FetchContext,
  corridors?: string[],
  limit: number = 10
): Promise<RemittanceRate[]> {
  const params = new URLSearchParams();
  if (corridors) for (const c of corridors) params.append('corridor', c);
  params.set('limit', String(limit));
  const headers = ctxHeaders(ctx);
  try {
    const res = await ctx.fetch(buildApiUrl(`/remittance/rates?${params}`, ctx.apiBase), { headers });
    if (!res.ok) {
      console.error(`[entityFetchers] fetchRemittanceRates ${res.status} ${res.statusText}`);
      return [];
    }
    const json = await res.json();
    return (json.data ?? json) as RemittanceRate[];
  } catch (err) {
    console.error('[entityFetchers] fetchRemittanceRates network error', err);
    return [];
  }
}
