import { describe, it, expect } from 'vitest';
import { sanitizeSidebarHtml, safeHref, safeJsonLdScript } from './safeHtml';

describe('safeHtml', () => {
	describe('sanitizeSidebarHtml', () => {
		it('keeps allowed structural tags', () => {
			const html = '<p>Hello <strong>world</strong></p>';
			expect(sanitizeSidebarHtml(html)).toBe('<p>Hello <strong>world</strong></p>');
		});

		it('removes <script> blocks and on* handlers', () => {
			const html =
				'<p onclick="alert(1)">x</p><script>alert(2)</script><p>safe</p>';
			const out = sanitizeSidebarHtml(html);
			expect(out).not.toContain('<script>');
			expect(out).not.toContain('onclick');
			expect(out).toContain('<p>safe</p>');
		});

		it('strips dangerous href schemes', () => {
			const html =
				'<a href="https://example.com">ok</a><a href="javascript:alert(1)">bad</a>';
			const out = sanitizeSidebarHtml(html);
			expect(out).toContain('href="https://example.com"');
			expect(out).not.toContain('javascript:');
		});
	});

	describe('safeHref', () => {
		it('allows http/https/relative URLs', () => {
			expect(safeHref('https://example.com')).toBe('https://example.com');
			expect(safeHref('/docs')).toBe('/docs');
			expect(safeHref('#section')).toBe('#section');
		});

		it('rejects dangerous schemes', () => {
			expect(safeHref('javascript:alert(1)')).toBe('');
			expect(safeHref('data:text/html,<script>')).toBe('');
		});
	});

	describe('safeJsonLdScript', () => {
		it('escapes </script> in JSON-LD payloads', () => {
			const payload = { title: '</script><script>alert(1)</script>' };
			const out = safeJsonLdScript(payload);
			// The JSON itself must not contain an unescaped </script> sequence;
			// the wrapper's own closing tag is the only literal </script> allowed.
			const jsonBody = out
				.replace('<script type="application/ld+json">', '')
				.replace('</script>', '');
			expect(jsonBody).toContain('\\u003c/script\\u003e');
			expect(jsonBody).not.toContain('</script>');
			expect(out.startsWith('<script type="application/ld+json">')).toBe(true);
		});

		it('returns empty string for null/undefined', () => {
			expect(safeJsonLdScript(null)).toBe('');
			expect(safeJsonLdScript(undefined)).toBe('');
		});
	});
});
