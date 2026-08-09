export function escapeHtml(str: unknown): string {
	if (str == null) return '';
	return String(str)
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#39;');
}

/**
 * Strip everything except a small set of safe tags and their `class`/`style`
 * attributes. Used for DataTable cell renders where the source is
 * admin/operator content but we only want lightweight formatting.
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
