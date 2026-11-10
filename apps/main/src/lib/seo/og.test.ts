import { describe, it, expect } from 'vitest';
import { ogCardUrl, ogMediaUrl, toolOgCard } from './og';

const ORIGIN = 'https://japa.to';

describe('ogCardUrl', () => {
	it('builds the generated-card URL under the page origin', () => {
		expect(ogCardUrl(ORIGIN, 'tool:visa-routes')).toBe('https://japa.to/og/tool:visa-routes.png');
	});
});

describe('toolOgCard', () => {
	it('resolves the tool slug on index pages', () => {
		expect(toolOgCard(ORIGIN, '/tools/visa-routes')).toBe('https://japa.to/og/tool:visa-routes.png');
	});

	it('resolves through a locale prefix', () => {
		expect(toolOgCard(ORIGIN, '/fr/tools/visa-routes')).toBe('https://japa.to/og/tool:visa-routes.png');
	});

	it('shares the parent tool card on detail pages', () => {
		expect(toolOgCard(ORIGIN, '/tools/visa-routes/abc-123')).toBe('https://japa.to/og/tool:visa-routes.png');
	});

	it('falls back to the brand card key for /tools and non-tool paths', () => {
		expect(toolOgCard(ORIGIN, '/tools')).toBe('https://japa.to/og/tools.png');
		expect(toolOgCard(ORIGIN, '/wiki/canada')).toBe('https://japa.to/og/tools.png');
	});
});

describe('ogMediaUrl', () => {
	it('absolutizes origin-relative API media paths', () => {
		expect(ogMediaUrl(ORIGIN, '/api/v1/media/abc')).toBe('https://japa.to/api/v1/media/abc');
	});

	it('passes absolute and protocol-relative URLs through', () => {
		expect(ogMediaUrl(ORIGIN, 'https://cdn.japa.to/x.png')).toBe('https://cdn.japa.to/x.png');
		expect(ogMediaUrl(ORIGIN, '//cdn.japa.to/x.png')).toBe('//cdn.japa.to/x.png');
	});

	it('rejects SVG — social crawlers ignore it', () => {
		expect(ogMediaUrl(ORIGIN, '/flags/ca.svg')).toBe('');
		expect(ogMediaUrl(ORIGIN, 'https://cdn.japa.to/x.svg?v=2')).toBe('');
		expect(ogMediaUrl(ORIGIN, '/x.SVG')).toBe('');
	});

	it('rejects data: and blob: URLs — crawlers cannot fetch them', () => {
		expect(ogMediaUrl(ORIGIN, 'data:image/png;base64,iVBOR')).toBe('');
		expect(ogMediaUrl(ORIGIN, 'blob:https://japa.to/abc')).toBe('');
	});

	it('rejects empty, bare-relative, and missing input', () => {
		expect(ogMediaUrl(ORIGIN, '')).toBe('');
		expect(ogMediaUrl(ORIGIN, 'media/x.png')).toBe('');
		expect(ogMediaUrl(ORIGIN, null)).toBe('');
		expect(ogMediaUrl(ORIGIN, undefined)).toBe('');
	});
});
