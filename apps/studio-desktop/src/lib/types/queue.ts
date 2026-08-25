/**
 * Queue / dashboard state types.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export type SyncState = 'new' | 'modified' | 'published' | 'in_review' | 'failed' | 'archived';

export interface FileState {
	relativePath: string;
	contentType: string;
	slug: string;
	title: string;
	lastHash: string;
	lastPublishedHash: string;
	lastPublishedAt: string;
	syncState: SyncState;
	lastError: string;
	contentId?: string;
	spaceId?: string;
	personaId?: string;
	guideCategory?: string;
	status?: string;
	visibility?: string;
	modifiedAt?: string;
}
