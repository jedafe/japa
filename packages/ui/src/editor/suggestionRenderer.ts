import type { SuggestionProps, SuggestionKeyDownProps } from '@tiptap/suggestion';
import type { EditorSearchHit } from './types';

interface RendererConfig {
	/** entity_type stored in the mention node attrs (Section 15: {id, entity_type, label}). */
	entityType?: string;
	/** Localized "No results" text (Section 7 i18n). Defaults to English. */
	noResultsText?: string;
	/** Text shown while the typed query is still shorter than
	 *  `minQueryLength` — the popup is open but the plugin has not
	 *  started fetching yet, so "No results" would be misleading. */
	startTypingText?: string;
	/** Mirror of the suggestion plugin's `minQueryLength` option so the
	 *  renderer knows when an empty item list means "keep typing" versus
	 *  "the search ran and found nothing". Defaults to 0. */
	minQueryLength?: number;
}

interface RenderReturn {
	onBeforeStart?: (props: SuggestionProps) => void;
	onStart?: (props: SuggestionProps) => void;
	onBeforeUpdate?: (props: SuggestionProps) => void;
	onUpdate?: (props: SuggestionProps) => void;
	onExit?: (props: SuggestionProps) => void;
	onKeyDown?: (props: SuggestionKeyDownProps) => boolean;
}

/**
 * Vanilla-DOM suggestion renderer for TipTap v3's Suggestion plugin.
 *
 * In v3, the plugin owns the async fetching flow — it calls the `items`
 * function from the suggestion config, handles debouncing/aborting, and
 * passes the results via `props.items`. The renderer only needs to render
 * those items and handle keyboard navigation.
 *
 * TipTap v3 may call `onUpdate` on every ProseMirror state transaction
 * (not just when the query text changes), which can be 60+ times per
 * second. To keep the popup clickable, we only rebuild the DOM when the
 * item set actually changes, and we update the `active` class in-place
 * when only the highlighted index moves.
 */
export function createSuggestionRenderer(cfg: RendererConfig): RenderReturn {
	let popup: HTMLDivElement | null = null;
	let index = 0;
	let command: ((item: unknown) => void) | null = null;
	/** Signature of the last rendered item set, used to skip no-op re-renders. */
	let lastSignature = '';
	/** The items currently rendered in the popup. */
	let currentItems: EditorSearchHit[] = [];
	/** The suggestion query as of the last render callback — used to pick
	 *  between the "start typing" hint and "No results" on an empty list. */
	let currentQuery = '';

	function itemsSignature(items: EditorSearchHit[]): string {
		if (items.length === 0) {
			// Include the empty-state mode so the signature changes (and the
			// popup text updates) when the query crosses minQueryLength
			// even though the item list stays empty.
			return queryBelowMin() ? 'empty:start-typing' : 'empty:no-results';
		}
		return items.map((i) => `${i.id}:${i.label}`).join('|');
	}

	function queryBelowMin(): boolean {
		return currentQuery.length < (cfg.minQueryLength ?? 0);
	}

	/**
	 * Full rebuild of the popup's child nodes. Only called when the item
	 * set changes — not on every state transaction.
	 */
	function rebuildList(items: EditorSearchHit[]) {
		if (!popup) return;
		currentItems = items;
		lastSignature = itemsSignature(items);
		popup.innerHTML = '';
		if (items.length === 0) {
			popup.textContent = queryBelowMin()
				? (cfg.startTypingText ?? 'Start typing…')
				: (cfg.noResultsText ?? 'No results');
			return;
		}
		items.forEach((hit, i) => {
			const btn = document.createElement('button');
			btn.type = 'button';
			btn.className = 'suggestion-item' + (i === index ? ' active' : '');
			btn.textContent = hit.label;
			btn.addEventListener('mouseenter', () => {
				index = i;
				updateActive();
			});
			btn.addEventListener('mousedown', (e) => {
				e.preventDefault();
				selectItem(items, i);
			});
			popup!.appendChild(btn);
		});
	}

	/**
	 * Update only the `active` class on existing buttons — no DOM rebuild.
	 * Called on mouseenter and arrow-key navigation.
	 */
	function updateActive() {
		if (!popup) return;
		const buttons = popup.querySelectorAll('.suggestion-item');
		buttons.forEach((btn, i) => {
			btn.classList.toggle('active', i === index);
		});
	}

	function selectItem(items: EditorSearchHit[], i: number) {
		if (!items[i] || !command) return;
		// Section 15: all reference nodes store { id, entity_type, label }.
		// entity_type defaults to 'user' for @-mentions. For user mentions
		// we also store the handle (username) so the view-page serializer
		// can emit a link to /u/{handle}.
		const item = items[i];
		const attrs: Record<string, string> = {
			id: item.id,
			entity_type: cfg.entityType ?? 'user',
			label: item.label
		};
		if (cfg.entityType === 'user' && item.handle) {
			attrs.handle = item.handle;
		}
		// §15: reference nodes link at render time — store the public path so
		// view pages can wrap hashtag/interlink spans in <a href>.
		if (item.url) {
			attrs.url = item.url;
		}
		command(attrs);
		close();
	}

	function onKeyDown(e: SuggestionKeyDownProps): boolean {
		if (!popup) return false;
		const items = currentItems;
		if (e.event.key === 'ArrowDown') {
			index = (index + 1) % Math.max(items.length, 1);
			updateActive();
			return true;
		}
		if (e.event.key === 'ArrowUp') {
			index = (index - 1 + items.length) % Math.max(items.length, 1);
			updateActive();
			return true;
		}
		if (e.event.key === 'Enter') {
			e.event.preventDefault();
			selectItem(items, index);
			return true;
		}
		if (e.event.key === 'Escape') {
			close();
			return true;
		}
		return false;
	}

	/**
	 * Returns true if the TipTap editor's DOM is visible (has a layout
	 * box). When the editor is hidden via CSS (e.g. markdown mode uses
	 * display:none on .editor-surface), clientRect returns all zeros
	 * and the popup would appear at the top-left of the viewport.
	 */
	function editorIsVisible(props: SuggestionProps): boolean {
		const dom = props.editor?.view?.dom as HTMLElement | undefined;
		if (!dom) return false;
		const rect = dom.getBoundingClientRect();
		return rect.width > 0 && rect.height > 0;
	}

	function positionPopup(props: SuggestionProps) {
		if (!popup) return;
		const rect = props.clientRect?.();
		if (rect && rect.width > 0) {
			popup.style.top = `${rect.bottom + 4}px`;
			popup.style.left = `${rect.left}px`;
		}
	}

	function close() {
		popup?.remove();
		popup = null;
		command = null;
		index = 0;
		lastSignature = '';
		currentItems = [];
		currentQuery = '';
	}

	return {
		onBeforeStart: (props) => {
			if (!editorIsVisible(props)) return;
			command = props.command;
			currentQuery = props.query ?? '';
			popup = document.createElement('div');
			popup.className = 'suggestion-popup';
			positionPopup(props);
			document.body.appendChild(popup);
			index = 0;
			const items = props.items as EditorSearchHit[];
			rebuildList(items);
		},
		onStart: (props) => {
			if (!editorIsVisible(props)) return;
			command = props.command;
			currentQuery = props.query ?? '';
			positionPopup(props);
			const items = props.items as EditorSearchHit[];
			rebuildList(items);
		},
		onBeforeUpdate: (props) => {
			command = props.command;
			currentQuery = props.query ?? '';
			positionPopup(props);
		},
		onUpdate: (props) => {
			command = props.command;
			currentQuery = props.query ?? '';
			positionPopup(props);
			// Safety net: if the editor becomes hidden (e.g. user
			// switched to markdown mode while a popup was open), close
			// the popup immediately rather than showing it at (0,0).
			if (!editorIsVisible(props)) {
				close();
				return;
			}
			const items = props.items as EditorSearchHit[];
			// Only rebuild the DOM if the item set actually changed.
			// TipTap v3 calls onUpdate on every state transaction; without
			// this guard the buttons are destroyed and recreated 60+ times
			// per second, making the popup impossible to click.
			const sig = itemsSignature(items);
			if (sig !== lastSignature) {
				index = 0;
				rebuildList(items);
			}
		},
		onKeyDown,
		onExit: close
	};
}
