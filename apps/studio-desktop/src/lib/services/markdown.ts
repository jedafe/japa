/**
 * Markdown / frontmatter helpers for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import * as yaml from 'js-yaml';
import { Marked, Renderer, type Tokens } from 'marked';
import DOMPurify from 'isomorphic-dompurify';

const FRONTMATTER_RE = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/;

export interface ParsedMarkdown {
	frontmatter: Record<string, unknown>;
	body: string;
}

export interface MarkdownToHtmlOptions {
	sanitize?: boolean;
}

export function parseFrontmatter(text: string): ParsedMarkdown {
	const match = text.match(FRONTMATTER_RE);
	if (!match) {
		return { frontmatter: {}, body: text };
	}

	try {
		const frontmatter = (yaml.load(match[1]) as Record<string, unknown>) || {};
		const body = text.slice(match[0].length);
		return { frontmatter, body };
	} catch {
		return { frontmatter: {}, body: text };
	}
}

export function buildMarkdown(frontmatter: Record<string, unknown>, body: string): string {
	const dumped = yaml.dump(frontmatter, { lineWidth: -1, noArrayIndent: false }).trim();
	return `---\n${dumped}\n---\n\n${body.trim()}\n`;
}

export function setFrontmatterField(text: string, key: string, value: unknown): string {
	const parsed = parseFrontmatter(text);
	parsed.frontmatter[key] = value;
	return buildMarkdown(parsed.frontmatter, parsed.body);
}

export function getFrontmatterField<T>(
	frontmatter: Record<string, unknown>,
	key: string,
	defaultValue: T
): T {
	const value = frontmatter[key];
	return value === undefined || value === null ? defaultValue : (value as T);
}

function escapeHtml(text: string): string {
	return text
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;')
		.replace(/'/g, '&#039;');
}

/** Scheme allowlist for markdown link/image targets. escapeHtml alone
 *  cannot stop [x](javascript:alert(1)) — the scheme executes on click
 *  even when quoted. Only http(s), site-relative, anchors, and mailto
 *  survive; the publish path also runs DOMPurify (sanitize:true). */
function safeMarkdownUrl(raw: string): string {
	const v = String(raw ?? '').trim();
	if (
		v === '' ||
		v.startsWith('#') ||
		v.startsWith('/') ||
		v.startsWith('./') ||
		v.startsWith('../')
	) {
		return v === '' ? '#' : v;
	}
	if (/^(https?|mailto):/i.test(v)) return v;
	return '#';
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
		return `<a href="${escapeHtml(safeMarkdownUrl(href))}" class="text-primary hover:underline"${titleAttr}>${this.parser.parseInline(tokens)}</a>`;
	}

	image({ href, title, text }: Tokens.Image): string {
		const titleAttr = title ? ` title="${escapeHtml(title)}"` : '';
		return `<img src="${escapeHtml(safeMarkdownUrl(href))}" alt="${escapeHtml(text)}" class="rounded-lg my-4 max-w-full h-auto"${titleAttr} />`;
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
