// Ref: japaconnect-engineering/sections/16-page-builder.md (explore_map)
// Shared interactive state + data for the `explore_map` parent block and its
// `explore_map_*` children. The parent instantiates this, setContext()s it,
// and runs the data-loading $effects against it; children read/write it via
// getExploreMapCtx(). Ported verbatim from the former monolith script.
import { getContext, setContext } from 'svelte';
import type { ExploreMapProps } from '../../types';
import type { FetchContext } from '../../data/types';
import { countryCentroid } from '../../geo/countryCentroids';
import { WORLD_COUNTRIES, WORLD_VIEWBOX, REGION_BOUNDS } from '../../geo/worldMapData';
import { EXPLORE_MAP_DEFAULTS } from './exploreMapDefaults';
import { buildApiUrl } from '@japaconnect/api-url';

export const EXPLORE_MAP_CTX = Symbol('explore-map');

export interface GeoSpaceLite {
	id: string; name: string; slug: string; level: string;
	geo_code?: string; description?: string; currency_code?: string | null;
	member_count?: number; follower_count?: number;
}
export interface FxCurrency { code: string; name: string; symbol: string }
export interface Pin {
	country: string; label: string; tagline?: string; category?: string; href?: string;
	x: number; y: number; color: string; flag: string;
}

const countryByCode = new Map(WORLD_COUNTRIES.map((c) => [c.code, c]));
const nameToCode = new Map(WORLD_COUNTRIES.map((c) => [c.name.toLowerCase(), c.code]));
const FALLBACK_FILL = '#ccdde8';
export const CARD_W = 150;
export const CARD_H = 60;
const VB_W = WORLD_VIEWBOX.w;
const VB_H = WORLD_VIEWBOX.h;
const reducedMotion = typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;

interface ExploreMapContextArgs {
	props: () => ExploreMapProps;
	fetchCtx: () => FetchContext | undefined;
	geoCountry: () => string | undefined;
}

export class ExploreMapContext {
	constructor(private args: ExploreMapContextArgs) {}

	/** Merged props — a getter (not $derived) so the class constructor can run
	 *  before this field initializes; still reactive since props() reads the
	 *  parent's $props() binding inside tracking contexts. */
	get P(): ExploreMapProps {
		return { ...EXPLORE_MAP_DEFAULTS, ...this.args.props() };
	}

	// ── Remote data (written by the parent's $effects) ─────────────
	apiCountries = $state<GeoSpaceLite[]>([]);
	apiRegions = $state<string[]>([]);
	currencies = $state<FxCurrency[]>([]);
	fxRates = $state<Record<string, number>>({});
	statCounts = $state<Record<string, number>>({});
	viewPins = $state<Pin[]>([]);
	loading = $state(true);

	// ── UI state ─────────────────────────────────────────────────
	selRegion = $state('');
	vbFit = $state(false);
	selCategory = $state('');
	selView = $state('');
	currency = $state('');
	vb = $state<[number, number, number, number]>([0, 0, VB_W, VB_H]);
	hoverIdx = $state(-1);
	svgEl = $state<SVGSVGElement | null>(null);

	// Pointer drag → pan; pointer proximity → pin hover (screen-space, so the
	// hit radius stays a constant ~14px at any zoom).
	panning = $state<{ px: number; py: number; vb: [number, number, number, number]; moved: boolean } | null>(null);
	justDragged = false;
	private tweenId = 0;

	headers(): Record<string, string> {
		const fetchCtx = this.args.fetchCtx();
		const h: Record<string, string> = {};
		if (!fetchCtx) return h;
		if (fetchCtx.token) h.authorization = `Bearer ${fetchCtx.token}`;
		if (fetchCtx.host) { h.host = fetchCtx.host; h['x-forwarded-host'] = fetchCtx.host; }
		if (fetchCtx.surface) h['x-jc-surface'] = fetchCtx.surface;
		return h;
	}

	async api<T>(path: string): Promise<T | null> {
		const fetchCtx = this.args.fetchCtx();
		if (!fetchCtx) return null;
		try {
			const res = await fetchCtx.fetch(buildApiUrl(path, fetchCtx.apiBase), { headers: this.headers() });
			if (!res.ok) return null;
			return (await res.json()) as T;
		} catch {
			return null;
		}
	}

	flag(code: string): string {
		if (!code || code.length !== 2) return '';
		return String.fromCodePoint(...[...code.toUpperCase()].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
	}

	centroid(code: string): [number, number] | null {
		const c = countryCentroid(code);
		if (!c) return null;
		return [((c[1] + 180) / 360) * VB_W, ((90 - c[0]) / 180) * VB_H];
	}

	bandColorFor(region: string, subregion: string): string {
		const key = this.P.region_bands?.[subregion] ?? this.P.region_bands?.[region];
		return this.P.col_bands?.find((b) => b.key === key)?.color ?? FALLBACK_FILL;
	}

	categoryColor(key?: string): string | null {
		return this.P.categories?.find((c) => c.key === key)?.color ?? null;
	}

	firstSentence(s?: string): string | undefined {
		if (!s) return undefined;
		const cut = s.split(/[.!?]/)[0]?.trim();
		if (!cut) return undefined;
		return cut.length > 64 ? cut.slice(0, 61).trimEnd() + '…' : cut;
	}

	toPin(country: string, label: string, tagline?: string, category?: string, href?: string, pos?: [number, number]): Pin | null {
		const code = country.toUpperCase();
		const c = pos ?? this.centroid(code);
		if (!c) return null;
		const meta = countryByCode.get(code);
		return {
			country: code, label, tagline, category, href,
			x: c[0], y: c[1],
			color: this.categoryColor(category) ?? this.bandColorFor(meta?.region ?? '', meta?.subregion ?? ''),
			flag: this.flag(code),
		};
	}

	/** Resolve a view's source key — shared by the viewPins $effect and basePins. */
	viewSource(selView: string): string | undefined {
		return this.P.views?.find((v) => v.key === selView)?.source;
	}

	/** University pins for the 'universities' view — called by the parent's effect. */
	async loadUniversityPins(): Promise<Pin[]> {
		const json = await this.api<{ data: Array<{ country?: string; country_code?: string }> }>('/universities?page_size=100');
		const byCountry = new Map<string, number>();
		for (const u of json?.data ?? []) {
			const code = (u.country_code ?? nameToCode.get((u.country ?? '').toLowerCase()) ?? '').toUpperCase();
			if (code) byCountry.set(code, (byCountry.get(code) ?? 0) + 1);
		}
		return [...byCountry.entries()]
			.map(([code, n]) => this.toPin(code, countryByCode.get(code)?.name ?? code, `${n} ${n === 1 ? 'university' : 'universities'}`, 'study', '/universities'))
			.filter((p): p is Pin => p !== null);
	}

	/** COL-city pins for the 'col_cities' view — called by the parent's effect. */
	async loadColCityPins(): Promise<Pin[]> {
		const json = await this.api<{ data: { cities: Array<{ description?: string }> } }>('/calculator/cost-of-living/cities?page_size=60');
		const byCountry = new Map<string, number>();
		for (const c of json?.data?.cities ?? []) {
			// Descriptions are seeded as "City, Country — …" — the tail resolves the country.
			const name = (c.description ?? '').split(/[—–-]/)[0]?.split(',').pop()?.trim().toLowerCase() ?? '';
			const code = nameToCode.get(name);
			if (code) byCountry.set(code, (byCountry.get(code) ?? 0) + 1);
		}
		return [...byCountry.entries()]
			.map(([code, n]) => this.toPin(code, countryByCode.get(code)?.name ?? code, `${n} ${n === 1 ? 'city' : 'cities'} tracked`, 'affordable', '/tools/cost-of-living'))
			.filter((p): p is Pin => p !== null);
	}

	// ── Pins ─────────────────────────────────────────────────────
	basePins = $derived.by<Pin[]>(() => {
		const view = this.P.views?.find((v) => v.key === this.selView);
		if (view && view.source !== 'countries') return this.viewPins;
		if (this.P.geo_source === 'manual' || this.apiCountries.length === 0) {
			return (this.P.pins ?? [])
				.map((p) =>
					this.toPin(
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
		return [...this.apiCountries]
			.sort((a, b) => (b.member_count ?? 0) + (b.follower_count ?? 0) - ((a.member_count ?? 0) + (a.follower_count ?? 0)))
			.slice(0, this.P.pin_limit ?? 8)
			.map((s) => this.toPin(s.geo_code ?? '', s.name, this.firstSentence(s.description), undefined, `/spaces/${s.slug}`))
			.filter((p): p is Pin => p !== null);
	});

	visiblePins = $derived.by<Pin[]>(() =>
		this.basePins.filter((p) => {
			if (this.selRegion && countryByCode.get(p.country)?.region !== this.selRegion) return false;
			if (this.selCategory && p.category !== this.selCategory) return false;
			return true;
		})
	);

	// ── Regions ──────────────────────────────────────────────────
	regionOptions = $derived.by<string[]>(() => {
		const known = this.apiRegions.filter((r) => REGION_BOUNDS[r]);
		return known.length ? known : Object.keys(REGION_BOUNDS);
	});

	// ── Zoom / pan ───────────────────────────────────────────────
	clampVb(v: [number, number, number, number]): [number, number, number, number] {
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

	setVb(target: [number, number, number, number]) {
		const t = this.clampVb(target);
		if (reducedMotion) { this.vb = t; return; }
		const id = ++this.tweenId;
		const from = [...this.vb] as [number, number, number, number];
		const start = performance.now();
		const dur = 260;
		const step = (now: number) => {
			if (id !== this.tweenId) return;
			const k = Math.min(1, (now - start) / dur);
			const e = 1 - Math.pow(1 - k, 3);
			this.vb = [from[0] + (t[0] - from[0]) * e, from[1] + (t[1] - from[1]) * e, from[2] + (t[2] - from[2]) * e, from[3] + (t[3] - from[3]) * e];
			if (k < 1) requestAnimationFrame(step);
		};
		requestAnimationFrame(step);
	}

	zoomBy(f: number) {
		const [x, y, w, h] = this.vb;
		const nw = w * f;
		const nh = h * f;
		this.setVb([x + (w - nw) / 2, y + (h - nh) / 2, nw, nh]);
	}

	fitBounds(b: [number, number, number, number]) {
		const [x0, y0, x1, y1] = b;
		let bw = x1 - x0;
		let bh = y1 - y0;
		if (bw / bh > 2) bh = bw / 2; else bw = bh * 2;
		this.setVb([x0 + (x1 - x0 - bw) / 2, y0 + (y1 - y0 - bh) / 2, bw, bh]);
	}

	fitRegion(region: string) {
		const b = REGION_BOUNDS[region];
		if (!b) { this.setVb([0, 0, VB_W, VB_H]); return; }
		this.fitBounds(b);
	}

	selectRegion(name: string) {
		this.selRegion = this.selRegion === name ? '' : name;
		if (this.selRegion) this.fitRegion(this.selRegion); else this.setVb([0, 0, VB_W, VB_H]);
	}

	resetView() {
		this.selRegion = '';
		this.setVb([0, 0, VB_W, VB_H]);
	}

	onDown(e: PointerEvent) {
		this.panning = { px: e.clientX, py: e.clientY, vb: [...this.vb] as [number, number, number, number], moved: false };
		(e.currentTarget as Element).setPointerCapture(e.pointerId);
		// Touch taps never fire a pre-click pointermove — surface the pin card on down.
		if (this.svgEl) this.hoverAt(e, this.svgEl.getBoundingClientRect());
	}

	/** Screen-space position of a pin under preserveAspectRatio — "meet"
	 * letterboxes (min scale), cover_mode "slice" crops (max scale). */
	pinScreen(p: Pin, rect: DOMRect): { x: number; y: number } {
		const s = this.P.cover_mode
			? Math.max(rect.width / this.vb[2], rect.height / this.vb[3])
			: Math.min(rect.width / this.vb[2], rect.height / this.vb[3]);
		const offX = (rect.width - this.vb[2] * s) / 2;
		const offY = (rect.height - this.vb[3] * s) / 2;
		return { x: (p.x - this.vb[0]) * s + offX + rect.left, y: (p.y - this.vb[1]) * s + offY + rect.top };
	}

	hoverAt(e: PointerEvent, rect: DOMRect) {
		let best = -1;
		let bestD = 18; // px hit radius
		for (let i = 0; i < this.visiblePins.length; i++) {
			const pt = this.pinScreen(this.visiblePins[i], rect);
			const d = Math.hypot(pt.x - e.clientX, pt.y - e.clientY);
			if (d < bestD) { bestD = d; best = i; }
		}
		this.hoverIdx = best;
	}

	onMove(e: PointerEvent) {
		if (!this.svgEl) return;
		const rect = this.svgEl.getBoundingClientRect();
		if (this.panning) {
			const scale = this.panning.vb[2] / rect.width;
			if (Math.abs(e.clientX - this.panning.px) + Math.abs(e.clientY - this.panning.py) > 4) this.panning.moved = true;
			if (this.panning.moved) {
				this.tweenId++; // cancel any in-flight zoom tween while dragging
				this.vb = this.clampVb([this.panning.vb[0] - (e.clientX - this.panning.px) * scale, this.panning.vb[1] - (e.clientY - this.panning.py) * scale, this.panning.vb[2], this.panning.vb[3]]);
				return;
			}
		}
		this.hoverAt(e, rect);
	}

	onUp() {
		if (this.panning?.moved) {
			this.justDragged = true;
			setTimeout(() => (this.justDragged = false), 60);
		}
		this.panning = null;
	}

	onLeave() { this.panning = null; this.hoverIdx = -1; }

	// A drag shouldn't fire pin-link clicks on release.
	onClickCapture(e: MouseEvent) {
		if (this.justDragged) {
			this.justDragged = false;
			e.preventDefault();
			e.stopPropagation();
		}
	}

	// ── Currency ─────────────────────────────────────────────────
	rate = $derived(this.fxRates[this.currency] ?? 1);
	symbol = $derived(this.currencies.find((c) => c.code === this.currency)?.symbol ?? (this.currency === 'USD' ? '$' : ''));

	fmtMoney(n: number): string {
		const v = n * this.rate;
		return v >= 10000
			? new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 }).format(v)
			: new Intl.NumberFormat('en', { maximumFractionDigits: 0 }).format(v);
	}

	bandRange(min: number, max: number | null): string {
		return max === null ? `${this.symbol}${this.fmtMoney(min)}+` : `${this.symbol}${this.fmtMoney(min)} – ${this.symbol}${this.fmtMoney(max)}`;
	}

	statValue(source: string, value?: string, suffix?: string): string {
		const n = this.statCounts[source];
		if (n != null) return `${n.toLocaleString()}${suffix ?? ''}`;
		return value ?? '—';
	}

	iconFor(name?: string): string | null {
		return name ?? null;
	}

	get visitorCode(): string {
		return this.args.geoCountry()?.toUpperCase() ?? '';
	}
}

export function setExploreMapCtx(ctx: ExploreMapContext) {
	setContext(EXPLORE_MAP_CTX, ctx);
}
export function getExploreMapCtx(): ExploreMapContext | undefined {
	return getContext<ExploreMapContext>(EXPLORE_MAP_CTX);
}

// Child layout helpers live in ./exploreMapChildren.ts (plain .ts — keeps them
// unit-testable without pulling lucide-svelte into the test runtime).
export { exploreMapChildVisible, partitionExploreMapChildren, defaultExploreMapChildren } from './exploreMapChildren';
