import { Node, mergeAttributes } from '@tiptap/core';

/**
 * Generic video embed node (§106 — Video Content Import).
 *
 * Unlike the YouTube extension (which embeds an iframe for a known URL),
 * this node stores the canonical `video_sources` row reference plus enough
 * metadata (title, thumbnail, embed HTML) to render a placeholder card in
 * the editor. The backend resolves the real player at view time via
 * GET /api/v1/videos/{id} — the server-side HTML sanitizer strips the
 * data-* attributes from body_html, so the node degrades to an empty
 * `.video-embed` div in stored content and the single page renders the
 * player from the linked video source.
 */
export const VideoNode = Node.create({
	name: 'video',
	group: 'block',
	atom: true,
	draggable: true,

	addAttributes() {
		return {
			video_source_id: { default: '' },
			source_url: { default: '' },
			source_type: { default: '' },
			external_id: { default: '' },
			title: { default: '' },
			thumbnail_url: { default: '' },
			embed_html: { default: '' },
			duration_secs: { default: 0 }
		};
	},

	parseHTML() {
		return [{ tag: 'div[data-video-source-id]' }];
	},

	renderHTML({ HTMLAttributes, node }) {
		const children: [string, Record<string, string>][] = [];
		if (node.attrs.thumbnail_url) {
			children.push([
				'img',
				{
					src: String(node.attrs.thumbnail_url),
					alt: String(node.attrs.title || ''),
					class: 'video-embed-thumb'
				}
			]);
		}

		return [
			'div',
			mergeAttributes(HTMLAttributes, {
				class: 'video-embed',
				'data-video-source-id': String(node.attrs.video_source_id ?? ''),
				'data-source-type': String(node.attrs.source_type ?? ''),
				'data-embed-html': String(node.attrs.embed_html ?? '')
			}),
			...children
		];
	}
});
