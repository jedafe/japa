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

class ComposeStore {
	/** Whether the modal is open. */
	open = $state(false);
	/** Which view the modal shows — 'menu' (type selector) or 'discussion' (inline form). */
	mode = $state<ComposeMode>('menu');
	/** When mode is 'discussion', whether the composer starts in question mode. */
	isQuestion = $state(false);

	/** Open the modal at the type-selector menu. */
	openMenu() {
		this.mode = 'menu';
		this.isQuestion = false;
		this.open = true;
	}

	/** Open the modal directly in the inline discussion composer. */
	openDiscussion(isQuestion = false) {
		this.mode = 'discussion';
		this.isQuestion = isQuestion;
		this.open = true;
	}

	/** Close the modal and reset to the menu view. */
	close() {
		this.open = false;
		this.mode = 'menu';
		this.isQuestion = false;
	}
}

export const compose = new ComposeStore();
