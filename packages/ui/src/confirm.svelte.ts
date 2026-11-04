// Shared promise-based confirm dialog for all JapaConnect apps — replaces
// native window.confirm() calls. Mount <ConfirmHost/> once in the root
// layout, then `if (!(await confirmDialog('...'))) return;` anywhere.

export interface ConfirmOptions {
	/** Label for the affirmative button. Default "Confirm". */
	confirmLabel?: string;
	/** Label for the dismissive button. Default "Cancel". */
	cancelLabel?: string;
	/** Dialog heading. Default "Confirm". */
	title?: string;
	/** Render the confirm button in the danger colour. Default true — most
	 * call sites guard destructive actions. */
	destructive?: boolean;
}

interface ConfirmRequest extends Required<ConfirmOptions> {
	message: string;
	resolve: (ok: boolean) => void;
}

let pending = $state<ConfirmRequest | null>(null);

export function getConfirmRequest(): ConfirmRequest | null {
	return pending;
}

export function confirmDialog(message: string, opts: ConfirmOptions = {}): Promise<boolean> {
	// A second call while one is open supersedes it — the earlier promise
	// resolves false so its caller takes the safe (non-destructive) branch.
	pending?.resolve(false);
	return new Promise((resolve) => {
		pending = {
			message,
			confirmLabel: opts.confirmLabel ?? 'Confirm',
			cancelLabel: opts.cancelLabel ?? 'Cancel',
			title: opts.title ?? 'Confirm',
			destructive: opts.destructive ?? true,
			resolve
		};
	});
}

export function answerConfirm(ok: boolean): void {
	const req = pending;
	pending = null;
	req?.resolve(ok);
}
