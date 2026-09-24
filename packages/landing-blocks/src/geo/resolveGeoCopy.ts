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
const TOKEN_RE = /\{\{\s*([A-Za-z_][A-Za-z0-9_]*)(?:\|([^}]+))?\s*\}\}/g;

// Folded literal maps keyed by the original object so callers can reuse the
// same `literals` reference across many strings without rebuilding.
const foldedCache = new WeakMap<Record<string, string>, Record<string, string>>();

function foldedLiterals(literals: Record<string, string>): Record<string, string> {
  let folded = foldedCache.get(literals);
  if (!folded) {
    folded = {};
    for (const [k, v] of Object.entries(literals)) folded[k.toLowerCase()] = v;
    foldedCache.set(literals, folded);
  }
  return folded;
}

/** Resolve all {{token}} placeholders in a single string. */
export function resolveString(
  str: string,
  tokens: GeoCopyTokens,
  literals?: Record<string, string>
): string {
  if (typeof str !== 'string') return str as unknown as string;
  const folded = literals ? foldedLiterals(literals) : undefined;
  return str.replace(TOKEN_RE, (match: string, tokenName: string, fallback?: string): string => {
    const field = TOKEN_ALIASES[tokenName.toLowerCase()];
    if (field) {
      const value = tokens[field];
      // The `names` field is an object, not a string — skip it.
      if (typeof value === 'string' && value !== '') return value;
    }
    if (folded) {
      const value = folded[tokenName.toLowerCase()];
      if (typeof value !== 'undefined') return value;
    }
    if (typeof fallback !== 'undefined' && fallback !== '') return fallback.trim();
    return match;
  });
}

/** Deep-walk an unknown value, resolving strings in-place (returns a clone). */
function resolveValue(value: unknown, tokens: GeoCopyTokens, literals?: Record<string, string>): unknown {
  if (typeof value === 'string') return resolveString(value, tokens, literals);
  if (Array.isArray(value)) return value.map((v) => resolveValue(v, tokens, literals));
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(value)) out[k] = resolveValue(v, tokens, literals);
    return out;
  }
  return value;
}

/** Resolve geo-aware and template-literal copy in every block's props. Returns a new array (no mutation). */
export function resolveGeoCopy(blocks: Block[], ctx: GeoContext): Block[] {
  return blocks.map((block) => ({
    ...block,
    props: resolveValue(block.props, ctx.tokens, ctx.literals) as Block['props'],
  })) as Block[];
}
