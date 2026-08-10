<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Button } from "$lib/components/ui/button";
	import { Card, CardContent } from "$lib/components/ui/card";
	import { seoAdminApi, contentSearchApi, type SeoValidationResult, type ContentSearchItem } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Search, AlertTriangle, CheckCircle2, RefreshCw, Globe, Link2 } from "lucide-svelte";

	const canManage = $derived(hasPermission("admin.settings"));

	let query = $state("");
	let selectedId = $state("");
	let loading = $state(false);
	let searching = $state(false);
	let errorMessage = $state("");
	let result = $state<SeoValidationResult | null>(null);
	let hasValidated = $state(false);
	let suggestions = $state<ContentSearchItem[]>([]);
	let showDropdown = $state(false);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	const META_FIELDS = [
		{ key: "meta_title", label: t("seo.meta_title", "Meta Title"), maxLength: 60 },
		{ key: "meta_description", label: t("seo.meta_description", "Meta Description"), maxLength: 160 },
		{ key: "og_title", label: t("seo.og_title", "OG Title"), maxLength: 60 },
		{ key: "og_description", label: t("seo.og_description", "OG Description"), maxLength: 160 },
		{ key: "og_image_url", label: t("seo.og_image_url", "OG Image URL"), maxLength: null },
		{ key: "canonical_url", label: t("seo.canonical_url", "Canonical URL"), maxLength: null },
		{ key: "json_ld_type", label: t("seo.json_ld_type", "JSON-LD Type"), maxLength: null },
		{ key: "robots_meta", label: t("seo.robots_meta", "Robots Meta"), maxLength: null }
	] as const;

	function getField(value: string | undefined | null): string {
		return value ?? "";
	}

	function scoreColor(score: number): string {
		if (score >= 80) return "var(--color-success)";
		if (score >= 50) return "var(--color-warning)";
		return "var(--color-danger)";
	}

	// UUID v4 regex — matches the gen_random_uuid() format used by all
	// content IDs. Used to detect when the user pasted a raw content ID
	// vs. a URL or search term.
	const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

	// extractIdFromUrl parses a pasted frontend URL and returns the content
	// slug if the URL points at a content-bearing route, or null if it's not
	// a content URL. Supported patterns (per apps/main route tree):
	//   /content/{slug}   /blog/{slug}   /forum/{slug}
	// The slug is resolved to an id via the content search endpoint (which
	// is authed and returns drafts, unlike the public GET /content/{slug}).
	function extractSlugFromUrl(input: string): string | null {
		let path = input.trim();
		try {
			// If it's a full URL, take just the pathname.
			if (path.startsWith("http://") || path.startsWith("https://")) {
				path = new URL(path).pathname;
			}
		} catch {
			return null;
		}
		// Strip leading slash and the (app) route group if present.
		// Matches /content/{slug}, /blog/{slug}, /forum/{slug} (with or
		// without the (app) SvelteKit route group prefix).
		const m = path.match(/^\/?(?:\(app\)\/)?(?:content|blog|forum)\/([^/]+)$/);
		return m ? decodeURIComponent(m[1]) : null;
	}

	async function searchContent(q: string) {
		const term = q.trim();
		if (!term || term.length < 2) {
			suggestions = [];
			showDropdown = false;
			return;
		}
		searching = true;
		try {
			const res = await contentSearchApi.search(term, 10);
			if (res.ok) {
				suggestions = res.data ?? [];
				showDropdown = suggestions.length > 0;
			} else {
				suggestions = [];
				showDropdown = false;
			}
		} catch {
			suggestions = [];
			showDropdown = false;
		} finally {
			searching = false;
		}
	}

	function handleInput(e: Event) {
		const target = e.target as HTMLInputElement;
		query = target.value;
		// Typing clears any previously-selected item — the displayed text
		// is now a search term, not a pinned selection.
		selectedId = "";
		// If the user typed/pasted a raw UUID, don't show the search dropdown —
		// they can just hit Validate. Same for a pasted URL (we resolve it on
		// validate, not on every keystroke).
		const isUuid = UUID_RE.test(query.trim());
		const isUrl = query.trim().startsWith("http://") || query.trim().startsWith("https://") || query.trim().startsWith("/");
		if (isUuid || isUrl) {
			suggestions = [];
			showDropdown = false;
			if (debounceTimer) clearTimeout(debounceTimer);
			return;
		}
		if (debounceTimer) clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => searchContent(query), 250);
	}

	function selectSuggestion(item: ContentSearchItem) {
		query = item.title || item.slug;
		selectedId = item.id;
		showDropdown = false;
		suggestions = [];
		validate(item.id);
	}

	async function validate(idOverride?: string) {
		const raw = (idOverride ?? query).trim();
		if (!raw) {
			errorMessage = t("seo.enter_id", "Please enter a content or page ID, URL, or search term");
			return;
		}
		let id = raw;
		// If the input is a UUID, validate directly.
		if (!UUID_RE.test(id)) {
			// If it's a URL, extract the slug and resolve via search.
			const slug = extractSlugFromUrl(id);
			if (slug) {
				loading = true;
				errorMessage = "";
				result = null;
				hasValidated = true;
				try {
					const res = await contentSearchApi.search(slug, 10);
					if (res.ok && res.data && res.data.length > 0) {
						// Prefer an exact slug match; otherwise take the first.
						const exact = res.data.find((c) => c.slug === slug);
						id = (exact ?? res.data[0]).id;
					} else {
						errorMessage = t("seo.url_not_found", "No content found for that URL");
						loading = false;
						return;
					}
				} catch (e) {
					errorMessage = e instanceof Error ? e.message : t("seo.validate_failed", "Failed to validate SEO");
					loading = false;
					return;
				}
			} else if (!idOverride) {
				// Not a UUID and not a URL. If a suggestion was selected from
				// the dropdown, selectedId holds its id — validate that.
				// Otherwise treat the input as a search term: if we have
				// suggestions, validate the first one; else run a fresh
				// search and validate the first hit.
				if (selectedId) {
					id = selectedId;
				} else if (suggestions.length > 0) {
					const first = suggestions[0];
					id = first.id;
					selectedId = first.id;
					query = first.title || first.slug;
				} else {
					loading = true;
					errorMessage = "";
					result = null;
					hasValidated = true;
					try {
						const res = await contentSearchApi.search(raw, 10);
						if (res.ok && res.data && res.data.length > 0) {
							const first = res.data[0];
							id = first.id;
							selectedId = first.id;
							query = first.title || first.slug;
						} else {
							errorMessage = t("seo.no_match", "No content matched your search");
							loading = false;
							return;
						}
					} catch (e) {
						errorMessage = e instanceof Error ? e.message : t("seo.validate_failed", "Failed to validate SEO");
						loading = false;
						return;
					}
				}
			}
		}
		loading = true;
		errorMessage = "";
		result = null;
		hasValidated = true;
		try {
			const res = await seoAdminApi.validate(id);
			if (res.ok) {
				result = res.data ?? null;
				if (!result) {
					errorMessage = t("seo.no_result", "No validation result returned");
				}
			} else {
				errorMessage = res.error;
			}
		} catch (e) {
			errorMessage = e instanceof Error ? e.message : t("seo.validate_failed", "Failed to validate SEO");
		} finally {
			loading = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === "Enter") {
			e.preventDefault();
			showDropdown = false;
			validate();
		} else if (e.key === "Escape") {
			showDropdown = false;
		}
	}

	function clearResult() {
		result = null;
		hasValidated = false;
		errorMessage = "";
		query = "";
		selectedId = "";
		suggestions = [];
		showDropdown = false;
	}
</script>

<svelte:head>
	<title>{t("seo.title", "SEO Validation")} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t("seo.title", "SEO Validation")}
	description={t("seo.description", "Validate SEO metadata for content and pages")}
/>

{#if !canManage}
	<div class="flex items-center justify-center py-12">
		<p class="text-sm" style="color: var(--color-muted)">{t("common.access_denied", "You do not have permission to access this page")}</p>
	</div>
{:else}
	<!-- Search / input card -->
	<Card class="mb-6">
		<CardContent class="p-5">
			<label for="seo-content-id" class="mb-2 block text-sm font-medium" style="color: var(--color-foreground)">
				{t("seo.content_id_label", "Content ID, URL, or search")}
			</label>
			<p class="mb-3 text-xs" style="color: var(--color-muted)">
				{t("seo.input_hint", "Paste a content ID, a frontend URL (e.g. /content/my-post), or type a title to search.")}
			</p>
			<div class="flex flex-col gap-3 sm:flex-row">
				<div class="relative flex-1">
					<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
					<input
						id="seo-content-id"
						type="text"
						bind:value={query}
						oninput={handleInput}
						onkeydown={handleKeydown}
						onfocus={() => { if (suggestions.length > 0) showDropdown = true; }}
						onblur={() => { setTimeout(() => showDropdown = false, 200); }}
						placeholder={t("seo.content_id_placeholder", "Content ID, URL, or search term...")}
						class="h-10 w-full rounded-lg border pl-9 pr-3 text-sm"
						style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
					/>
					{#if searching}
						<RefreshCw class="absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 animate-spin" style="color: var(--color-muted)" />
					{/if}
					{#if showDropdown && suggestions.length > 0}
						<ul class="absolute z-[60] mt-1 max-h-64 w-full overflow-auto rounded-lg border shadow-lg" style="border-color: var(--color-border); background: var(--color-surface)">
							{#each suggestions as item (item)}
								<li>
									<button
										type="button"
										class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-black/5 dark:hover:bg-white/10"
										style="color: var(--color-foreground)"
										onmousedown={() => selectSuggestion(item)}
									>
										<Link2 class="h-3.5 w-3.5 shrink-0" style="color: var(--color-muted)" />
										<span class="min-w-0 flex-1 truncate">{item.title || item.slug}</span>
										<span class="shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted)">{item.content_type}</span>
									</button>
								</li>
							{/each}
						</ul>
					{/if}
				</div>
				<Button onclick={() => validate()} disabled={loading || searching}>
					{#if loading}
						<RefreshCw class="h-3.5 w-3.5 animate-spin" />
						{t("seo.validating", "Validating...")}
					{:else}
						<Search class="h-3.5 w-3.5" />
						{t("seo.validate", "Validate")}
					{/if}
				</Button>
				{#if hasValidated}
					<Button variant="outline" onclick={clearResult}>
						{t("common.clear", "Clear")}
					</Button>
				{/if}
			</div>
		</CardContent>
	</Card>

	{#if errorMessage}
		<div class="mb-4 flex items-center gap-2 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			<AlertTriangle class="h-4 w-4 shrink-0" />
			<span>{errorMessage}</span>
		</div>
	{/if}

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<RefreshCw class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
			<span class="ml-2 text-sm" style="color: var(--color-muted)">{t("seo.validating", "Validating...")}</span>
		</div>
	{:else if result}
		<!-- Score indicator -->
		<Card class="mb-6">
			<CardContent class="flex items-center gap-6 p-6">
				<div class="relative flex h-24 w-24 shrink-0 items-center justify-center">
					<svg class="h-24 w-24 -rotate-90" viewBox="0 0 100 100">
						<circle cx="50" cy="50" r="42" fill="none" stroke="var(--color-border)" stroke-width="8" />
						<circle
							cx="50" cy="50" r="42" fill="none" stroke={scoreColor(result.score)} stroke-width="8"
							stroke-linecap="round"
							stroke-dasharray={`${(result.score / 100) * 2 * Math.PI * 42} ${2 * Math.PI * 42}`}
						/>
					</svg>
					<span class="absolute text-2xl font-bold" style="color: {scoreColor(result.score)}">{result.score}</span>
				</div>
				<div class="min-w-0 flex-1">
					<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t("seo.seo_score", "SEO Score")}</h2>
					<p class="mt-1 text-sm" style="color: var(--color-muted)">
						{#if result.issues.length === 0}
							{t("seo.no_issues", "No issues detected — SEO metadata looks good")}
						{:else}
							{result.issues.length} {t("seo.issues_found", "issue(s) found")}
						{/if}
					</p>
					<div class="mt-2 flex flex-wrap items-center gap-2 text-xs">
						<span class="rounded-full px-2 py-0.5 font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-foreground)">
							{t("seo.type", "Type")}: {result.type}
						</span>
						<span class="rounded-full px-2 py-0.5 font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-foreground)">
							{t("seo.slug", "Slug")}: {result.slug || "—"}
						</span>
					</div>
				</div>
			</CardContent>
		</Card>

		<!-- Issues list -->
		<Card class="mb-6">
			<CardContent class="p-5">
				<h3 class="mb-3 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">
					{t("seo.issues", "Issues")}
				</h3>
				{#if result.issues.length === 0}
					<div class="flex items-center gap-2 text-sm" style="color: var(--color-success)">
						<CheckCircle2 class="h-4 w-4" />
						{t("seo.no_issues", "No issues detected — SEO metadata looks good")}
					</div>
				{:else}
					<ul class="space-y-2">
						{#each result.issues as issue (issue)}
							<li class="flex items-start gap-2 text-sm" style="color: var(--color-foreground)">
								<AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" style="color: var(--color-warning)" />
								<span class="font-mono text-xs">{issue}</span>
							</li>
						{/each}
					</ul>
				{/if}
			</CardContent>
		</Card>

		<!-- Metadata fields grid -->
		<Card>
			<CardContent class="p-5">
				<h3 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">
					{t("seo.metadata_fields", "Metadata Fields")}
				</h3>
				<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
					{#each META_FIELDS as field (field)}
						{@const value = getField((result as unknown as Record<string, string>)[field.key])}
						{@const present = value.length > 0}
						<div class="rounded-lg border p-3" style="border-color: var(--color-border); background: var(--color-background)">
							<div class="mb-1 flex items-center justify-between">
								<span class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{field.label}</span>
								{#if present}
									<CheckCircle2 class="h-3.5 w-3.5" style="color: var(--color-success)" />
								{:else}
									<AlertTriangle class="h-3.5 w-3.5" style="color: var(--color-danger)" />
								{/if}
							</div>
							<p class="break-words text-sm" style="color: {present ? 'var(--color-foreground)' : 'var(--color-muted)'}">
								{present ? value : t("seo.missing", "Missing")}
							</p>
							<div class="mt-1 flex items-center gap-2 text-[10px]" style="color: var(--color-muted)">
								<span>{present ? `${value.length} ${t("seo.chars", "chars")}` : t("seo.not_set", "not set")}</span>
								{#if field.maxLength && present}
									<span style="color: {value.length > field.maxLength ? 'var(--color-danger)' : 'var(--color-muted)'}">
										/ {field.maxLength} {t("seo.max", "max")}
									</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</CardContent>
		</Card>
	{:else if !hasValidated}
		<div class="flex flex-col items-center justify-center py-16 text-center">
			<Globe class="h-10 w-10" style="color: var(--color-muted)" />
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				{t("seo.empty_hint", "Paste a content ID or frontend URL above, or type a title to search and validate its SEO metadata.")}
			</p>
		</div>
	{/if}
{/if}
