import { describe, it, expect } from 'vitest';
import { resolveAiPageContext } from './pageContext';

describe('resolveAiPageContext', () => {
	it('wiki page → wikipage anchor with full context', () => {
		const ctx = resolveAiPageContext('/wiki/toronto/housing', {
			page: { id: 'p1', space_id: 's1', slug: 'housing', title: 'Housing', category: 'guides' },
			spaceDetail: { id: 's1', slug: 'toronto', name: 'Toronto' }
		});
		expect(ctx?.anchor).toBe('wikipage:p1');
		expect(ctx?.context).toMatchObject({
			content_type: 'wiki_page',
			content_id: 'p1',
			space_id: 's1',
			slug: 'housing',
			title: 'Housing'
		});
	});
	it('wiki space index → space anchor', () => {
		const ctx = resolveAiPageContext('/wiki/toronto', {
			spaceDetail: { id: 's1', slug: 'toronto', name: 'Toronto' }
		});
		expect(ctx?.anchor).toBe('space:s1');
	});
	it('blog post → content anchor', () => {
		const ctx = resolveAiPageContext('/blog/moving-to-canada', {
			article: { id: 'c9', slug: 'moving-to-canada', title: 'Moving', content_type: 'post' }
		});
		expect(ctx?.anchor).toBe('content:c9');
		expect(ctx?.context).toMatchObject({ content_type: 'post', content_id: 'c9' });
	});
	it('unknown path → path anchor with title only', () => {
		const ctx = resolveAiPageContext('/forum', { title: 'Forum' });
		expect(ctx?.anchor).toBe('path:main:/forum');
	});
	it('entity path with missing data → path anchor', () => {
		const ctx = resolveAiPageContext('/blog/moving-to-canada', {});
		expect(ctx?.anchor).toBe('path:main:/blog/moving-to-canada');
	});
});
