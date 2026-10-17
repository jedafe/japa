#!/usr/bin/env bun
// Generates packages/landing-blocks/src/geo/worldMapData.ts
// Sources (fetched once at generation time, committed output — no runtime deps):
//   - Natural Earth country geometries (johan/world.geo.json, ISO_A3 keyed)
//   - UN region/subregion + ISO alpha-2 metadata (mledoze world-countries)
// Projection: equirectangular, viewBox 1000x500 — identical plane to the
// countryCentroid pin projection (x = (lng+180)/360*1000, y = (90-lat)/180*500).
// Usage: bun scripts/gen-world-map.mjs

const GEO_URL = 'https://raw.githubusercontent.com/johan/world.geo.json/master/countries.geo.json';
const META_URL = 'https://cdn.jsdelivr.net/npm/world-countries@5/countries.json';
const ADMIN1_URL =
	'https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_10m_admin_1_states_provinces.geojson';
const CITIES_URL = 'https://download.geonames.org/export/dump/cities15000.zip';
const OUT = new URL('../packages/landing-blocks/src/geo/worldMapData.ts', import.meta.url).pathname;
const OUT_ADMIN1 = new URL('../packages/landing-blocks/src/geo/worldAdmin1.ts', import.meta.url).pathname;
const OUT_CITIES = new URL('../packages/landing-blocks/src/geo/worldCities.ts', import.meta.url).pathname;

const W = 1000;
const H = 500;
const px = (lng) => ((lng + 180) / 360) * W;
const py = (lat) => ((90 - lat) / 180) * H;
const r = (n) => Math.round(n * 10) / 10;

const [geo, meta, admin1, citiesZip] = await Promise.all([
	fetch(GEO_URL).then((r) => r.json()),
	fetch(META_URL).then((r) => r.json()),
	fetch(ADMIN1_URL).then((r) => r.json()),
	fetch(CITIES_URL).then((r) => r.arrayBuffer()),
]);

const byA3 = new Map(meta.map((c) => [c.cca3, c]));
// Manual fixes for ids the GeoJSON carries that the meta set lacks/renames.
const A3_FIX = {
	'-99': null, // Northern Cyprus / Somaliland — not in UN meta
	'CS-KM': null,
	XKX: { cca2: 'XK', region: 'Europe', subregion: 'Southeast Europe', name: { common: 'Kosovo' } },
};
const DROP = new Set(['ATA']); // Antarctica — off the mockup

/** Un-wrap a ring that crosses the antimeridian (verts on both ±180 sides)
 * by translating negative-lng vertices +360 so it draws contiguously at the
 * right edge instead of streaking across the map. */
function unwrapRing(ring) {
	const lngs = ring.map((p) => p[0]);
	const min = Math.min(...lngs);
	const max = Math.max(...lngs);
	if (max - min <= 300) return ring;
	return ring.map(([lng, lat]) => [lng < 0 ? lng + 360 : lng, lat]);
}

function ringToPath(ring) {
	const rr = unwrapRing(ring);
	let d = '';
	for (let i = 0; i < rr.length; i++) d += `${i === 0 ? 'M' : 'L'}${r(px(rr[i][0]))} ${r(py(rr[i][1]))}`;
	return d + 'Z';
}

/** Shoelace area of a ring in projected space — used to pick each country's
 * mainland polygon for region bounds (excludes overseas territories). */
function ringArea(ring) {
	let a = 0;
	for (let i = 0; i < ring.length - 1; i++) {
		a += px(ring[i][0]) * py(ring[i + 1][1]) - px(ring[i + 1][0]) * py(ring[i][1]);
	}
	return Math.abs(a / 2);
}

const countries = [];
const skipped = [];

for (const f of geo.features) {
	const a3 = f.id;
	if (DROP.has(a3)) continue;
	const m = byA3.get(a3) ?? A3_FIX[a3];
	if (m === null || m === undefined) {
		skipped.push(`${a3} ${f.properties?.name ?? '?'}`);
		continue;
	}
	const polys = f.geometry.type === 'Polygon' ? [f.geometry.coordinates] : f.geometry.coordinates;
	const d = polys.flat().map(ringToPath).join(' ');
	// Largest polygon by exterior-ring area → region-bound anchor.
	let best = null;
	let bestArea = 0;
	for (const poly of polys) {
		const a = ringArea(poly[0]);
		if (a > bestArea) {
			bestArea = a;
			best = poly;
		}
	}
	countries.push({
		code: m.cca2,
		a3,
		name: m.name?.common ?? f.properties?.name ?? '',
		region: m.region ?? 'Other',
		subregion: m.subregion ?? '',
		d,
		_main: best,
	});
}

// Per-region AND per-country bounds from mainland polygons only (padded),
// for zoom targets — regions for the explorer filter, countries for
// state-level wiki pages ("state inside its country" view).
const bounds = {};
const countryBounds = {};
for (const c of countries) {
	for (const ring of c._main ?? []) {
		const rr = unwrapRing(ring);
		for (const [lng, lat] of rr) {
			const b = (bounds[c.region] ??= [W, H, 0, 0]);
			b[0] = Math.min(b[0], px(lng));
			b[1] = Math.min(b[1], py(lat));
			b[2] = Math.max(b[2], px(lng));
			b[3] = Math.max(b[3], py(lat));
			const cb = (countryBounds[c.code] ??= [W, H, 0, 0]);
			cb[0] = Math.min(cb[0], px(lng));
			cb[1] = Math.min(cb[1], py(lat));
			cb[2] = Math.max(cb[2], px(lng));
			cb[3] = Math.max(cb[3], py(lat));
		}
	}
}
for (const k of Object.keys(bounds)) {
	const b = bounds[k];
	bounds[k] = [Math.max(0, r(b[0] - 20)), Math.max(0, r(b[1] - 20)), Math.min(W, r(b[2] + 20)), Math.min(H, r(b[3] + 20))];
}
for (const k of Object.keys(countryBounds)) {
	const b = countryBounds[k];
	countryBounds[k] = [Math.max(0, r(b[0] - 8)), Math.max(0, r(b[1] - 8)), Math.min(W, r(b[2] + 8)), Math.min(H, r(b[3] + 8))];
}

const out = countries.map(({ _main, ...c }) => c);
const ts = `// GENERATED by scripts/gen-world-map.mjs — do not edit by hand.
// Re-run: bun scripts/gen-world-map.mjs
export interface WorldCountry {
  code: string;      // ISO alpha-2
  a3: string;        // ISO alpha-3
  name: string;
  region: string;    // UN region
  subregion: string; // UN subregion
  d: string;         // SVG path, equirectangular projection, viewBox 1000x500
}
export const WORLD_VIEWBOX = { w: ${W}, h: ${H} };
export const WORLD_COUNTRIES: WorldCountry[] = ${JSON.stringify(out)};
export const REGION_BOUNDS: Record<string, [number, number, number, number]> = ${JSON.stringify(bounds)};
// Per-country mainland-polygon bounds [x0,y0,x1,y1] — zoom targets for
// subnational contexts (e.g. state wiki pages fit the parent country).
export const COUNTRY_BOUNDS: Record<string, [number, number, number, number]> = ${JSON.stringify(countryBounds)};
`;

await Bun.write(OUT, ts);
console.log(`wrote ${out.length} countries → ${OUT} (${(ts.length / 1024).toFixed(0)}KB)`);
if (skipped.length) console.log('skipped:', skipped.join(', '));
console.log('region bounds:', JSON.stringify(bounds));

// ── Admin-1 (state/province) centroids, keyed by ISO-3166-2 ("NG-LA") ────
// Used to pin subnational spaces at their real position instead of the
// parent-country centroid. Only codes matching /^[A-Z]{2}-/ are kept —
// NE uses "-99-X##~" for unrecognised units.
function ringCentroid(ring) {
	const rr = unwrapRing(ring);
	let area = 0;
	let cx = 0;
	let cy = 0;
	for (let i = 0; i < rr.length - 1; i++) {
		const x0 = px(rr[i][0]);
		const y0 = py(rr[i][1]);
		const x1 = px(rr[i + 1][0]);
		const y1 = py(rr[i + 1][1]);
		const cross = x0 * y1 - x1 * y0;
		area += cross;
		cx += (x0 + x1) * cross;
		cy += (y0 + y1) * cross;
	}
	if (Math.abs(area) < 1e-9) {
		// Degenerate ring — fall back to vertex mean.
		const mx = rr.reduce((s, p) => s + px(p[0]), 0) / rr.length;
		const my = rr.reduce((s, p) => s + py(p[1]), 0) / rr.length;
		return [r(mx), r(my)];
	}
	return [r(cx / (3 * area)), r(cy / (3 * area))];
}

/** Douglas-Peucker simplification in projected space — keeps admin-1
 * outlines crisp at state-zoom while cutting path data ~80%. */
function simplifyRing(ring, tol) {
	const pts = unwrapRing(ring).map(([lng, lat]) => [px(lng), py(lat)]);
	if (pts.length <= 4) return pts;
	const keep = new Uint8Array(pts.length);
	keep[0] = keep[pts.length - 1] = 1;
	const stack = [[0, pts.length - 1]];
	while (stack.length) {
		const [a, b] = stack.pop();
		let maxD = 0, idx = -1;
		const [ax, ay] = pts[a], [bx, by] = pts[b];
		const dx = bx - ax, dy = by - ay;
		const len = Math.hypot(dx, dy);
		// Closed rings share first/last points — a zero-length chord makes the
		// perpendicular formula degenerate to 0; fall back to point distance.
		const degenerate = len < 1e-6;
		for (let i = a + 1; i < b; i++) {
			const d = degenerate
				? Math.hypot(pts[i][0] - ax, pts[i][1] - ay)
				: Math.abs(dy * pts[i][0] - dx * pts[i][1] + bx * ay - by * ax) / len;
			if (d > maxD) { maxD = d; idx = i; }
		}
		if (maxD > tol && idx > 0) {
			keep[idx] = 1;
			stack.push([a, idx], [idx, b]);
		}
	}
	return pts.filter((_, i) => keep[i]);
}

function ringToSimplifiedPath(ring, tol) {
	// Adaptive tolerance: small units (city LGAs ~0.1 units wide) would
	// collapse to 2 points at 0.4 — scale tolerance to the ring's extent.
	const unwrapped = unwrapRing(ring);
	let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
	for (const [lng, lat] of unwrapped) {
		const x = px(lng), y = py(lat);
		if (x < minX) minX = x; if (x > maxX) maxX = x;
		if (y < minY) minY = y; if (y > maxY) maxY = y;
	}
	const span = Math.max(maxX - minX, maxY - minY);
	const tolEff = Math.min(tol, Math.max(0.03, span * 0.04));
	const pts = simplifyRing(ring, tolEff);
	let d = '';
	for (let i = 0; i < pts.length; i++) d += `${i === 0 ? 'M' : 'L'}${r(pts[i][0])} ${r(pts[i][1])}`;
	return d + 'Z';
}

const ADMIN1_SIMPLIFY = 0.4; // projected units (~0.14°) — invisible at these zooms

const admin1Out = {};
let admin1Skipped = 0;
for (const f of admin1.features) {
	const code = String(f.properties?.iso_3166_2 ?? '');
	if (!/^[A-Z]{2}-[A-Z0-9]+$/.test(code)) {
		admin1Skipped++;
		continue;
	}
	const polys = f.geometry.type === 'Polygon' ? [f.geometry.coordinates] : f.geometry.coordinates;
	let best = null;
	let bestArea = 0;
	for (const poly of polys) {
		const a = ringArea(poly[0]);
		if (a > bestArea) {
			bestArea = a;
			best = poly;
		}
	}
	if (!best) {
		admin1Skipped++;
		continue;
	}
	const [x, y] = ringCentroid(best[0]);
	// Bounds of the largest exterior ring — zoom target for city-level wiki
	// pages ("city inside its state" view).
	const rr = unwrapRing(best[0]);
	let bx0 = W, by0 = H, bx1 = 0, by1 = 0;
	for (const [lng, lat] of rr) {
		bx0 = Math.min(bx0, px(lng));
		by0 = Math.min(by0, py(lat));
		bx1 = Math.max(bx1, px(lng));
		by1 = Math.max(by1, py(lat));
	}
	admin1Out[code] = {
		x, y,
		name: f.properties?.name ?? '',
		b: [r(bx0 - 8), r(by0 - 8), r(bx1 + 8), r(by1 + 8)],
		// Simplified SVG path — drawn as a highlighted breakout shape on
		// subnational wiki maps (state outline inside its country).
		d: polys.flat().map((ring) => ringToSimplifiedPath(ring, ADMIN1_SIMPLIFY)).join(' '),
	};
}

const admin1Ts = `// GENERATED by scripts/gen-world-map.mjs — do not edit by hand.
// Re-run: bun scripts/gen-world-map.mjs
// Admin-1 (state/province) polygon centroids from Natural Earth 10m,
// keyed by ISO-3166-2 code (e.g. "NG-LA"). x/y are in the same
// equirectangular viewBox (1000x500) as worldMapData.
export interface Admin1Centroid {
  x: number;
  y: number;
  name: string;
  /** Largest-ring bounds [x0,y0,x1,y1] — zoom target for city-level views. */
  b: [number, number, number, number];
  /** Simplified SVG path (DP ~0.4 units) — highlighted breakout shape. */
  d: string;
}
export const ADMIN1_CENTROIDS: Record<string, Admin1Centroid> = ${JSON.stringify(admin1Out)};
`;

await Bun.write(OUT_ADMIN1, admin1Ts);
console.log(`wrote ${Object.keys(admin1Out).length} admin-1 centroids → ${OUT_ADMIN1} (${(admin1Ts.length / 1024).toFixed(0)}KB, skipped ${admin1Skipped})`);

// ── City positions (GeoNames cities15000, CC-BY 4.0 geonames.org) ────────
// Keyed "{cc}:{ascii-normalized name}" → {x,y}. On duplicate keys the
// highest-population row wins. Used to pin city-level wiki spaces at their
// real location instead of the parent-state centroid.
const { execSync } = await import('node:child_process');
const { mkdtempSync, writeFileSync } = await import('node:fs');
const { tmpdir } = await import('node:os');
const { join } = await import('node:path');
const tmpZip = join(mkdtempSync(join(tmpdir(), 'jcgeo-')), 'cities15000.zip');
writeFileSync(tmpZip, Buffer.from(citiesZip));
const citiesTxt = execSync(`unzip -p ${JSON.stringify(tmpZip)} cities15000.txt`, {
	maxBuffer: 128 * 1024 * 1024,
}).toString('utf8');

const norm = (s) =>
	s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().trim(); // U+0300–U+036F combining marks
const cityOut = {};
const cityPop = {};
for (const line of citiesTxt.split('\n')) {
	if (!line) continue;
	const f = line.split('\t');
	const cc = (f[8] ?? '').toLowerCase();
	const name = norm(f[2] ?? f[1] ?? ''); // asciiname → name fallback
	const lat = parseFloat(f[4]);
	const lng = parseFloat(f[5]);
	const pop = parseInt(f[14] ?? '0', 10) || 0;
	if (!cc || !name || !Number.isFinite(lat) || !Number.isFinite(lng)) continue;
	const key = `${cc}:${name}`;
	if ((cityPop[key] ?? -1) >= pop) continue;
	cityPop[key] = pop;
	cityOut[key] = { x: r(px(lng)), y: r(py(lat)) };
}

// ── City boundaries (geoBoundaries ADM2, CC-BY 4.0) ─────────────────────
// City wiki pages draw the city's own outline ("city inside its state")
// instead of the parent-state shape. Bounded to countries that actually
// have city-level spaces in the geo catalog — discover via JC_API_URL's
// /spaces?level=city (falls back to the core seed countries when the API
// is unreachable). Match key: "{cc}:{ascii-normalized lowercase name}".
const ADMIN2_ISO2 = (() => {
	const seed = new Set(['NG', 'GH', 'KE']); // geo-catalog countries seen seeded
	return async () => {
		try {
			const api = (process.env.JC_API_URL ?? 'http://localhost:8080/api/v1').replace(/\/$/, '');
			const res = await fetch(`${api}/spaces?level=city&page_size=100`, {
				signal: AbortSignal.timeout(5000),
			});
			const body = await res.json();
			for (const s of body?.data ?? []) {
				if (s.geo_code) seed.add(String(s.geo_code).toUpperCase());
			}
		} catch {
			console.warn('admin-2: API unreachable, using seed country set only');
		}
		return [...seed];
	};
})();

const admin2Out = {};
let admin2Countries = 0;
for (const cc of await ADMIN2_ISO2()) {
	const a3 = countries.find((c) => c.code === cc)?.a3;
	if (!a3) continue;
	try {
		const metaRes = await fetch(`https://www.geoboundaries.org/api/current/gbOpen/${a3}/ADM2/`, {
			signal: AbortSignal.timeout(10000),
		}).then((r) => r.json());
		const url = metaRes?.simplifiedGeometryGeoJSON;
		if (!url) continue;
		const adm2 = await fetch(url).then((r) => r.json());
		for (const f of adm2.features ?? []) {
			const name = norm(f.properties?.shapeName ?? '');
			if (!name || !f.geometry) continue;
			const polys =
				f.geometry.type === 'Polygon' ? [f.geometry.coordinates] : f.geometry.coordinates;
			const d = polys.flat().map((ring) => ringToSimplifiedPath(ring, ADMIN1_SIMPLIFY)).join(' ');
			if (!d) continue;
			// bounds from all exterior rings
			let bx0 = W, by0 = H, bx1 = 0, by1 = 0;
			for (const poly of polys) {
				for (const [lng, lat] of unwrapRing(poly[0])) {
					bx0 = Math.min(bx0, px(lng)); by0 = Math.min(by0, py(lat));
					bx1 = Math.max(bx1, px(lng)); by1 = Math.max(by1, py(lat));
				}
			}
			admin2Out[`${cc.toLowerCase()}:${name}`] = {
				d,
				b: [r(bx0 - 4), r(by0 - 4), r(bx1 + 4), r(by1 + 4)],
			};
		}
		admin2Countries++;
	} catch (e) {
		console.warn(`admin-2 fetch failed for ${cc}:`, e.message);
	}
}

const citiesTs = `// GENERATED by scripts/gen-world-map.mjs — do not edit by hand.
// Re-run: bun scripts/gen-world-map.mjs
// City positions from GeoNames cities15000 (CC-BY 4.0, geonames.org).
// Keyed "{alpha-2}:{ascii-normalized lowercase name}" (e.g. "ng:agege");
// x/y are in the same equirectangular viewBox (1000x500) as worldMapData.
export interface CityPosition {
  x: number;
  y: number;
}
export const CITY_POSITIONS: Record<string, CityPosition> = ${JSON.stringify(cityOut)};

// City-level boundary outlines (geoBoundaries ADM2, CC-BY 4.0) for countries
// that have seeded city spaces — keyed "{cc}:{normalized name}". Drawn as
// the breakout shape on city wiki pages; b = bounds [x0,y0,x1,y1].
export interface CityShape {
  d: string;
  b: [number, number, number, number];
}
export const CITY_SHAPES: Record<string, CityShape> = ${JSON.stringify(admin2Out)};
`;
await Bun.write(OUT_CITIES, citiesTs);
console.log(`wrote ${Object.keys(cityOut).length} city positions + ${Object.keys(admin2Out).length} city shapes (${admin2Countries} countries) → ${OUT_CITIES} (${(citiesTs.length / 1024).toFixed(0)}KB)`);
