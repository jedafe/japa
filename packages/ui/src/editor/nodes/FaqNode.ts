import { Node, type CommandProps, type Editor, type JSONContent } from '@tiptap/core';
import { TextSelection } from '@tiptap/pm/state';

/**
 * FAQ accordion block (§45 V1 Block Catalog — "FAQ Accordion" / "Details
 * Summary"). Four cooperating nodes model a Q/A pair list:
 *
 *   faq (block, data-type="faq")
 *     └ faqItem (details.faq-item[data-type="faq-item"][open])
 *         ├ faqSummary  (summary.faq-question — inline question text)
 *         └ faqContent  (div.faq-answer — block+ answer content)
 *
 * Serialized as semantic <details>/<summary> markup: published posts render a
 * native, JS-free accordion, and the backend sanitizer whitelists the tags and
 * their data-type markers (internal/content/sanitize.go), so editor reloads
 * round-trip losslessly.
 *
 * Markdown round-trip follows the linkPreview precedent: the nodes emit raw
 * HTML, which the markdown parser feeds back through parseHTML. Note that
 * inline marks inside a faq item serialize as literal markdown syntax inside
 * the HTML block (the markdown lexer does not re-process markdown nested in
 * raw HTML) — accepted trade-off, same as other structured nodes.
 *
 * Keyboard UX (no node view):
 *   - Enter in a summary → caret into the answer body.
 *   - Enter on an empty summary of the LAST item → drops the item (or the
 *     whole block when it's the only item) and exits with a paragraph —
 *     the escape hatch.
 *   - Enter on an EMPTY trailing paragraph of an answer → removes the
 *     paragraph and jumps to the next item's summary (or appends a new
 *     Q/A item when the caret is in the last item).
 *   - Backspace at the start of an empty summary → removes the item (or
 *     the whole block when it holds a single item).
 */

/** The JSON document for a fresh faq block with one empty item. */
function newFaqDoc(): Record<string, unknown> {
	return {
		type: 'faq',
		content: [
			{
				type: 'faqItem',
				content: [
					{ type: 'faqSummary' },
					{ type: 'faqContent', content: [{ type: 'paragraph' }] }
				]
			}
		]
	};
}

export const FaqNode = Node.create({
	name: 'faq',
	group: 'block',
	content: 'faqItem+',
	defining: true,

	addOptions() {
		return {
			/** Label for the editor-only "add item" button rendered by the node view. */
			addItemLabel: 'Add question',
			/** Live title of the document being edited (e.g. the post title) —
			 *  resolved at insert time so the auto-generated subheading uses
			 *  the current value. */
			getPostTitle: (): string => ''
		};
	},

	parseHTML() {
		return [{ tag: 'div[data-type="faq"]' }];
	},

	renderHTML() {
		return ['div', { 'data-type': 'faq', class: 'faq-block' }, 0];
	},

	// Editor-only node view: wraps the items in a content div and appends a
	// non-editable "Add question" button that inserts a new faqItem.
	addNodeView() {
		const label = this.options.addItemLabel;
		return ({ editor, getPos }) => {
			const dom = document.createElement('div');
			dom.setAttribute('data-type', 'faq');
			dom.className = 'faq-block';
			const contentDOM = document.createElement('div');
			contentDOM.className = 'faq-items';
			const add = document.createElement('button');
			add.type = 'button';
			add.className = 'faq-add';
			add.contentEditable = 'false';
			add.textContent = `+ ${label}`;
			add.addEventListener('click', (e) => {
				e.preventDefault();
				const pos = typeof getPos === 'function' ? getPos() : undefined;
				if (typeof pos !== 'number') return;
				editor
					.chain()
					.focus()
					.command(({ tr, state }) => {
						const cur = tr.doc.nodeAt(pos);
						if (!cur || cur.type.name !== 'faq') return false;
						const item = state.schema.nodes.faqItem.createAndFill();
						if (!item) return false;
						// Inside the faq node, right after its last item.
						const insertPos = pos + cur.nodeSize - 1;
						tr.insert(insertPos, item);
						// New item's summary text slot = insertPos + 2.
						tr.setSelection(
							TextSelection.near(tr.doc.resolve(insertPos + 2), 1)
						).scrollIntoView();
						return true;
					})
					.run();
			});
			dom.append(contentDOM, add);
			return { dom, contentDOM };
		};
	},

	renderMarkdown(node: JSONContent, h: { renderChildren: (nodes: JSONContent | JSONContent[]) => string }) {
		const inner = h.renderChildren(node.content ?? []);
		return `<div data-type="faq">\n${inner}\n</div>`;
	},

	addCommands() {
		return {
			/** Insert an H2 "FAQ" + subheading + faq block with one empty item;
			 *  caret lands in the question. */
			insertFaq:
				() =>
				({ chain, state }: CommandProps) => {
					const postTitle = this.options.getPostTitle?.() ?? '';
					const subtitle = postTitle
						? `Frequently Asked Questions about ${postTitle}`
						: 'Frequently Asked Questions';
					const seed = [
						{
							type: 'heading',
							attrs: { level: 2 },
							content: [{ type: 'text', text: 'FAQ' }]
						},
						{ type: 'paragraph', content: [{ type: 'text', text: subtitle }] },
						newFaqDoc()
					];
					// After insertion, the selection sits inside the new item's
					// answer (insertContent leaves it at the end of inserted
					// content) — walk up to the faqItem and move into its summary.
					const caretIntoSummary = ({ tr }: CommandProps) => {
						const $sel = tr.selection.$from;
						for (let d = $sel.depth; d >= 1; d--) {
							if ($sel.node(d).type.name === 'faqItem') {
								tr.setSelection(
									TextSelection.near(tr.doc.resolve($sel.before(d) + 2), 1)
								).scrollIntoView();
								return true;
							}
						}
						return false;
					};

					const $from = state.selection.$from;
					// A faq can't nest inside another faq — append the new
					// block after the enclosing one instead.
					for (let d = $from.depth; d >= 1; d--) {
						if ($from.node(d).type.name === 'faq') {
							return chain()
								.insertContentAt($from.after(d), seed)
								.command(caretIntoSummary)
								.focus()
								.run();
						}
					}
					return chain()
						.insertContent(seed)
						.command(caretIntoSummary)
						.focus()
						.run();
				}
		};
	}
});

export const FaqItemNode = Node.create({
	name: 'faqItem',
	content: 'faqSummary faqContent',
	defining: true,
	isolating: true,

	parseHTML() {
		return [{ tag: 'details[data-type="faq-item"]' }];
	},

	renderHTML() {
		// No `open` in stored markup — published pages render a collapsed
		// accordion. The `name` group gives native single-open semantics
		// (one item open at a time, matching the page-builder FaqBlock).
		return ['details', { 'data-type': 'faq-item', class: 'faq-item', name: 'jc-faq' }, 0];
	},

	// Editor-only node view: items mount expanded (DOM `open` property — not
	// serialized) so answers are visible for editing; the published markup
	// stays collapsed because `open` is never emitted by renderHTML.
	addNodeView() {
		return () => {
			const dom = document.createElement('details');
			dom.className = 'faq-item';
			dom.open = true;
			return { dom, contentDOM: dom };
		};
	},

	renderMarkdown(node: JSONContent, h: { renderChildren: (nodes: JSONContent | JSONContent[]) => string }) {
		const children = node.content ?? [];
		const question = children[0] ? h.renderChildren([children[0]]).trim() : '';
		const answer = children.length > 1 ? h.renderChildren(children.slice(1)).trim() : '';
		return `<details name="jc-faq" data-type="faq-item"><summary data-type="faq-question">${question}</summary><div data-type="faq-answer">${answer}</div></details>`;
	}
});

export const FaqSummaryNode = Node.create({
	name: 'faqSummary',
	content: 'inline*',
	defining: true,

	parseHTML() {
		return [{ tag: 'summary[data-type="faq-question"]' }];
	},

	renderHTML() {
		return ['summary', { 'data-type': 'faq-question', class: 'faq-question' }, 0];
	},

	renderMarkdown(node: JSONContent, h: { renderChildren: (nodes: JSONContent | JSONContent[]) => string }) {
		return h.renderChildren(node.content ?? []);
	},

	addKeyboardShortcuts() {
		return {
			Enter: ({ editor }: { editor: Editor }) => {
				const { state } = editor;
				const { $from, empty } = state.selection;
				if (!empty || $from.parent.type.name !== 'faqSummary') return false;

				const itemDepth = $from.depth - 1;
				const faqDepth = itemDepth - 1;
				const faqNode = $from.node(faqDepth);
				const isLastItem = $from.index(itemDepth) === faqNode.childCount - 1;

				// Escape hatch: Enter on the empty summary of the last item
				// drops the item (or the whole block when it's the only one —
				// `faqItem+` requires at least one item) and exits with a
				// paragraph at the block's position.
				if ($from.parent.content.size === 0 && isLastItem) {
					return editor
						.chain()
						.focus()
						.command(({ tr, state }) => {
							if (faqNode.childCount <= 1) {
								const faqStart = $from.before(faqDepth);
								const faqEnd = $from.after(faqDepth);
								tr.delete(faqStart, faqEnd);
								tr.insert(faqStart, state.schema.nodes.paragraph.create());
								tr.setSelection(
									TextSelection.near(tr.doc.resolve(faqStart + 1), 1)
								).scrollIntoView();
								return true;
							}
							const itemStart = $from.before(itemDepth);
							const itemEnd = $from.after(itemDepth);
							const faqEnd = $from.after(faqDepth);
							tr.delete(itemStart, itemEnd);
							const insertPos = faqEnd - (itemEnd - itemStart);
							tr.insert(insertPos, state.schema.nodes.paragraph.create());
							tr.setSelection(
								TextSelection.near(tr.doc.resolve(insertPos + 1), 1)
							).scrollIntoView();
							return true;
						})
						.run();
				}

				// Otherwise jump into the answer section of this item.
				const sel = TextSelection.near(state.doc.resolve($from.after() + 1), 1);
				return editor
					.chain()
					.focus()
					.command(({ tr }) => {
						tr.setSelection(sel);
						return true;
					})
					.run();
			},
			Backspace: ({ editor }: { editor: Editor }) => {
				const { state } = editor;
				const { $from, empty } = state.selection;
				if (!empty || $from.parent.type.name !== 'faqSummary') return false;
				if ($from.parentOffset !== 0 || $from.parent.content.size > 0) return false;

				const itemDepth = $from.depth - 1;
				const faqDepth = itemDepth - 1;
				const faqNode = $from.node(faqDepth);
				return editor
					.chain()
					.focus()
					.command(({ tr }) => {
						if (faqNode.childCount <= 1) {
							tr.delete($from.before(faqDepth), $from.after(faqDepth));
						} else {
							tr.delete($from.before(itemDepth), $from.after(itemDepth));
						}
						return true;
					})
					.run();
			}
		};
	}
});

export const FaqContentNode = Node.create({
	name: 'faqContent',
	content: 'block+',
	isolating: true,

	parseHTML() {
		return [{ tag: 'div[data-type="faq-answer"]' }];
	},

	renderHTML() {
		return ['div', { 'data-type': 'faq-answer', class: 'faq-answer' }, 0];
	},

	renderMarkdown(node: JSONContent, h: { renderChildren: (nodes: JSONContent | JSONContent[]) => string }) {
		return h.renderChildren(node.content ?? []);
	},

	addKeyboardShortcuts() {
		return {
			Enter: ({ editor }: { editor: Editor }) => {
				const { state } = editor;
				const { $from, empty } = state.selection;
				if (!empty || $from.parent.type.name !== 'paragraph') return false;
				// Only the empty trailing paragraph of an answer triggers the
				// item hop — Enter anywhere else falls through to the default
				// paragraph split so answers can grow freely.
				if ($from.parent.content.size !== 0) return false;

				let contentDepth = -1;
				let itemDepth = -1;
				let faqDepth = -1;
				for (let d = $from.depth; d >= 1; d--) {
					const name = $from.node(d).type.name;
					if (name === 'faqContent') contentDepth = d;
					else if (name === 'faqItem') itemDepth = d;
					else if (name === 'faq') {
						faqDepth = d;
						break;
					}
				}
				if (contentDepth === -1 || itemDepth === -1 || faqDepth === -1) return false;
				const contentNode = $from.node(contentDepth);
				if (contentNode.lastChild !== $from.parent) return false;

				const faqNode = $from.node(faqDepth);
				const isLastItem = $from.index(itemDepth) === faqNode.childCount - 1;
				// The paragraph may be dropped on the hop — unless it's the
				// answer's only block (`block+` requires one child).
				const dropPara = contentNode.childCount > 1;

				return editor
					.chain()
					.focus()
					.command(({ tr, state }) => {
						// Remove the empty paragraph, then hop to the next
						// question — the next item's summary, or a freshly
						// appended item when the caret was in the last one.
						const paraStart = $from.before();
						const removed = dropPara ? $from.after() - paraStart : 0;
						if (dropPara) tr.delete(paraStart, $from.after());
						if (!isLastItem) {
							const nextItemStart = $from.after(itemDepth) - removed;
							tr.setSelection(
								TextSelection.near(tr.doc.resolve(nextItemStart + 2), 1)
							).scrollIntoView();
							return true;
						}
						const insertPos = $from.end(faqDepth) - removed;
						const item = state.schema.nodes.faqItem.createAndFill();
						if (!item) return false;
						tr.insert(insertPos, item);
						// New item's summary text slot = insertPos + 2.
						tr.setSelection(
							TextSelection.near(tr.doc.resolve(insertPos + 2), 1)
						).scrollIntoView();
						return true;
					})
					.run();
			}
		};
	}
});

export const FAQ_NODES = [FaqNode, FaqItemNode, FaqSummaryNode, FaqContentNode];

declare module '@tiptap/core' {
	interface Commands<ReturnType> {
		faq: {
			insertFaq: () => ReturnType;
		};
	}
}
