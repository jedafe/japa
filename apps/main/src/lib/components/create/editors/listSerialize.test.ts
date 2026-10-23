import { describe, it, expect } from 'vitest';
import { displayNumber, serializeListBodyHtml } from './listSerialize';
import type { ListItem } from '$lib/types';

const item = (over: Partial<ListItem> = {}): ListItem => ({ heading: 'Item', ...over });

describe('displayNumber', () => {
	it('numbers ascending for numbered mode', () => {
		expect(displayNumber('numbered', 0, 3)).toBe(1);
		expect(displayNumber('numbered', 2, 3)).toBe(3);
	});
	it('numbers ascending for unnumbered mode', () => {
		expect(displayNumber('unnumbered', 1, 3)).toBe(2);
	});
	it('counts down for countdown mode', () => {
		expect(displayNumber('countdown', 0, 3)).toBe(3);
		expect(displayNumber('countdown', 2, 3)).toBe(1);
	});
});

describe('serializeListBodyHtml', () => {
	it('numbers items 1..3 in numbered mode with positional ids', () => {
		const html = serializeListBodyHtml(
			'numbered',
			'',
			[item({ heading: 'A' }), item({ heading: 'B' }), item({ heading: 'C' })],
			{}
		);
		expect(html).toContain('<div class="list-item" id="item-1">');
		expect(html).toContain('<div class="list-item" id="item-2">');
		expect(html).toContain('<div class="list-item" id="item-3">');
		expect(html.indexOf('id="item-1"')).toBeLessThan(html.indexOf('id="item-2"'));
		expect(html.indexOf('id="item-2"')).toBeLessThan(html.indexOf('id="item-3"'));
		expect(html).toContain('<span class="li-num">1.</span>');
		expect(html).toContain('<span class="li-num">2.</span>');
		expect(html).toContain('<span class="li-num">3.</span>');
	});

	it('reverses baked numbers in countdown mode', () => {
		const html = serializeListBodyHtml(
			'countdown',
			'',
			[item({ heading: 'A' }), item({ heading: 'B' }), item({ heading: 'C' })],
			{}
		);
		const first = html.indexOf('id="item-1"');
		const last = html.indexOf('id="item-3"');
		expect(html.slice(first)).toContain('<span class="li-num">3.</span>');
		expect(html.slice(last)).toContain('<span class="li-num">1.</span>');
	});

	it('emits no li-num spans in unnumbered mode', () => {
		const html = serializeListBodyHtml('unnumbered', '', [item({ heading: 'A' })], {});
		expect(html).not.toContain('li-num');
		expect(html).toContain('<h3>A</h3>');
	});

	it('renders a content embed placeholder with the snapshot card inside', () => {
		const html = serializeListBodyHtml('numbered', '', [
			item({
				embed: {
					kind: 'content',
					content_id: 'cid-1',
					content_slug: 'visa-guide',
					snapshot: {
						title: 'Visa Guide',
						excerpt: 'All about visas',
						image_url: 'https://cdn.example.com/v.png',
						content_type: 'guide'
					}
				}
			})
		], {});
		expect(html).toContain('data-type="content-embed"');
		expect(html).toContain('data-content-slug="visa-guide"');
		expect(html).toContain('href="/guide/visa-guide"');
		expect(html).toContain('Visa Guide');
		expect(html).toContain('All about visas');
		expect(html).toContain('https://cdn.example.com/v.png');
	});

	it('renders a url embed as a link-preview placeholder', () => {
		const html = serializeListBodyHtml('numbered', '', [
			item({ embed: { kind: 'url', url: 'https://example.com/article' } })
		], {});
		expect(html).toContain('data-type="link-preview"');
		expect(html).toContain('data-url="https://example.com/article"');
	});

	it('renders an iframe when the url embed resolved an embed_url', () => {
		const html = serializeListBodyHtml('numbered', '', [
			item({
				embed: {
					kind: 'url',
					url: 'https://example.com/article',
					resolved: { embed_url: 'https://www.youtube.com/embed/abc' }
				}
			})
		], {});
		expect(html).toContain('<iframe src="https://www.youtube.com/embed/abc"');
		expect(html).not.toContain('link-preview');
	});

	it('renders a video embed placeholder with the thumbnail poster', () => {
		const html = serializeListBodyHtml('numbered', '', [
			item({
				embed: {
					kind: 'video',
					video_source_id: 'vs-42',
					resolved: { image_url: 'https://cdn.example.com/thumb.jpg' }
				}
			})
		], {});
		expect(html).toContain('<div class="video-embed" data-video-source-id="vs-42">');
		expect(html).toContain('<img src="https://cdn.example.com/thumb.jpg"');
	});

	it('renders a media_id image through mediaUrls without a loading attr', () => {
		const html = serializeListBodyHtml(
			'numbered',
			'',
			[item({ media_id: 'm-9' })],
			{ 'm-9': 'https://cdn.example.com/pic.jpg' }
		);
		expect(html).toContain('<img src="https://cdn.example.com/pic.jpg" alt="">');
		expect(html).not.toContain('loading=');
	});

	it('renders source_url + credit as a linked li-credit line', () => {
		const html = serializeListBodyHtml('numbered', '', [
			item({ source_url: 'https://src.example.com', credit: 'Jane Doe' })
		], {});
		expect(html).toContain('<p class="li-credit">');
		expect(html).toContain('<a href="https://src.example.com">Jane Doe</a>');
	});

	it('renders credit without a link when source_url is absent', () => {
		const html = serializeListBodyHtml('numbered', '', [item({ credit: 'Jane Doe' })], {});
		expect(html).toContain('<p class="li-credit">Jane Doe</p>');
		expect(html).not.toContain('<a href');
	});

	it('escapes heading text but embeds item body_html raw', () => {
		const html = serializeListBodyHtml('numbered', '', [
			item({ heading: 'Fish & <Chips> "Deluxe"', body_html: '<p>raw <em>markup</em></p>' })
		], {});
		expect(html).toContain('Fish &amp; &lt;Chips&gt; &quot;Deluxe&quot;');
		expect(html).toContain('<p>raw <em>markup</em></p>');
	});

	it('emits intro_html before the first list-item', () => {
		const html = serializeListBodyHtml('numbered', '<p>Intro here</p>', [item()], {});
		expect(html.indexOf('<p>Intro here</p>')).toBe(0);
		expect(html.indexOf('<p>Intro here</p>')).toBeLessThan(html.indexOf('class="list-item"'));
	});

	it('serializes an empty list to just the intro', () => {
		expect(serializeListBodyHtml('numbered', '<p>Only intro</p>', [], {})).toBe('<p>Only intro</p>');
		expect(serializeListBodyHtml('numbered', '', [], {})).toBe('');
	});

	it('drops markup-free empty paragraphs from intro and item bodies', () => {
		const html = serializeListBodyHtml('numbered', '<p></p>', [
			item({ heading: 'A', body_html: '<p><br></p>' }),
			item({ heading: 'B', body_html: '<p>real</p>' })
		], {});
		expect(html.startsWith('<p></p>')).toBe(false);
		expect(html).toContain('id="item-1"><h3>');
		expect(html).toContain('<p>real</p>');
	});

	it('links the content-embed card via the generic route when the snapshot lacks content_type', () => {
		const html = serializeListBodyHtml('numbered', '', [
			item({ embed: { kind: 'content', content_slug: 'x-post', snapshot: { title: 'X' } } })
		], {});
		expect(html).toContain('href="/content/x-post"');
	});
});
