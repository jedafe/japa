<script lang="ts">
	import PageHeader from "$lib/components/layout/page-header.svelte";
	import { Button } from "$lib/components/ui/button";
	import { seoAdminApi, type SeoValidationResult } from "$lib/api";
	import { t } from "$lib/stores/i18n.svelte";
	import { hasPermission } from "$lib/stores/permissions.svelte";
	import { Search, AlertTriangle, CheckCircle2, RefreshCw, Globe } from "lucide-svelte";

	const canManage = $derived(hasPermission("admin.settings"));

	let contentId = $state("");
	let loading = $state(false);
	let errorMessage = $state("");
	let result = $state<SeoValidationResult | null>(null);
	let hasValidated = $state(false);

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

	async function validate() {
		const id = contentId.trim();
		if (!id) {
			errorMessage = t("seo.enter_id", "Please enter a content or page ID");
			return;
		}
		loading = true;
		errorMessage = "";
		result = null;
		hasValidated = true;
		try {
			const res = await seoAdminApi.validate(id);
			if (res.ok) {
				result = res.data.data ?? null;
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
			validate();
		}
	}

	function clearResult() {
		result = null;
		hasValidated = false;
		errorMessage = "";
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
	<div class="mb-6 rounded-xl border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<label for="seo-content-id" class="mb-2 block text-sm font-medium" style="color: var(--color-foreground)">
			{t("seo.content_id_label", "Content or Page ID")}
		</label>
		<div class="flex flex-col gap-3 sm:flex-row">
			<div class="relative flex-1">
				<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
				<input
					id="seo-content-id"
					type="text"
					bind:value={contentId}
					onkeydown={handleKeydown}
					placeholder={t("seo.content_id_placeholder", "Enter a content, page, or space ID...")}
					class="h-10 w-full rounded-lg border pl-9 pr-3 text-sm"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
				/>
			</div>
			<Button onclick={validate} disabled={loading}>
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
	</div>

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
		<div class="mb-6 flex items-center gap-6 rounded-xl border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
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
		</div>

		<!-- Issues list -->
		<div class="mb-6 rounded-xl border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
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
					{#each result.issues as issue}
						<li class="flex items-start gap-2 text-sm" style="color: var(--color-foreground)">
							<AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" style="color: var(--color-warning)" />
							<span class="font-mono text-xs">{issue}</span>
						</li>
					{/each}
				</ul>
			{/if}
		</div>

		<!-- Metadata fields grid -->
		<div class="rounded-xl border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h3 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">
				{t("seo.metadata_fields", "Metadata Fields")}
			</h3>
			<div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
				{#each META_FIELDS as field}
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
		</div>
	{:else if !hasValidated}
		<div class="flex flex-col items-center justify-center py-16 text-center">
			<Globe class="h-10 w-10" style="color: var(--color-muted)" />
			<p class="mt-3 text-sm" style="color: var(--color-muted)">
				{t("seo.empty_hint", "Enter a content or page ID above and click Validate to check its SEO metadata.")}
			</p>
		</div>
	{/if}
{/if}
