/**
 * Scan projectRoot/jc-content/ for Markdown files and extract queue metadata.
 */

import { readDir, readTextFile, type DirEntry } from '@tauri-apps/plugin-fs';
import { join } from '@tauri-apps/api/path';
import { slugify } from '$lib/services/prompt';
import { hashFile } from '$lib/services/hash';

export interface ScannedFile {
	relativePath: string;
	contentType: string;
	slug: string;
	title: string;
	hash: string;
	guideCategory?: string;
}

interface Frontmatter {
	title?: string;
	slug?: string;
	content_type?: string;
	guide_category?: string;
}

function titleFromSlug(slug: string): string {
	return slug
		.replace(/[-_]+/g, ' ')
		.replace(/\s+/g, ' ')
		.trim()
		.replace(/\b\w/g, (c) => c.toUpperCase());
}

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

function parseFrontmatter(text: string): Frontmatter {
	const match = text.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/);
	if (!match) return {};

	const block = match[1];
	const result: Frontmatter = {};

	for (const rawLine of block.split(/\r?\n/)) {
		const line = rawLine.trim();
		if (!line || line.startsWith('#')) continue;

		const m = line.match(/^(title|slug|content_type|guide_category):\s*(.*)$/);
		if (!m) continue;

		const key = m[1] as keyof Frontmatter;
		const value = unquote(m[2]);
		if (value) {
			result[key] = value;
		}
	}

	return result;
}

function isMissingDirectoryError(message: string): boolean {
	return /no such file|cannot find the path|cannot find the file|os error 2|not found/i.test(
		message
	);
}

export async function scanProject(projectRoot: string): Promise<ScannedFile[]> {
	const result: ScannedFile[] = [];
	const contentDir = await join(projectRoot, 'jc-content');

	let contentTypeEntries: DirEntry[] = [];
	try {
		contentTypeEntries = await readDir(contentDir);
	} catch (e) {
		const message = e instanceof Error ? e.message : String(e);
		if (isMissingDirectoryError(message)) {
			return [];
		}
		throw e;
	}

	for (const contentTypeEntry of contentTypeEntries) {
		if (!contentTypeEntry.isDirectory) continue;
		if (contentTypeEntry.name.startsWith('.')) continue;

		const contentTypePath = await join(contentDir, contentTypeEntry.name);
		let fileEntries: DirEntry[] = [];
		try {
			fileEntries = await readDir(contentTypePath);
		} catch {
			continue;
		}

		for (const fileEntry of fileEntries) {
			if (!fileEntry.isFile) continue;
			if (!fileEntry.name.endsWith('.md') || fileEntry.name.startsWith('.')) continue;

			// Valid path: jc-content/<content-type>/<slug>.md
			const relativePath = ['jc-content', contentTypeEntry.name, fileEntry.name].join('/');
			const filePath = await join(contentTypePath, fileEntry.name);
			const text = await readTextFile(filePath);
			const hash = await hashFile(text);
			const frontmatter = parseFrontmatter(text);

			const filenameSlug = slugify(fileEntry.name.replace(/\.md$/i, ''));
			const slug = frontmatter.slug ? slugify(frontmatter.slug) : filenameSlug;
			const title = frontmatter.title ? frontmatter.title : titleFromSlug(slug);

			// Validate frontmatter content_type if present, but always use directory name.
			if (frontmatter.content_type && frontmatter.content_type !== contentTypeEntry.name) {
				// Intentionally ignored: directory name is the source of truth.
			}

			result.push({
				relativePath,
				contentType: contentTypeEntry.name,
				slug,
				title,
				hash,
				guideCategory: frontmatter.guide_category
			});
		}
	}

	return result.sort((a, b) => a.relativePath.localeCompare(b.relativePath));
}
