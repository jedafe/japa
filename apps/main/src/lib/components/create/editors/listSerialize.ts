// Ref: docs/superpowers/specs/2026-09-28-list-content-type-design.md
//      .jeos/sections/14-content-type-system.md (List amendment)
//
// Pure serialization seam for the list (listicle) composer. Emits the
// canonical body_html markup — intro HTML followed by one
// `<div class="list-item" id="item-N">` block per item with baked-in
// number badges (`<span class="li-num">N.</span>`), embed placeholders
// hydrated client-side on the public page, and source/credit lines.
// Only sanitizer-surviving markup is emitted (internal/content/sanitize.go):
// div[class,id,data-type,data-url,data-content-slug,data-video-source-id],
// h3, span[class], p, a[href], img[src,alt]. `loading` is NOT whitelisted
// on img — emitting it would produce a silent diff on save.

import type { ListItem, ListMode } from '$lib/types';

const esc = (s: string) =>
	s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

/** Displayed number for the item at `index`. Numbered/unnumbered lists count
 *  up from 1; countdown lists bake the reverse number (first item = total). */
export function displayNumber(mode: ListMode, index: number, total: number): number {
	return mode === 'countdown' ? total - index : index + 1;
}

/** Serialize an item's visual slot: media image XOR one embed (internal
 *  content, external link preview, or video source). Empty string when the
 *  item carries neither. */
function embedHtml(item: ListItem, mediaUrls: Record<string, string>): string {
	if (item.media_id && mediaUrls[item.media_id]) {
		// NOTE: `loading` is not in the img attr whitelist (sanitize.go) — it
		// would be stripped on save. Only src/alt/title/width/height survive.
		return `<img src="${esc(mediaUrls[item.media_id])}" alt="">`;
	}
	const e = item.embed;
	if (!e) return '';
	if (e.kind === 'content' && e.content_slug) {
		const snap = e.snapshot ?? {};
		const card =
			`<a href="/content/${esc(e.content_slug)}" class="ce-card">` +
			(snap.image_url ? `<img src="${esc(snap.image_url)}" alt="">` : '') +
			`<span class="ce-title">${esc(snap.title ?? e.content_slug)}</span>` +
			(snap.excerpt ? `<span class="ce-excerpt">${esc(snap.excerpt)}</span>` : '') +
			`</a>`;
		return `<div data-type="content-embed" data-content-slug="${esc(e.content_slug)}">${card}</div>`;
	}
	if (e.kind === 'url' && e.url) {
		if (e.resolved?.embed_url) {
			// Dormant in v1 — GET /link-preview never returns embed_url, so the
			// composer cannot populate it. Kept so a future oEmbed resolver can
			// light it up without a schema change (spec reconciliation 4).
			return `<iframe src="${esc(e.resolved.embed_url)}" allowfullscreen loading="lazy"></iframe>`;
		}
		return `<div data-type="link-preview" data-url="${esc(e.url)}"></div>`;
	}
	if (e.kind === 'video' && e.video_source_id) {
		// Poster child gives a no-JS preview; hydrateVideoEmbeds replaces
		// children with the player. Thumbnail rides in resolved.image_url.
		const poster = e.resolved?.image_url
			? `<img src="${esc(e.resolved.image_url)}" alt="" class="video-embed-thumb">`
			: '';
		return `<div class="video-embed" data-video-source-id="${esc(e.video_source_id)}">${poster}</div>`;
	}
	return '';
}

/** Serialize the whole list composer state into the canonical body_html
 *  persisted on the contents row. `mediaUrls` maps media_id → public URL
 *  (resolved by the composer via GET /media/{id}). */
export function serializeListBodyHtml(
	mode: ListMode,
	introHtml: string,
	items: ListItem[],
	mediaUrls: Record<string, string>
): string {
	const parts: string[] = [];
	if (introHtml.trim()) parts.push(introHtml);
	items.forEach((item, i) => {
		const n = i + 1;
		const num = displayNumber(mode, i, items.length);
		const badge = mode === 'unnumbered' ? '' : `<span class="li-num">${num}.</span> `;
		const heading = item.heading?.trim()
			? `<h3>${badge}${esc(item.heading.trim())}</h3>`
			: badge
				? `<h3>${badge.trim()}</h3>`
				: '';
		// Item body_html embeds raw — the server re-sanitizes on save and
		// field_values.items[].body_html is re-sanitized by
		// validateListFieldValues before persistence.
		const body = item.body_html?.trim() ? item.body_html : '';
		const visual = embedHtml(item, mediaUrls);
		const credit = item.source_url || item.credit
			? `<p class="li-credit">${
					item.source_url
						? `<a href="${esc(item.source_url)}">${esc(item.credit || item.source_url)}</a>`
						: esc(item.credit ?? '')
				}</p>`
			: '';
		parts.push(`<div class="list-item" id="item-${n}">${heading}${visual}${body}${credit}</div>`);
	});
	return parts.join('');
}
