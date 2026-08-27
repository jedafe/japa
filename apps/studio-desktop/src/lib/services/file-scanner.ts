/**
 * Scan projectRoot/jc-content/ for Markdown files and extract queue metadata.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { readDir, readTextFile, stat, type DirEntry } from '@tauri-apps/plugin-fs';
import { join } from '@tauri-apps/api/path';
import { parseFrontmatter } from '$lib/services/markdown';
import { hashFile } from '$lib/services/hash';
import { slugify } from '$lib/services/prompt';

export interface ScannedFile {
	relativePath: string;
	contentType: string;
	slug: string;
	title: string;
	hash: string;
	guideCategory?: string;
	spaceId?: string;
	personaId?: string;
	status?: string;
	visibility?: string;
	modifiedAt?: string;
}

function titleFromSlug(slug: string): string {
	return slug
		.replace(/[-_]+/g, ' ')
		.replace(/\s+/g, ' ')
		.trim()
		.replace(/\b\w/g, (c) => c.toUpperCase());
}

function getStringField(frontmatter: Record<string, unknown>, key: string): string | undefined {
	const value = frontmatter[key];
	if (typeof value === 'string') return value;
	return undefined;
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
		} catch (e) {
			console.warn(`Failed to read content type directory ${contentTypePath}:`, e instanceof Error ? e.message : e);
			continue;
		}

		for (const fileEntry of fileEntries) {
			if (!fileEntry.isFile) continue;
			if (!fileEntry.name.endsWith('.md') || fileEntry.name.startsWith('.')) continue;

			const relativePath = ['jc-content', contentTypeEntry.name, fileEntry.name].join('/');
			const filePath = await join(contentTypePath, fileEntry.name);
			const text = await readTextFile(filePath);
			const hash = await hashFile(text);
			const { frontmatter } = parseFrontmatter(text);

			const filenameSlug = slugify(fileEntry.name.replace(/\.md$/i, ''));
			const declaredSlug = getStringField(frontmatter, 'slug');
			const slug = declaredSlug ? slugify(declaredSlug) : filenameSlug;
			const title = getStringField(frontmatter, 'title') || titleFromSlug(slug);

			let modifiedAt: string | undefined;
			try {
				const info = await stat(filePath);
				if (info.mtime) {
					modifiedAt = info.mtime.toISOString();
				}
			} catch {
				// best-effort
			}

			result.push({
				relativePath,
				contentType: contentTypeEntry.name,
				slug,
				title,
				hash,
				guideCategory: getStringField(frontmatter, 'guide_category'),
				spaceId: getStringField(frontmatter, 'space_id'),
				personaId: getStringField(frontmatter, 'persona_id'),
				status: getStringField(frontmatter, 'status'),
				visibility: getStringField(frontmatter, 'visibility'),
				modifiedAt
			});
		}
	}

	return result.sort((a, b) => a.relativePath.localeCompare(b.relativePath));
}
