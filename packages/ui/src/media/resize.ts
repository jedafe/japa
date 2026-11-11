// Right-sizing for /media/serve URLs. The serve handler understands
// ?w= / ?h= (+ optional ?fmt=) and 302s to a signed imgproxy derivative, so
// any component embedding a media object in a known slot can ask for the
// size it renders without holding the signing key. Non-serve URLs (data:
// URIs, bundled assets, already-proxied URLs) pass through untouched.

const SERVE_RE = /\/media\/serve\//;

export interface MediaSizeOpts {
	w?: number;
	h?: number;
	format?: 'webp' | 'png' | 'jpg' | 'jpeg' | 'avif';
}

/** Append resize params to a /media/serve URL; pass everything else through. */
export function mediaSized(url: string | undefined | null, opts: MediaSizeOpts): string {
	if (!url || !SERVE_RE.test(url)) return url ?? '';
	const p = new URLSearchParams();
	if (opts.w) p.set('w', String(Math.min(2048, Math.round(opts.w))));
	if (opts.h) p.set('h', String(Math.min(2048, Math.round(opts.h))));
	if (opts.format) p.set('fmt', opts.format);
	const qs = p.toString();
	return qs ? `${url}${url.includes('?') ? '&' : '?'}${qs}` : url;
}

/** Build a width-descriptor srcset of resized serve-URL variants. */
export function mediaSrcSet(url: string | undefined | null, widths: number[]): string {
	if (!url || !SERVE_RE.test(url)) return '';
	return widths.map((w) => `${mediaSized(url, { w })} ${w}w`).join(', ');
}
