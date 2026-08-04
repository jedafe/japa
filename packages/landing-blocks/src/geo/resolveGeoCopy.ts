import type { Block, GeoContext, GeoCopyTokens } from '../types';

/** Token alias map — short names resolve to the canonical token field. */
const TOKEN_ALIASES: Record<string, keyof GeoCopyTokens> = {
  country: 'country',
  country_short: 'country_short',
  country_adjective: 'country_adjective',
  continent: 'continent',
  region: 'region',
  diaspora_phrase: 'diaspora_phrase',
  diaspora: 'diaspora_phrase',
  destination_phrase: 'destination_phrase',
  destination: 'destination_phrase',
};

/** Regex matching {{token}} or {{token|fallback}}. */
const TOKEN_RE = /\{\{(\w+)(?:\|([^}]+))?\}\}/g;

/** Resolve all {{token}} placeholders in a single string. */
export function resolveString(str: string, tokens: GeoCopyTokens): string {
  return str.replace(TOKEN_RE, (match: string, tokenName: string, fallback?: string): string => {
    const field = TOKEN_ALIASES[tokenName];
    if (!field) return match; // unknown token — leave as-is
    const value = tokens[field];
    // The `names` field is an object, not a string — skip it.
    if (typeof value !== 'string' || value === '') return fallback ?? match;
    return value;
  });
}

/** Deep-walk an unknown value, resolving strings in-place (returns a clone). */
function resolveValue(value: unknown, tokens: GeoCopyTokens): unknown {
  if (typeof value === 'string') return resolveString(value, tokens);
  if (Array.isArray(value)) return value.map((v) => resolveValue(v, tokens));
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value)) out[k] = resolveValue(v, tokens);
    return out;
  }
  return value;
}

/** Resolve geo-aware copy in every block's props. Returns a new array (no mutation). */
export function resolveGeoCopy(blocks: Block[], ctx: GeoContext): Block[] {
  return blocks.map((block) => ({
    ...block,
    props: resolveValue(block.props, ctx.tokens) as Block['props'],
  })) as Block[];
}
