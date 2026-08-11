<script lang="ts">
	/**
	 * AI Privacy settings — manages the user's `ai_exempt` flag.
	 *
	 * GET /api/v1/me/ai-privacy → { profile_ai_exempt: boolean }
	 * PUT /api/v1/me/ai-privacy { entity_type: "profile", entity_id: <self>, ai_exempt: bool }
	 *
	 * When `profile_ai_exempt` is true, the AI agent's privacy layer blocks
	 * the user's profile data from being read by agent tools. The user can
	 * also exempt individual content rows and custom field values from the
	 * content edit pages (future: per-item toggles on dashboard content list).
	 *
	 * Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Privacy Exemption)
	 * Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 10, task 10.05)
	 */
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import { Shield, ShieldCheck } from 'lucide-svelte';
	import type { AIPrivacyStatus, AIPrivacyUpdateResult } from '$lib/ai/agentTypes';

	let { data }: { data: unknown } = $props();

	// The loader returns { privacy: { profile_ai_exempt }, profile: { id } }.
	// Degrade to safe defaults when either fetch failed.
	const loaded = $derived(data as { privacy?: AIPrivacyStatus; profile?: { id?: string } } | null);
	const initial = $derived(loaded?.privacy ?? { profile_ai_exempt: false });
	const userId = $derived(loaded?.profile?.id ?? '');

	// Single-boolean form — DirtyForm provides saving/error/saved state via
	// form.saving and form.error; SettingsSection renders the save bar.
	const form = new DirtyForm(
		untrack(() => ({ profile_ai_exempt: initial.profile_ai_exempt ?? false })),
		async (v) => {
			// entity_id is the user's own ID — the backend rejects any other
			// value with 403 `not_owner`. The id comes from the /me fetch in
			// the settings loader. If it is missing (the /me fetch failed),
			// we cannot update — surface an error rather than sending a bad id.
			if (!userId) {
				const msg = t('settings.ai_privacy.no_user_id', 'Could not resolve your user ID. Please reload the page.');
				return { data: null, error: { message: msg, status: 400 }, status: 400 };
			}
			const res = await api.put<AIPrivacyUpdateResult>('/me/ai-privacy', {
				entity_type: 'profile',
				entity_id: userId,
				ai_exempt: v.profile_ai_exempt
			});
			if (res.error) {
				return { data: null, error: res.error, status: res.status };
			}
			return { data: res.data, error: null, status: res.status };
		}
	);

	function toggle(next: boolean) {
		form.set({ profile_ai_exempt: next });
	}
</script>

<SettingsSection titleKey="settings.nav.ai_privacy" descKey="settings.ai_privacy.desc" {form}>
	<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="flex items-start justify-between gap-4">
			<div class="flex items-start gap-3">
				{#if form.values.profile_ai_exempt}
					<ShieldCheck class="mt-0.5 h-5 w-5 shrink-0" style="color: var(--color-success, #16a34a)" />
				{:else}
					<Shield class="mt-0.5 h-5 w-5 shrink-0" style="color: var(--color-muted)" />
				{/if}
				<div>
					<p class="text-sm font-medium" style="color: var(--color-text)">
						{t('settings.ai_privacy.profile_exempt_title', 'Exempt my profile from AI agent access')}
					</p>
					<p class="mt-1 text-xs" style="color: var(--color-muted)">
						{t('settings.ai_privacy.profile_exempt_desc', 'When enabled, the AI agent cannot read your profile data or use it as context for tool calls. Your content and custom field values are not affected — manage those from their respective edit pages.')}
					</p>
				</div>
			</div>
			<label class="relative inline-flex shrink-0 cursor-pointer items-center">
				<input
					type="checkbox"
					class="peer sr-only"
					checked={form.values.profile_ai_exempt}
					disabled={form.saving}
					onchange={(e) => toggle(e.currentTarget.checked)}
				/>
				<div
					class="h-6 w-11 rounded-full transition-colors peer-checked:bg-[var(--color-primary)] peer-disabled:opacity-50"
					style="background-color: var(--color-muted)"
				></div>
				<div
					class="absolute left-0.5 h-5 w-5 rounded-full bg-white transition-transform peer-checked:translate-x-5"
				></div>
			</label>
		</div>
	</div>

	<!-- Info box: what is exempted -->
	<div class="mt-4 rounded-md border p-3 text-xs" style="border-color: var(--color-border); background-color: var(--color-background)">
		<p class="font-medium" style="color: var(--color-text)">
			{t('settings.ai_privacy.what_is_exempted', 'What does AI exemption do?')}
		</p>
		<ul class="mt-2 space-y-1.5" style="color: var(--color-muted)">
			<li>• {t('settings.ai_privacy.exempt_profile', 'Blocks the agent from reading your profile (users.ai_exempt)')}</li>
			<li>• {t('settings.ai_privacy.exempt_content', 'Per-content exemption is set from each content item\'s edit menu')}</li>
			<li>• {t('settings.ai_privacy.exempt_cfv', 'Per-custom-field-value exemption is set from the field value editor')}</li>
			<li>• {t('settings.ai_privacy.exempt_not_block', 'Exemption does not hide your content from normal site visitors — only from the AI agent\'s tools')}</li>
		</ul>
	</div>
</SettingsSection>
