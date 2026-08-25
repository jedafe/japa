/**
 * Save project instructions to `.claude/CLAUDE.md`.
 * Uses the Tauri dialog to request the path and the Tauri fs plugin to write.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { save } from '@tauri-apps/plugin-dialog';
import { writeTextFile } from '@tauri-apps/plugin-fs';

export async function saveProjectInstruction(projectRoot: string, text: string): Promise<string> {
	const filePath = `${projectRoot}/.claude/CLAUDE.md`;

	const selected = await save({
		title: 'Save Claude project instructions',
		defaultPath: filePath
	});

	if (!selected) {
		throw new Error('Save cancelled.');
	}

	await writeTextFile(selected, text);
	return selected;
}
