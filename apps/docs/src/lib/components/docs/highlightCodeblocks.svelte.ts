/**
 * Post-render code-block enhancement (browser-only, dynamically imported).
 *
 * Wraps bare <pre><code> blocks produced by the TipTap → HTML renderer in the
 * CodeBlock chrome (language badge + copy button). Language is inferred from
 * the code element's class (e.g. `language-go`) or a `data-language` attribute.
 *
 * ADR-060: highlighting is progressive enhancement — failures degrade silently
 * to the unstyled <pre> block.
 */
import { t } from '$lib/i18n';

export function enhanceCodeblocks(root: HTMLElement): void {
	const blocks = root.querySelectorAll<HTMLPreElement>('pre');
	for (const pre of blocks) {
		if (pre.closest('.jc-code')) continue; // already enhanced
		const code = pre.querySelector('code');
		if (!code) continue;

		const lang = inferLang(code);

		const wrapper = document.createElement('div');
		wrapper.className = 'jc-code';

		const header = document.createElement('div');
		header.className = 'jc-code-header';

		const langLabel = document.createElement('span');
		langLabel.className = 'jc-code-lang';
		langLabel.textContent = lang;
		header.appendChild(langLabel);

		const copyBtn = document.createElement('button');
		copyBtn.type = 'button';
		copyBtn.className = 'jc-code-copy';
		copyBtn.setAttribute('aria-label', t('a11y.copy_code'));
		copyBtn.textContent = t('a11y.copy_code');
		copyBtn.addEventListener('click', async () => {
			try {
				await navigator.clipboard.writeText(code.textContent ?? '');
				copyBtn.classList.add('copied');
				copyBtn.textContent = t('a11y.code_copied');
				setTimeout(() => {
					copyBtn.classList.remove('copied');
					copyBtn.textContent = t('a11y.copy_code');
				}, 1500);
			} catch {
				/* clipboard unavailable */
			}
		});
		header.appendChild(copyBtn);

		pre.parentNode?.insertBefore(wrapper, pre);
		wrapper.appendChild(header);
		wrapper.appendChild(pre);
	}
}

function inferLang(code: HTMLElement): string {
	const cls = code.className;
	const match = /language-([\w-]+)/.exec(cls);
	if (match) return match[1];
	const dataLang = code.getAttribute('data-language');
	if (dataLang) return dataLang;
	return '';
}
