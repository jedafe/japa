/**
 * Markdown / frontmatter helpers for the web prompts tab.
 * Ported from apps/studio-desktop/src/lib/services/markdown.ts (no Tauri deps).
 * Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md
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
