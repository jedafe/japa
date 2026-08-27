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
	// Derived aliases kept in sync by the queue store for the queue UI.
	// See Section 105 — Content Queue / Dashboard.
	lastModified?: string;
	lastPublished?: string;
	japaStatus?: string;
	localStatus?: SyncState;
}
