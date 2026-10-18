// §115 — pathname → AiPageContext for the Tools surface. Tool anchors
// come from the app's own TOOLS registry (ToolMeta.route ↔ slug), never a
// hardcoded route list; everything else falls back to path:{pathname}.
import { TOOLS } from '$lib/tools/registry';
import type { AiPageContext } from '@japaconnect/ai-panel';

export function resolveAiPageContext(
	pathname: string,
	data: Record<string, unknown>
): AiPageContext {
	const tool = TOOLS.find((t) => pathname === t.route || pathname.startsWith(t.route + '/'));
	if (tool) {
		return {
			anchor: `tool:${tool.slug}`,
			label: tool.name,
			context: { content_type: 'tool', slug: tool.slug, title: tool.name }
		};
	}
	const title = typeof data.title === 'string' ? data.title : '';
	return {
		anchor: `path:${pathname}`.toLowerCase(),
		label: title || 'Page',
		context: title ? { title } : {}
	};
}
