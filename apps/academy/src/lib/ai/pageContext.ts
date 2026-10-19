// §115 — pathname+data → AiPageContext for the Academy surface. Track and
// lesson pages get entity anchors; everything else falls back to
// path:{pathname}.
import type { AiPageContext } from '@japaconnect/ai-panel';

type Data = Record<string, unknown>;

function str(v: unknown): string {
	return typeof v === 'string' ? v : '';
}

export function resolveAiPageContext(pathname: string, data: Data): AiPageContext {
	const lesson = data.lesson as { id?: string; title?: string } | null;
	if (lesson?.id) {
		return {
			anchor: `lesson:${lesson.id}`,
			label: `Lesson · ${lesson.title ?? lesson.id}`,
			context: {
				content_type: 'academy_lesson',
				content_id: lesson.id,
				...(lesson.title ? { title: lesson.title } : {})
			}
		};
	}
	const track = data.track as { id?: string; slug?: string; title?: string } | null;
	if (track?.id) {
		return {
			anchor: `track:${track.id}`,
			label: `Track · ${track.title ?? track.slug}`,
			context: {
				content_type: 'academy_track',
				content_id: track.id,
				...(track.slug ? { slug: track.slug } : {}),
				...(track.title ? { title: track.title } : {})
			}
		};
	}
	const title = str(data.title);
	return {
		anchor: `path:academy:${pathname}`.toLowerCase(),
		label: title || 'Page',
		context: title ? { title } : {}
	};
}
