// §115 — pathname+data → AiPageContext for the Docs surface. Doc pages
// (/docs/{audience}/{slug} → data.page from loadDocPage) get doc:{id}
// anchors; the payload's slug must agree with the URL — during client
// navigation pathname updates before page data, and a stale object would
// otherwise mint a wrong-entity anchor. Everything else falls back to
// path:docs:{pathname}.
import type { AiPageContext } from '@japaconnect/ai-panel';

type Data = Record<string, unknown>;

function str(v: unknown): string {
	return typeof v === 'string' ? v : '';
}

export function resolveAiPageContext(pathname: string, data: Data): AiPageContext {
	const segs = pathname.split('/').filter(Boolean);

	if (segs[0] === 'docs' && segs.length >= 3) {
		const p = data.page as
			| {
					id?: string;
					title?: string;
					slug?: string;
					category?: string | null;
					subcategory?: string | null;
					audience?: string;
			  }
			| null;
		if (p?.id && (!p.slug || p.slug === segs[2] || p.slug === segs[segs.length - 1])) {
			return {
				anchor: `doc:${p.id}`,
				label: `Docs · ${p.title ?? p.slug}`,
				context: {
					content_type: 'doc',
					content_id: p.id,
					...(p.slug ? { slug: p.slug } : {}),
					...(p.title ? { title: p.title } : {}),
					...(p.category ? { doc_category: p.category } : {}),
					...(p.subcategory ? { doc_subcategory: p.subcategory } : {}),
					...(p.audience ? { audience: p.audience } : {})
				}
			};
		}
	}

	const title = str(data.title);
	return {
		anchor: `path:docs:${pathname}`.toLowerCase(),
		label: title || 'Page',
		context: title ? { title } : {}
	};
}
