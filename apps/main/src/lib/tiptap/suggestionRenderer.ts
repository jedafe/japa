import type { SuggestionProps, SuggestionKeyDownProps } from '@tiptap/suggestion';
import type { SearchHit } from '$lib/types';

interface RendererConfig {
	onSearch: (query: string) => Promise<SearchHit[]>;
	/** entity_type stored in the mention node attrs (Section 15: {id, entity_type, label}). */
	entityType?: string;
}

interface RenderReturn {
	onStart?: (props: SuggestionProps) => void;
	onUpdate?: (props: SuggestionProps) => void;
	onExit?: (props: SuggestionProps) => void;
	onKeyDown?: (props: SuggestionKeyDownProps) => boolean;
}

/**
 * Vanilla-DOM suggestion renderer for TipTap's Suggestion plugin.
 * Creates a floating popup, fetches results via onSearch, and calls
 * props.command when the user picks an item. Keyboard nav (Up/Down/Enter/Esc)
 * is handled inside the popup.
 */
export function createSuggestionRenderer(cfg: RendererConfig): RenderReturn {
	let popup: HTMLDivElement | null = null;
	let items: SearchHit[] = [];
	let index = 0;
	let command: ((item: unknown) => void) | null = null;

	function renderList() {
		if (!popup) return;
		popup.innerHTML = '';
		if (items.length === 0) {
			popup.textContent = 'No results';
			return;
		}
		items.forEach((hit, i) => {
			const btn = document.createElement('button');
			btn.type = 'button';
			btn.className = 'suggestion-item' + (i === index ? ' active' : '');
			btn.textContent = hit.label;
			btn.addEventListener('mouseenter', () => {
				index = i;
				renderList();
			});
			btn.addEventListener('mousedown', (e) => {
				e.preventDefault();
				selectItem(i);
			});
			popup!.appendChild(btn);
		});
	}

	async function updateItems(query: string) {
		items = await cfg.onSearch(query);
		index = 0;
		renderList();
	}

	function selectItem(i: number) {
		if (!items[i] || !command) return;
		// Section 15: all reference nodes store { id, entity_type, label }.
		// entity_type defaults to 'user' for @-mentions.
		command({ id: items[i].id, entity_type: cfg.entityType ?? 'user', label: items[i].label });
		close();
	}

	function onKeyDown(e: SuggestionKeyDownProps): boolean {
		if (!popup) return false;
		if (e.event.key === 'ArrowDown') {
			index = (index + 1) % Math.max(items.length, 1);
			renderList();
			return true;
		}
		if (e.event.key === 'ArrowUp') {
			index = (index - 1 + items.length) % Math.max(items.length, 1);
			renderList();
			return true;
		}
		if (e.event.key === 'Enter') {
			e.event.preventDefault();
			selectItem(index);
			return true;
		}
		if (e.event.key === 'Escape') {
			close();
			return true;
		}
		return false;
	}

	function close() {
		popup?.remove();
		popup = null;
		command = null;
	}

	return {
		onStart: (props) => {
			command = props.command;
			popup = document.createElement('div');
			popup.className = 'suggestion-popup';
			const rect = props.clientRect?.();
			if (rect) {
				popup.style.top = `${rect.bottom + 4}px`;
				popup.style.left = `${rect.left}px`;
			}
			document.body.appendChild(popup);
			void updateItems(props.query || '');
		},
		onUpdate: (props) => {
			command = props.command;
			const rect = props.clientRect?.();
			if (popup && rect) {
				popup.style.top = `${rect.bottom + 4}px`;
				popup.style.left = `${rect.left}px`;
			}
			void updateItems(props.query || '');
		},
		onKeyDown,
		onExit: close
	};
}
