// Mobile-only view state shared by the /studio layout (bottom tab bar +
// composer FABs) and the Prompts/Create tab pages (which register their
// content-type options + setter so the layout's type-picker modal can
// drive the composer's type selection).

export interface StudioMobileTypeItem {
	id: string;
	label: string;
}

export const studioMobile = $state({
	/** Which composer column is visible on mobile — 'editor' is the main
	 *  column, 'config' the right rail (PromptRightPanel/CreateRightPanel). */
	section: 'editor' as 'editor' | 'config',
	/** Content-type options registered by the active tab page — empty while
	 *  no composer tab is mounted (also suppresses the mobile FABs). */
	typeItems: [] as StudioMobileTypeItem[],
	/** Currently selected content-type id — mirrors the composer's picker. */
	selectedTypeId: null as string | null,
	/** True when the type can no longer change (editing existing content). */
	typeLocked: false,
	/** Setter registered by the active tab page — applies the modal pick. */
	selectType: null as ((id: string) => void) | null
});

/** Clears the per-page registration — call in the registering $effect's
 *  cleanup so a stale bridge can't leak into another tab. */
export function resetStudioMobileBridge(): void {
	studioMobile.typeItems = [];
	studioMobile.selectedTypeId = null;
	studioMobile.typeLocked = false;
	studioMobile.selectType = null;
}
