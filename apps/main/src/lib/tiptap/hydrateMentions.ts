/**
 * Svelte action that scans a container for `.mention[data-handle]` spans
 * and wraps each one in an `<a href="/u/{handle}">` link so mentions on
 * view pages link to the mentioned user's profile (Section 15).
 *
 * This runs on the rendered HTML (not in the editor) so it works for both
 * `editor.getHTML()` output (blog/content pages) and `serialize.ts` output
 * (forum pages), and also fixes existing content that was created before
 * the link feature was added — as long as the mention span carries a
 * `data-handle` attribute.
 *
 * Ref: japaconnect-engineering/sections/15-block-editor-tiptap.md
 *
 * Usage:
 *   <div class="prose" use:hydrateMentions>{@html body}</div>
 */
export function hydrateMentions(node: HTMLElement) {
	/**
	 * Guard a `data-url` value before it becomes a link href. The sanitizer
	 * validates data-url too, but hydrate must never trust the DOM — a
	 * `javascript:`/entity-encoded payload would execute on click. Returns
	 * the URL when safe (http/https/mailto/tel or a site-relative path),
	 * else '' so the caller renders a span instead of a link.
	 */
	function safeDataUrl(url: string): string {
		const trimmed = url.trim();
		if (!trimmed || trimmed.startsWith('//')) return '';
		const lowered = trimmed
			.split('')
			.filter((ch) => ch.charCodeAt(0) > 32)
			.join('')
			.toLowerCase();
		if (/^(javascript|data|vbscript):/.test(lowered)) return '';
		return trimmed;
	}

	function scan() {
		const mentions = node.querySelectorAll<HTMLElement>(
			'span.mention[data-handle]:not([data-linked])'
		);
		for (const el of mentions) {
			const handle = el.dataset.handle;
			// Handles are username slugs — reject anything outside the
			// slug alphabet so a stored data-handle="/x" onmouseover=…
			// cannot break out of the /u/ path into markup/JS.
			if (!handle || !/^[a-z0-9_]{1,30}$/i.test(handle)) continue;
			el.setAttribute('data-linked', 'true');
			const link = document.createElement('a');
			link.href = `/u/${handle}`;
			link.className = 'mention-link';
			el.parentElement?.insertBefore(link, el);
			link.appendChild(el);
		}

		// §15: hashtag (#tag) and interlink ([[content]]) nodes carry their
		// public path in data-url — wrap them in links the same way mentions
		// link to /u/{handle}.
		const refs = node.querySelectorAll<HTMLElement>(
			'span.hashtag[data-url]:not([data-linked]), span.interlink[data-url]:not([data-linked])'
		);
		for (const el of refs) {
			const url = el.dataset.url;
			if (!url) continue;
			el.setAttribute('data-linked', 'true');
			const safe = safeDataUrl(url);
			if (!safe) {
				// Unsafe target — drop the link and leave a plain span so no
				// javascript:/entity-encoded payload becomes clickable.
				const span = document.createElement('span');
				span.className = el.classList.contains('interlink') ? 'interlink-link' : 'hashtag-link';
				el.parentElement?.insertBefore(span, el);
				span.appendChild(el);
				continue;
			}
			const link = document.createElement('a');
			link.href = safe;
			link.className = el.classList.contains('interlink') ? 'interlink-link' : 'hashtag-link';
			el.parentElement?.insertBefore(link, el);
			link.appendChild(el);
		}
	}

	scan();

	const observer = new MutationObserver(() => scan());
	observer.observe(node, { childList: true, subtree: true });

	return {
		destroy() {
			observer.disconnect();
		},
	};
}
