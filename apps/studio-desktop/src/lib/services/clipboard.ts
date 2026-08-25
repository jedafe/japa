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
	document.body.appendChild(textarea);
	textarea.select();
	try {
		document.execCommand('copy');
	} finally {
		textarea.remove();
	}
}
