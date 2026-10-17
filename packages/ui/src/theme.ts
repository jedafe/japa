/**
 * Shared site-theming helper for the four SSR surfaces (main, docs, academy,
 * tools). Maps the public `appearance.*` system settings (served via
 * GET /site/settings/public) to a CSS override block that each app's root
 * layout injects into <svelte:head>, so admin theme changes apply during SSR
 * with no flash.
 *
 * Selector strategy mirrors the pre-existing primary_color handling:
 * `html:root` / `html:root[data-theme='dark']` out-specify the app.css
 * `:root` blocks, and `html:root hN` (0-1-2) out-specifies Tailwind utility
 * classes (0-1-0) so heading weight/size controls actually take effect.
 *
 * Semantics (conventions/system-settings-catalog.md):
 *  - Light color values apply to :root only.
 *  - `*_dark` values apply to [data-theme='dark'] only; empty = theme default.
 *  - appearance.primary_color is the exception: it applies to BOTH themes for
 *    backwards compatibility; primary_color_dark overrides it in dark.
 *
 * All values are validated before interpolation (hex whitelist, enum
 * whitelists) — arbitrary setting text never reaches the emitted CSS.
 */

export interface ThemeSettings {
	[key: string]: unknown;
}

// Valid CSS hex forms: #rgb, #rgba, #rrggbb, #rrggbbaa.
const HEX_RE = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

const FONT_STACKS: Record<string, string> = {
	inter: "'Inter', ui-sans-serif, system-ui, sans-serif",
	lora: "'Lora', ui-serif, Georgia, serif",
	poppins: "'Poppins', ui-sans-serif, system-ui, sans-serif",
	roboto: "'Roboto', ui-sans-serif, system-ui, sans-serif",
	'open-sans': "'Open Sans', ui-sans-serif, system-ui, sans-serif",
	montserrat: "'Montserrat', ui-sans-serif, system-ui, sans-serif",
	'system-ui': "ui-sans-serif, system-ui, sans-serif",
	georgia: "Georgia, 'Times New Roman', serif",
	monospace: "ui-monospace, 'SF Mono', Menlo, monospace"
};

const FONT_SIZES = new Set(['14', '15', '16', '17', '18']);
const FONT_WEIGHTS = new Set(['400', '500', '600']);
const HEADING_WEIGHTS = new Set(['500', '600', '700', '800', '900']);
const RADII = new Set(['0', '4', '8', '12', '16']);

/** Heading size scale presets — multiplier over the per-level base em size. */
const HEADING_SCALES: Record<string, number> = {
	compact: 0.9,
	default: 1.0,
	comfortable: 1.125,
	large: 1.25,
	xl: 1.5
};

/** Base em sizes per heading level (relative to the surrounding font size). */
const HEADING_BASE_EM: Record<string, number> = {
	h1: 2.0,
	h2: 1.6,
	h3: 1.3,
	h4: 1.15,
	h5: 1.0,
	h6: 0.9
};

const HEADING_SELECTORS = ['h1', 'h2', 'h3', 'h4', 'h5', 'h6'];

function str(settings: ThemeSettings, key: string): string {
	const v = settings[key];
	return typeof v === 'string' ? v.trim() : '';
}

function hex(settings: ThemeSettings, key: string): string {
	const v = str(settings, key);
	return HEX_RE.test(v) ? v : '';
}

/** Light-theme color overrides keyed to :root. */
const LIGHT_COLOR_VARS: [string, string][] = [
	['appearance.secondary_color', '--color-secondary'],
	['appearance.background_color', '--color-bg'],
	['appearance.surface_color', '--color-surface'],
	['appearance.surface_subtle_color', '--color-surface-subtle'],
	['appearance.text_color', '--color-text'],
	['appearance.text_subtle_color', '--color-text-subtle'],
	['appearance.muted_color', '--color-muted'],
	['appearance.border_color', '--color-border']
];

/** Dark-theme overrides keyed to [data-theme='dark']. */
const DARK_COLOR_VARS: [string, string][] = [
	['appearance.secondary_color_dark', '--color-secondary'],
	['appearance.background_color_dark', '--color-bg'],
	['appearance.surface_color_dark', '--color-surface'],
	['appearance.surface_subtle_color_dark', '--color-surface-subtle'],
	['appearance.text_color_dark', '--color-text'],
	['appearance.text_subtle_color_dark', '--color-text-subtle'],
	['appearance.muted_color_dark', '--color-muted'],
	['appearance.border_color_dark', '--color-border']
];

/**
 * Relative luminance → contrasting text color. An admin-chosen primary could
 * easily be too light for white text (or too dark for dark text), so the
 * contrast token is derived rather than configured.
 */
function contrastFor(hexColor: string): string {
	const h = hexColor.replace('#', '');
	const full =
		h.length === 3 || h.length === 4
			? h
					.split('')
					.slice(0, 3)
					.map((c) => c + c)
					.join('')
			: h;
	const r = parseInt(full.slice(0, 2), 16) / 255;
	const g = parseInt(full.slice(2, 4), 16) / 255;
	const b = parseInt(full.slice(4, 6), 16) / 255;
	const lum = 0.2126 * r + 0.7152 * g + 0.0722 * b;
	return lum > 0.45 ? '#0a0a0a' : '#ffffff';
}

function headingRules(scale: number, weight: string): string {
	const rules: string[] = [];
	if (scale !== 1.0) {
		for (const tag of HEADING_SELECTORS) {
			rules.push(
				`html:root ${tag} { font-size: ${(HEADING_BASE_EM[tag] * scale).toFixed(3).replace(/0+$/, '').replace(/\.$/, '')}em; }`
			);
		}
	}
	if (weight) {
		rules.push(
			`${HEADING_SELECTORS.map((t) => `html:root ${t}`).join(', ')} { font-weight: ${weight}; }`
		);
	}
	return rules.join('\n');
}

/**
 * Returns the inner CSS for a `<style>` tag applying the admin's appearance
 * settings. Empty string when nothing needs overriding — callers render the
 * tag only when the result is non-empty.
 */
export function buildThemeOverrides(settings: ThemeSettings | null | undefined): string {
	if (!settings) return '';
	const light: string[] = [];
	const dark: string[] = [];

	// Backwards-compatible: primary_color applies to both themes. Hover is
	// derived via color-mix (mirroring app.css's --color-secondary-hover) so
	// hover states track the admin-chosen primary instead of the stale
	// default hex.
	const primary = hex(settings, 'appearance.primary_color');
	const primaryDark = hex(settings, 'appearance.primary_color_dark');
	for (const [key, cssVar] of LIGHT_COLOR_VARS) {
		const v = hex(settings, key);
		if (v) light.push(`${cssVar}: ${v};`);
	}
	for (const [key, cssVar] of DARK_COLOR_VARS) {
		const v = hex(settings, key);
		if (v) dark.push(`${cssVar}: ${v};`);
	}

	const fontSans = FONT_STACKS[str(settings, 'appearance.font_family')];
	if (fontSans) light.push(`--font-sans: ${fontSans};`);
	const fontSerif = FONT_STACKS[str(settings, 'appearance.font_family_serif')];
	if (fontSerif) light.push(`--font-serif: ${fontSerif};`);

	const radius = str(settings, 'appearance.border_radius');
	if (RADII.has(radius)) {
		light.push(`--radius: ${radius}px;`, `--radius-card: ${Number(radius) + 4}px;`);
	}

	const out: string[] = [];
	if (primary) {
		light.push(`--color-primary: ${primary};`, `--color-primary-hover: color-mix(in oklab, ${primary} 82%, black);`, `--color-primary-contrast: ${contrastFor(primary)};`);
		const dp = primaryDark || primary;
		dark.push(`--color-primary: ${dp};`, `--color-primary-hover: color-mix(in oklab, ${dp} 82%, white);`, `--color-primary-contrast: ${contrastFor(dp)};`);
	} else if (primaryDark) {
		dark.push(`--color-primary: ${primaryDark};`, `--color-primary-hover: color-mix(in oklab, ${primaryDark} 82%, white);`, `--color-primary-contrast: ${contrastFor(primaryDark)};`);
	}

	// Secondary hover/contrast — same derivation so admin-chosen secondary
	// stays legible. secondary_color only applies to the light block; the
	// dark variant comes from secondary_color_dark (or inherits the token).
	const secondary = hex(settings, 'appearance.secondary_color');
	const secondaryDark = hex(settings, 'appearance.secondary_color_dark');
	if (secondary) {
		light.push(`--color-secondary-hover: color-mix(in oklab, ${secondary} 82%, black);`, `--color-secondary-contrast: ${contrastFor(secondary)};`);
	}
	if (secondaryDark) {
		dark.push(`--color-secondary-hover: color-mix(in oklab, ${secondaryDark} 82%, white);`, `--color-secondary-contrast: ${contrastFor(secondaryDark)};`);
	}
	if (light.length) out.push(`html:root { ${light.join(' ')} }`);
	if (dark.length) out.push(`html:root[data-theme='dark'] { ${dark.join(' ')} }`);

	const fontSize = str(settings, 'appearance.font_size');
	const fontWeight = str(settings, 'appearance.font_weight');
	const bodyRules: string[] = [];
	if (FONT_SIZES.has(fontSize)) bodyRules.push(`font-size: ${fontSize}px;`);
	if (FONT_WEIGHTS.has(fontWeight)) bodyRules.push(`font-weight: ${fontWeight};`);
	if (bodyRules.length) out.push(`html:root body { ${bodyRules.join(' ')} }`);

	// Headings — desktop base, then tablet (≤1024px) and mobile (≤640px).
	const hScale = HEADING_SCALES[str(settings, 'appearance.heading_size')] ?? 1.0;
	const hWeight = HEADING_WEIGHTS.has(str(settings, 'appearance.heading_weight'))
		? str(settings, 'appearance.heading_weight')
		: '';
	const base = headingRules(hScale, hWeight);
	if (base) out.push(base);

	// Heading font family — 'inherit' (or unset) follows the body font.
	// --font-heading is exposed as a token so elements with an inline
	// font-family (e.g. article titles, serif by design) opt in via
	// var(--font-heading, var(--font-serif)) fallback chains.
	const hFont = FONT_STACKS[str(settings, 'appearance.heading_font_family')];
	if (hFont) {
		out.push(`html:root { --font-heading: ${hFont}; }`);
		out.push(
			`${HEADING_SELECTORS.map((t) => `html:root ${t}`).join(', ')} { font-family: var(--font-heading); }`
		);
	}

	for (const [suffix, query] of [
		['tablet', '1024px'],
		['mobile', '640px']
	] as const) {
		const sRaw = str(settings, `appearance.heading_size_${suffix}`);
		const wRaw = str(settings, `appearance.heading_weight_${suffix}`);
		const s = sRaw === 'inherit' || sRaw === '' ? 0 : (HEADING_SCALES[sRaw] ?? 0);
		const w = wRaw === 'inherit' || wRaw === '' || !HEADING_WEIGHTS.has(wRaw) ? '' : wRaw;
		if (!s && !w) continue;
		const rules = headingRules(s || 1.0, w);
		if (rules) out.push(`@media (max-width: ${query}) {\n${rules}\n}`);
	}

	return out.join('\n');
}
