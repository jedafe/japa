/**
 * Copy text to the clipboard.
 * Uses the Web Clipboard API (works inside Tauri windows).
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export async function copyToClipboard(text: string): Promise<void> {
	if (navigator.clipboard && window.isSecureContext) {
		await navigator.clipboard.writeText(text);
		return;
	}

	const textarea = document.createElement('textarea');
	textarea.value = text;
	textarea.style.position = 'fixed';
	textarea.style.opacity = '0';
	textarea.setAttribute('aria-hidden', 'true');
	document.body.appendChild(textarea);
	textarea.select();
	try {
		const ok = document.execCommand('copy');
		if (!ok) {
			throw new Error('document.execCommand("copy") returned false');
		}
	} finally {
		textarea.remove();
	}
}
