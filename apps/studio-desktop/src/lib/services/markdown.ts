/**
 * Minimal Markdown helpers for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export interface ParsedMarkdown {
	frontmatter: string;
	body: string;
	frontmatterFields: Record<string, string>;
}

const FRONTMATTER_RE = /^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/;

function unquote(value: string): string {
	const trimmed = value.trim();
	if (
		(trimmed.startsWith('"') && trimmed.endsWith('"')) ||
		(trimmed.startsWith("'") && trimmed.endsWith("'"))
	) {
		return trimmed.slice(1, -1).trim();
	}
	return trimmed;
}

export function parseFrontmatter(text: string): ParsedMarkdown {
	const match = text.match(FRONTMATTER_RE);
	if (!match) {
		return { frontmatter: '', body: text, frontmatterFields: {} };
	}

	const block = match[1];
	const body = text.slice(match[0].length);
	const frontmatterFields: Record<string, string> = {};

	for (const rawLine of block.split(/\r?\n/)) {
		const line = rawLine.trim();
		if (!line || line.startsWith('#')) continue;

		const idx = line.indexOf(':');
		if (idx === -1) continue;

		const key = line.slice(0, idx).trim();
		const value = unquote(line.slice(idx + 1));
		if (key) {
			frontmatterFields[key] = value;
		}
	}

	return { frontmatter: match[0], body, frontmatterFields };
}

export function setFrontmatterField(text: string, key: string, value: string): string {
	const match = text.match(FRONTMATTER_RE);
	if (!match) {
		const fields = `---\n${key}: ${value}\n---\n`;
		return fields + text;
	}

	const block = match[1];
	const lines = block.split(/\r?\n/);
	let found = false;
	const updated = lines
		.map((rawLine) => {
			const line = rawLine.trim();
			if (!line) return rawLine;

			const idx = line.indexOf(':');
			if (idx === -1) return rawLine;

			const lineKey = line.slice(0, idx).trim();
			if (lineKey === key) {
				found = true;
				return `${key}: ${value}`;
			}
			return rawLine;
		})
		.filter((line) => line !== null) as string[];

	if (!found) {
		updated.push(`${key}: ${value}`);
	}

	const newFrontmatter = `---\n${updated.join('\n')}\n---`;
	return text.replace(FRONTMATTER_RE, newFrontmatter);
}

export function buildMarkdown(frontmatter: Record<string, string>, body: string): string {
	const lines = Object.entries(frontmatter)
		.filter(([, value]) => value !== '' && value !== undefined && value !== null)
		.map(([key, value]) => {
			const safe = String(value).replace(/"/g, '\\"');
			return `${key}: "${safe}"`;
		});
	return `---\n${lines.join('\n')}\n---\n\n${body.trim()}\n`;
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
