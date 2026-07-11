<script lang="ts">
	import { locale } from '$lib/stores/i18n.svelte';
	import { Languages } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	function toggle() {
		const current = locale.current;
		const next = current === 'en' ? 'fr' : 'en';
		locale.set(next);

		const path = page.url.pathname;
		let newPath: string;

		if (next !== 'en') {
			const prefix = `/${next}`;
			if (path.startsWith('/en')) {
				newPath = prefix + path.slice(3) || prefix;
			} else if (path.startsWith('/fr')) {
				newPath = prefix + path.slice(3) || prefix;
			} else {
				newPath = prefix + path;
			}
		} else {
			const stripped = path.replace(/^\/(fr|es|pt|ar|sw|ha|yo|am|zu|sn|st|tn|ve|ts|ss|xh|nr|zu)\//, '/');
			newPath = stripped || '/';
		}

		goto(newPath);
	}
</script>

<button
	onclick={toggle}
	class="flex h-11 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
	style="color: var(--color-muted)"
	aria-label="Switch language"
>
	<Languages class="h-4 w-4" />
	<span class="text-xs font-semibold uppercase tracking-wide">{locale.current === 'en' ? 'FR' : 'EN'}</span>
</button>
