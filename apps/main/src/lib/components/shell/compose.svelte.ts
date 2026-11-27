/**
 * Cross-component compose-modal state.
 *
 * ADR-058: the Compose button opens a modal composer. Discussion-type posts
 * compose inline in the modal; blog posts and CPTs route to /create.
 * This store lets any component (LeftNav, MobileBottomTabBar, forum list
 * button) open the modal without prop-drilling.
 *
 * Ref: japaconnect-engineering/adr/ADR-058-main-app-shell.md
 */

export type ComposeMode = 'menu' | 'discussion';

export interface ComposeSpacePreset {
	spaceId: string;
	spaceName: string;
}

/** Poll data carried from the quick composer's inline builder into the
 *  discussion composer (expiresAt is the raw datetime-local value). */
export interface ComposePollSeed {
	question: string;
	options: string[];
	isMultiSelect: boolean;
	expiresAt: string;
}

export interface OpenDiscussionOptions {
	isQuestion?: boolean;
	presetSpace?: ComposeSpacePreset | null;
	initialTitle?: string;
	initialPoll?: ComposePollSeed | null;
	/** Space preselected but NOT locked — the picker still shows (unlike
	 *  presetSpace, which replaces the picker with a fixed chip). */
	initialSpace?: ComposeSpacePreset | null;
}

class ComposeStore {
	/** Whether the modal is open. */
	open = $state(false);
	/** Which view the modal shows — 'menu' (type selector) or 'discussion' (inline form). */
	mode = $state<ComposeMode>('menu');
	/** When mode is 'discussion', whether the composer starts in question mode. */
	isQuestion = $state(false);
	/** Space-hub preset (§05 hub-redesign) — the inline composer locks its
	 *  target to this space instead of showing the space picker. */
	presetSpace = $state<ComposeSpacePreset | null>(null);
	/** Title typed into a quick composer, carried over when it hands off
	 *  to the full discussion composer. */
	initialTitle = $state('');
	/** Poll drafted in a quick composer, carried into the discussion composer. */
	initialPoll = $state<ComposePollSeed | null>(null);
	/** Space chosen in a quick composer, preselected (not locked) in the sheet. */
	initialSpace = $state<ComposeSpacePreset | null>(null);

	/** Open the modal at the type-selector menu. */
	openMenu() {
		this.mode = 'menu';
		this.isQuestion = false;
		this.presetSpace = null;
		this.initialTitle = '';
		this.initialPoll = null;
		this.initialSpace = null;
		this.open = true;
	}

	/** Open the modal directly in the inline discussion composer. */
	openDiscussion(opts: OpenDiscussionOptions = {}) {
		this.mode = 'discussion';
		this.isQuestion = opts.isQuestion ?? false;
		this.presetSpace = opts.presetSpace ?? null;
		this.initialTitle = opts.initialTitle ?? '';
		this.initialPoll = opts.initialPoll ?? null;
		this.initialSpace = opts.initialSpace ?? null;
		this.open = true;
	}

	/** Close the modal and reset to the menu view. */
	close() {
		this.open = false;
		this.mode = 'menu';
		this.isQuestion = false;
		this.presetSpace = null;
		this.initialTitle = '';
		this.initialPoll = null;
		this.initialSpace = null;
	}
}

export const compose = new ComposeStore();
