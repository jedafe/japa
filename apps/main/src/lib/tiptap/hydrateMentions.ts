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
	function scan() {
		const mentions = node.querySelectorAll<HTMLElement>(
			'span.mention[data-handle]:not([data-linked])'
		);
		for (const el of mentions) {
			const handle = el.dataset.handle;
			if (!handle) continue;
			el.setAttribute('data-linked', 'true');
			const link = document.createElement('a');
			link.href = `/u/${handle}`;
			link.className = 'mention-link';
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
