// packages/landing-blocks/src/data/types.ts

/** Minimal fetcher context — the consuming app provides fetch + API base. */
export interface FetchContext {
  fetch: typeof fetch;
  apiBase: string;
  /** Optional auth token for endpoints that require it. */
  token?: string | null;
  /** Optional CSRF token for mutating public endpoints. */
  csrfToken?: string | null;
  /** Request host header. */
  host?: string;
  /**
   * Consuming app's surface identifier, sent as X-JC-Surface so the backend
   * resolves feature flags and geo rules for the correct surface (valid
   * values: internal/features/eval.go).
   */
  surface?: string;
}

// API response shapes (derived from backend struct tags — no `any`).

export interface JobSummary {
  id: string;
  title: string;
  slug?: string;
  company?: string;
  location?: string;
  country?: string;
  remote?: boolean;
  visa_sponsorship?: boolean;
  salary_range?: string;
  type?: string;
  featured?: boolean;
}

export interface ScholarshipSummary {
  id: string;
  title: string;
  name?: string;
  slug?: string;
  country?: string;
  amount?: string;
  deadline?: string;
  eligibility?: string;
  level?: string;
}

export interface UniversitySummary {
  id: string;
  name: string;
  slug: string;
  country?: string;
  world_ranking?: number;
  country_ranking?: number;
  logo_url?: string;
  programs?: string[];
}

export interface SpaceSummary {
  id: string;
  name: string;
  slug: string;
  description?: string;
  icon_url?: string;
  member_count?: number;
  follower_count?: number;
}

export interface BlogSummary {
  id: string;
  title: string;
  slug: string;
  excerpt?: string;
  cover_image_url?: string;
  published_at?: string;
  author?: { name: string; avatar_url?: string };
}

export interface FeedItemSummary {
  id: string;
  title?: string;
  content_preview?: string;
  content_type?: string;
  space?: { name: string; slug: string };
  author?: { name: string; username: string; avatar_url?: string };
  created_at?: string;
  reaction_count?: number;
  comment_count?: number;
}

export interface RemittanceRate {
  id: string;
  corridor: string;
  from_country: string;
  to_country: string;
  rate: number;
  provider?: string;
  fee?: number;
  updated_at?: string;
}

export interface ToolSummary {
  id: string;
  name: string;
  slug: string;
  description: string;
  href: string;
  icon?: string;
  badge?: string;
  accent?: string;
}
