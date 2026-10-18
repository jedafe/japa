// §115 — pathname+data → AiPageContext for the Docs surface. Doc pages
// (data.page from loadDocPage: id, title, slug, category, subcategory,
// audience) get doc:{id} anchors; everything else falls back to
// path:{pathname}.
import type { AiPageContext } from '@japaconnect/ai-panel';

type Data = Record<string, unknown>;

function str(v: unknown): string {
	return typeof v === 'string' ? v : '';
}

export function resolveAiPageContext(pathname: string, data: Data): AiPageContext {
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
	if (p?.id) {
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
	const title = str(data.title);
	return {
		anchor: `path:${pathname}`.toLowerCase(),
		label: title || 'Page',
		context: title ? { title } : {}
	};
}
