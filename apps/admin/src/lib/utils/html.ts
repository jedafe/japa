/**
 * Strip everything except a small set of safe tags and their `class`/`style`
 * attributes. Used for cell renders and page-builder previews where the source
 * is user/admin content but we only want lightweight formatting.
 */
export function sanitizeHtml(html: string): string {
	return html.replace(
		/<(\/?)(b|i|strong|em|code|span)(\b[^>]*)?>/gi,
		(match, slash, tag, attrs) => {
			if (slash) return `</${tag}>`;
			if (!attrs) return `<${tag}>`;
			const safe = attrs.replace(/\s+(?!class|style)[a-z-]+=("[^"]*"|'[^']*'|\S+)/gi, '');
			return `<${tag}${safe}>`;
		}
	);
}
