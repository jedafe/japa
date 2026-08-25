/**
 * Queue / dashboard state types.
 * Ref: content queue dashboard for Japa Studio desktop.
 */

export type SyncState = 'new' | 'modified' | 'published' | 'in_review' | 'failed' | 'archived';

export interface FileState {
	relativePath: string;
	contentType: string;
	slug: string;
	title: string;
	guideCategory?: string;
	lastHash: string;
	lastPublishedHash: string;
	lastPublishedAt: string;
	syncState: SyncState;
	lastError: string;
	contentId?: string;
}
