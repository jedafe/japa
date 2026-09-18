/**
 * Markdown → HTML conversion for the publish payload (body_html).
 * Ported from apps/studio-desktop/src/lib/services/markdown.ts.
 * Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md
 */

import { Marked, Renderer, type Tokens } from 'marked';
import DOMPurify from 'isomorphic-dompurify';

export interface MarkdownToHtmlOptions {
	sanitize?: boolean;
}

function escapeHtml(text: string): string {
	return text
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#039;');
}

const headingSizes: Record<number, string> = {
	1: 'text-3xl',
	2: 'text-2xl',
	3: 'text-xl',
	4: 'text-lg',
	5: 'text-base',
	6: 'text-sm'
};

class TailwindRenderer extends Renderer {
	paragraph({ tokens }: Tokens.Paragraph): string {
		return `<p class="my-2">${this.parser.parseInline(tokens)}</p>\n`;
	}

	heading({ tokens, depth }: Tokens.Heading): string {
		const sizeClass = headingSizes[depth] ?? 'text-base';
		return `<h${depth} class="font-bold my-4 ${sizeClass}">${this.parser.parseInline(tokens)}</h${depth}>\n`;
	}

	link({ href, title, tokens }: Tokens.Link): string {
		const titleAttr = title ? ` title="${escapeHtml(title)}"` : '';
		return `<a href="${escapeHtml(href)}" class="text-primary hover:underline"${titleAttr}>${this.parser.parseInline(tokens)}</a>`;
	}

	image({ href, title, text }: Tokens.Image): string {
		const titleAttr = title ? ` title="${escapeHtml(title)}"` : '';
		return `<img src="${escapeHtml(href)}" alt="${escapeHtml(text)}" class="rounded-lg my-4 max-w-full h-auto"${titleAttr} />`;
	}

	list({ items, ordered, start }: Tokens.List): string {
		const tag = ordered ? 'ol' : 'ul';
		const listClass = ordered ? 'list-decimal' : 'list-disc';
		const startAttr = ordered && start && start !== 1 ? ` start="${start}"` : '';
		const body = items.map((item) => this.listitem(item)).join('');
		return `<${tag} class="${listClass} pl-6 my-2"${startAttr}>${body}</${tag}>\n`;
	}

	listitem(item: Tokens.ListItem): string {
		const text = this.parser.parseInline(item.tokens);
		const checkbox = item.task
			? `<input type="checkbox" ${item.checked ? 'checked' : ''} disabled /> `
			: '';
		return `<li class="my-1">${checkbox}${text}</li>\n`;
	}

	code({ text, lang, escaped }: Tokens.Code): string {
		const language = (lang || '').split(/\s+/)[0];
		const languageClass = language ? `language-${escapeHtml(language)}` : '';
		const codeText = escaped ? text : escapeHtml(text);
		return `<pre class="rounded bg-surface p-3 overflow-x-auto"><code class="${languageClass}">${codeText}</code></pre>\n`;
	}

	codespan({ text }: Tokens.Codespan): string {
		return `<code class="rounded bg-surface px-1 text-sm">${escapeHtml(text)}</code>`;
	}

	blockquote({ tokens }: Tokens.Blockquote): string {
		const text = this.parser.parse(tokens);
		return `<blockquote class="border-l-4 border-border pl-4 italic text-muted-foreground my-4">${text}</blockquote>\n`;
	}

	hr(): string {
		return `<hr class="border-border my-4" />\n`;
	}

	strong({ tokens }: Tokens.Strong): string {
		return `<strong>${this.parser.parseInline(tokens)}</strong>`;
	}

	em({ tokens }: Tokens.Em): string {
		return `<em>${this.parser.parseInline(tokens)}</em>`;
	}

	del({ tokens }: Tokens.Del): string {
		return `<del>${this.parser.parseInline(tokens)}</del>`;
	}

	table({ align, header, rows }: Tokens.Table): string {
		const aligns = align || [];
		const headerCells = header
			.map((cell, i) => {
				const text = this.parser.parseInline(cell.tokens);
				const alignClass = aligns[i] ? `text-${aligns[i]}` : '';
				return `<th class="border border-border px-3 py-2 font-semibold ${alignClass}">${text}</th>`;
			})
			.join('');
		const body = rows
			.map((row) => {
				const cells = row
					.map((cell, i) => {
						const text = this.parser.parseInline(cell.tokens);
						const alignClass = aligns[i] ? `text-${aligns[i]}` : '';
						return `<td class="border border-border px-3 py-2 ${alignClass}">${text}</td>`;
					})
					.join('');
				return `<tr class="border-b border-border">${cells}</tr>\n`;
			})
			.join('');
		return `<table class="w-full text-sm my-4 border-collapse"><thead class="bg-accent"><tr>${headerCells}</tr></thead><tbody>${body}</tbody></table>\n`;
	}
}

const markedInstance = new Marked().setOptions({
	gfm: true,
	renderer: new TailwindRenderer()
});

export function markdownToHtml(markdown: string, options?: MarkdownToHtmlOptions): string {
	const html = markedInstance.parse(markdown, { async: false }) as string;
	return options?.sanitize ? DOMPurify.sanitize(html) : html;
}
