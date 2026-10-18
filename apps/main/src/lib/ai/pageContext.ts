// §115 — pathname+data → AiPageContext. Entity extractors read API-loaded
// page data only (Rule 16); the path:{pathname} fallback gives every page
// a scoped thread (pathname chars are always anchor-safe — route.id
// values contain ()[] and are NOT usable in anchors).
import type { AiPageContext } from '@japaconnect/ai-panel';

type Data = Record<string, unknown>;

function str(v: unknown): string {
	return typeof v === 'string' ? v : '';
}

function pathAnchor(pathname: string, data: Data): AiPageContext {
	const title = str(data.title);
	return {
		anchor: `path:${pathname}`.toLowerCase(),
		label: title || 'Page',
		context: title ? { title } : {}
	};
}

function articleContext(d: Data): AiPageContext | null {
	const a = d.article as
		| {
				id?: string;
				slug?: string;
				title?: string;
				content_type?: string;
				space_id?: string | null;
				tags?: { slug?: string }[];
		  }
		| null;
	if (!a?.id) return null;
	return {
		anchor: `content:${a.id}`,
		label: `Blog · ${a.title ?? a.slug}`,
		context: {
			content_type: a.content_type ?? 'post',
			content_id: a.id,
			...(a.slug ? { slug: a.slug } : {}),
			...(a.title ? { title: a.title } : {}),
			...(a.space_id ? { space_id: a.space_id } : {}),
			...(Array.isArray(a.tags) && a.tags.length
				? { tags: a.tags.map((t) => str(t?.slug)).filter(Boolean) }
				: {})
		}
	};
}

function spaceContext(d: Data): AiPageContext | null {
	const space = (d.spaceDetail ?? d.space) as
		| { id?: string; slug?: string; name?: string }
		| null;
	if (!space?.id) return null;
	return {
		anchor: `space:${space.id}`,
		label: `Wiki · ${space.name ?? space.slug}`,
		context: {
			content_type: 'space',
			space_id: space.id,
			slug: space.slug ?? '',
			title: space.name ?? ''
		}
	};
}

function wikiPageContext(d: Data): AiPageContext | null {
	const p = d.page as
		| { id?: string; space_id?: string; slug?: string; title?: string; category?: string }
		| null;
	if (!p?.id) return null;
	return {
		anchor: `wikipage:${p.id}`,
		label: `Wiki · ${p.title ?? p.slug}`,
		context: {
			content_type: 'wiki_page',
			content_id: p.id,
			...(p.space_id ? { space_id: p.space_id } : {}),
			...(p.slug ? { slug: p.slug } : {}),
			...(p.title ? { title: p.title } : {})
		}
	};
}

export function resolveAiPageContext(pathname: string, data: Data): AiPageContext {
	const segs = pathname.split('/').filter(Boolean);
	// During client navigation pathname updates before page data — only
	// trust entity payloads whose slug agrees with the URL, otherwise a
	// stale object from the previous page would mint a wrong-entity anchor.
	const lastSeg = segs[segs.length - 1] ?? '';
	// /wiki/{space_slug}/{page_slug} → page; /wiki/{space_slug} → space.
	// (/wiki/{space}/{page}/history shares the page anchor — same entity.)
	if (segs[0] === 'wiki' && segs.length >= 3) {
		const p = data.page as { slug?: string } | null;
		if (p?.slug && (segs[2] === p.slug || segs[segs.length - 2] === p.slug)) {
			const ctx = wikiPageContext(data);
			if (ctx) return ctx;
		}
	}
	if (segs[0] === 'wiki' && segs.length === 2) {
		const s = (data.spaceDetail ?? data.space) as { slug?: string } | null;
		if (!s?.slug || s.slug === segs[1]) {
			const ctx = spaceContext(data);
			if (ctx) return ctx;
		}
	}
	// Generic entity fallback: article-bearing pages (blog, content,
	// guides, stories, comparisons) and space-bearing pages get their
	// entity anchor regardless of the route segment shape.
	const a = data.article as { slug?: string } | null;
	if (!a?.slug || a.slug === lastSeg) {
		const viaArticle = articleContext(data);
		if (viaArticle) return viaArticle;
	}
	const viaSpace = spaceContext(data);
	if (viaSpace) return viaSpace;
	return pathAnchor(pathname, data);
}
