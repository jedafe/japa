// packages/landing-blocks/src/data/entityFetchers.ts
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
  const headers: Record<string, string> = {};
  if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;
  if (ctx.host) { headers.host = ctx.host; headers['x-forwarded-host'] = ctx.host; }

  const pathMap: Record<EntityType, string> = {
    post: `/explore${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    blog: `/blog${qs(query.filters, { page_size: limit, ...sortParam })}`,
    // thread → /forum (forum threads), NOT /explore (mixed feed). The forum
    // endpoint lists threads by created_at desc; sort/geo params are ignored.
    thread: `/forum${qs(query.filters, { page_size: limit })}`,
    // space/organization → /spaces (returns real destination data with
    // geo-filtering support; /trending/spaces is also available but returns
    // trending snapshots which may be empty before the first compute cycle).
    space: `/spaces${qs(query.filters, { page_size: limit, ...geoParam })}`,
    organization: `/spaces${qs(query.filters, { page_size: limit, ...geoParam })}`,
    university: `/universities${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    job: `/jobs${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    scholarship: `/scholarships${qs(query.filters, { page_size: limit, ...sortParam, ...geoParam })}`,
    tool: '', // handled locally — no API call
    remittance_rate: `/remittance/rates${qs(query.filters, { limit })}`,
  };

  if (type === 'tool') return TOOL_REGISTRY.slice(0, limit);

  const path = pathMap[type];
  if (!path) return [];

  try {
    const res = await ctx.fetch(`${ctx.apiBase}/api/v1${path}`, { headers });
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
  const headers: Record<string, string> = {};
  if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;
  if (ctx.host) { headers.host = ctx.host; headers['x-forwarded-host'] = ctx.host; }

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
    const res = await ctx.fetch(`${ctx.apiBase}/api/v1${path}`, { headers });
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
  const headers: Record<string, string> = {};
  if (ctx.token) headers.authorization = `Bearer ${ctx.token}`;
  if (ctx.host) { headers.host = ctx.host; headers['x-forwarded-host'] = ctx.host; }
  try {
    const res = await ctx.fetch(`${ctx.apiBase}/api/v1/remittance/rates?${params}`, { headers });
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
