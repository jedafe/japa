/**
 * Markdown / frontmatter helpers for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import * as yaml from 'js-yaml';

const FRONTMATTER_RE = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/;

export interface ParsedMarkdown {
	frontmatter: Record<string, unknown>;
	body: string;
}

export function parseFrontmatter(text: string): ParsedMarkdown {
	const match = text.match(FRONTMATTER_RE);
	if (!match) {
		return { frontmatter: {}, body: text };
	}

	try {
		const frontmatter = yaml.load(match[1]) as Record<string, unknown> || {};
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

export function setFrontmatterField(
	text: string,
	key: string,
	value: unknown
): string {
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

export function markdownToHtml(markdown: string): string {
	const trimmed = markdown.trim();
	if (!trimmed) return '';

	let html = trimmed;

	html = html.replace(/```([\w-]*)\n([\s\S]*?)```/g, (_, lang, code) => {
		return `<pre class="rounded bg-surface p-3 overflow-x-auto"><code class="language-${lang}">${escapeHtml(code)}</code></pre>`;
	});

	html = html.replace(/^###### (.*$)/gim, '<h6>$1</h6>');
	html = html.replace(/^##### (.*$)/gim, '<h5>$1</h5>');
	html = html.replace(/^#### (.*$)/gim, '<h4>$1</h4>');
	html = html.replace(/^### (.*$)/gim, '<h3>$1</h3>');
	html = html.replace(/^## (.*$)/gim, '<h2>$1</h2>');
	html = html.replace(/^# (.*$)/gim, '<h1>$1</h1>');

	html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1" class="rounded-lg my-4 max-w-full" />');
	html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" class="text-primary hover:underline">$1</a>');

	html = html.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
	html = html.replace(/__(.*?)__/g, '<strong>$1</strong>');
	html = html.replace(/\*(.*?)\*/g, '<em>$1</em>');
	html = html.replace(/_(.*?)_/g, '<em>$1</em>');
	html = html.replace(/`(.*?)`/g, '<code class="rounded bg-surface px-1 text-sm">$1</code>');

	const lines = html.split(/\r?\n/);
	const out: string[] = [];
	let inList = false;

	for (const line of lines) {
		const listMatch = line.match(/^(\s*)[-*]\s+(.*)$/);
		if (listMatch) {
			if (!inList) {
				out.push('<ul class="list-disc pl-6 my-2">');
				inList = true;
			}
			out.push(`<li>${listMatch[2]}</li>`);
			continue;
		}

		if (inList) {
			out.push('</ul>');
			inList = false;
		}

		if (line.trim() === '') {
			out.push('<br />');
		} else if (!line.startsWith('<')) {
			out.push(`<p class="my-2">${line}</p>`);
		} else {
			out.push(line);
		}
	}

	if (inList) out.push('</ul>');

	return out.join('\n');
}
