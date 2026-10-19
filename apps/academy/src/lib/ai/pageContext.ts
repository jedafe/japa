// §115 — pathname+data → AiPageContext for the Academy surface. Entity
// payloads are only trusted when their slug agrees with the URL — during
// client navigation pathname updates before page data, and a stale object
// would otherwise mint a wrong-entity anchor.
//   /tracks/{slug}                      → track:{track.id}
//   /tracks/{slug}/{module}/{lesson}    → lesson:{lesson.id}
//   everything else                     → path:academy:{pathname}
import type { AiPageContext } from '@japaconnect/ai-panel';

type Data = Record<string, unknown>;

function str(v: unknown): string {
	return typeof v === 'string' ? v : '';
}

export function resolveAiPageContext(pathname: string, data: Data): AiPageContext {
	const segs = pathname.split('/').filter(Boolean);

	if (segs[0] === 'tracks' && segs.length === 4) {
		const lesson = data.lesson as { id?: string; slug?: string; title?: string } | null;
		if (lesson?.id && (!lesson.slug || lesson.slug === segs[3])) {
			return {
				anchor: `lesson:${lesson.id}`,
				label: `Lesson · ${lesson.title ?? lesson.slug}`,
				context: {
					content_type: 'academy_lesson',
					content_id: lesson.id,
					...(lesson.slug ? { slug: lesson.slug } : {}),
					...(lesson.title ? { title: lesson.title } : {})
				}
			};
		}
	}

	if (segs[0] === 'tracks' && segs.length === 2) {
		const track = data.track as { id?: string; slug?: string; title?: string } | null;
		if (track?.id && (!track.slug || track.slug === segs[1])) {
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
	}

	const title = str(data.title);
	return {
		anchor: `path:academy:${pathname}`.toLowerCase(),
		label: title || 'Page',
		context: title ? { title } : {}
	};
}
