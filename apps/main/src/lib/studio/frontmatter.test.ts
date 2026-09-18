import { describe, it, expect } from 'vitest';
import { parseFrontmatter, buildMarkdown, setFrontmatterField } from './frontmatter';
import { markdownToHtml } from './markdown-html';

describe('parseFrontmatter', () => {
	it('extracts frontmatter and body', () => {
		const text = '---\ntitle: "Hello"\nslug: "hello"\n---\n\n# Hello';
		const parsed = parseFrontmatter(text);
		expect(parsed.frontmatter).toEqual({ title: 'Hello', slug: 'hello' });
		expect(parsed.body.trim()).toBe('# Hello');
	});

	it('returns empty frontmatter and full text when no frontmatter exists', () => {
		const text = '# Hello';
		const parsed = parseFrontmatter(text);
		expect(parsed.frontmatter).toEqual({});
		expect(parsed.body).toBe('# Hello');
	});

	it('returns empty frontmatter and the full text when the YAML is malformed', () => {
		// Ported contract: a parse failure yields { frontmatter: {}, body: text }
		// — the unparseable block stays in the body rather than being dropped.
		const text = '---\n: [unclosed\n---\n\n# Hello';
		const parsed = parseFrontmatter(text);
		expect(parsed.frontmatter).toEqual({});
		expect(parsed.body).toBe(text);
	});
});

describe('buildMarkdown', () => {
	it('rebuilds markdown with frontmatter and body', () => {
		const result = buildMarkdown({ title: 'Hello' }, '# Hello');
		expect(result).toContain('---');
		expect(result).toContain('title: Hello');
		expect(result).toContain('# Hello');
	});

	it('round-trips through parseFrontmatter', () => {
		const fm = { title: 'Guide', slug: 'guide', tags: ['a', 'b'], status: 'draft' };
		const body = '# Guide\n\nBody text.';
		const parsed = parseFrontmatter(buildMarkdown(fm, body));
		expect(parsed.frontmatter).toEqual(fm);
		expect(parsed.body.trim()).toBe(body);
	});
});

describe('setFrontmatterField', () => {
	it('adds a field to existing frontmatter', () => {
		const text = '---\ntitle: "Hello"\n---\n\n# Hello';
		const updated = setFrontmatterField(text, 'status', 'in_review');
		const parsed = parseFrontmatter(updated);
		expect(parsed.frontmatter).toEqual({ title: 'Hello', status: 'in_review' });
	});
});

describe('markdownToHtml', () => {
	it('converts headings and paragraphs', () => {
		const html = markdownToHtml('# Title\n\nSome body.');
		expect(html).toContain('<h1');
		expect(html).toContain('>Title</h1>');
		expect(html).toContain('<p class="my-2">Some body.</p>');
	});

	it('converts links and bold text', () => {
		const html = markdownToHtml('[link](https://example.com) and **bold**.');
		expect(html).toContain('<a href="https://example.com"');
		expect(html).toContain('<strong>bold</strong>');
	});

	it('sanitizes dangerous HTML when requested', () => {
		const html = markdownToHtml('# Title\n\n<script>alert(1)</script>', { sanitize: true });
		expect(html).not.toContain('<script>');
		expect(html).toContain('>Title</h1>');
	});
});
