package rbac

import (
	"fmt"
	"strings"
)

// describe.go turns RBAC permission nodes into user-facing phrases so 403
// responses can name the action the account is missing ("add reactions",
// "manage API keys") instead of a bare "no permission" string. Node names
// describe the action the caller just attempted — revealing the phrase leaks
// nothing about who holds the node or how it is granted.

// permissionVerbs maps an action token to its verb phrase. Snake_case action
// tokens are split first; scope suffixes (own/any/all/self) and the "hard_"
// prefix are stripped before lookup.
var permissionVerbs = map[string]string{
	"list": "list", "view": "view", "read": "read", "create": "create",
	"add": "add", "new": "create", "update": "update", "edit": "edit",
	"delete": "delete", "remove": "remove", "suspend": "suspend",
	"unsuspend": "unsuspend", "ban": "ban", "unban": "unban",
	"approve": "approve", "reject": "reject", "accept": "accept",
	"decline": "decline", "publish": "publish", "unpublish": "unpublish",
	"moderate": "moderate", "pin": "pin", "unpin": "unpin",
	"lock": "lock", "unlock": "unlock", "share": "share",
	"convert": "convert", "manage": "manage", "impersonate": "impersonate users",
	"export": "export", "import": "import", "invite": "invite",
	"respond": "respond to", "upload": "upload", "download": "download",
	"use": "use", "promote": "promote", "demote": "demote",
	"seed": "seed", "sync": "sync", "refund": "refund",
	"review": "review", "feature": "feature", "unfeature": "unfeature",
	"verify": "verify", "unverify": "unverify", "merge": "merge",
	"reorder": "reorder", "clear": "clear", "archive": "archive",
	"unarchive": "unarchive", "restore": "restore", "send": "send",
	"schedule": "schedule", "cancel": "cancel", "complete": "complete",
	"transfer": "transfer", "withdraw": "withdraw", "configure": "configure",
	"toggle": "toggle", "enable": "enable", "disable": "disable",
	"grant": "grant", "revoke": "revoke", "generate": "generate",
	"translate": "translate", "assign": "assign", "unassign": "unassign",
	"react": "react to", "comment": "comment on", "reply": "reply to",
	"follow": "follow", "unfollow": "unfollow", "join": "join",
	"leave": "leave", "flag": "flag", "report": "report",
	"claim": "claim", "redeem": "redeem", "purchase": "purchase",
	"enroll": "enroll in", "unenroll": "unenroll from", "submit": "submit",
	"rate": "rate", "vote": "vote on", "react_to": "react to",
	"reindex": "reindex", "rebuild": "rebuild", "flush": "flush",
	"purge": "purge", "rotate": "rotate", "reset": "reset",
	"run": "run", "trigger": "trigger", "execute": "execute",
	"apply": "apply for", "process": "process", "payout": "pay out",
	"earn": "earn", "spend": "spend", "topup": "top up",
	"donate": "donate", "tip": "tip", "boost": "boost",
	"bookmark": "bookmark", "mute": "mute", "unmute": "unmute",
	"block": "block", "unblock": "unblock", "hide": "hide",
	"unhide": "unhide", "mark": "mark", "resolve": "resolve",
	"close": "close", "reopen": "reopen", "start": "start",
	"stop": "stop", "attach": "attach", "detach": "detach",
	"link": "link", "unlink": "unlink", "connect": "connect",
	"disconnect": "disconnect", "sign": "sign", "countersign": "countersign",
	"attest": "attest", "certify": "certify", "endorse": "endorse",
	"recommend": "recommend", "suggest": "suggest", "search": "search",
	"filter": "filter", "sort": "sort", "aggregate": "aggregate",
	"analyze": "analyze", "track": "track", "monitor": "monitor",
	"audit": "audit", "inspect": "inspect", "probe": "probe",
	"test": "test", "simulate": "simulate", "preview": "preview",
	"duplicate": "duplicate", "clone": "clone", "copy": "copy",
	"move": "move", "rename": "rename", "reorder_children": "reorder",
	"take": "take", "retake": "retake", "attempt": "attempt",
	"answer": "answer", "ask": "ask", "host": "host",
	"attend": "attend", "register": "register for", "unregister": "unregister from",
	"checkin": "check in to", "checkout": "check out",
	"nominate": "nominate", "elect": "elect", "appoint": "appoint",
	"dismiss": "dismiss", "hire": "hire", "fire": "fire",
	"issue": "issue", "revoke_access": "revoke access to",
	"moderation": "access moderation tools", "broadcast": "send broadcasts",
	"settings": "change settings", "analytics": "view analytics",
	"dashboard": "view dashboards", "logs": "view logs",
	"stats": "view stats", "insights": "view insights",
	"set": "set", "retry": "retry", "assist": "assist with", "award": "award",
	"save": "save", "subscribe": "subscribe to", "unsubscribe": "unsubscribe from",
	"post": "post", "fetch": "fetch", "receive": "receive",
	"request": "request", "browse": "browse", "crawl": "crawl",
	"adjust": "adjust", "lookup": "look up",
}

// resourceNames maps resource tokens that shouldn't be pluralized or need a
// display name different from the node segment.
var resourceNames = map[string]string{
	"content": "content", "media": "media files", "analytics": "analytics",
	"news": "news", "revenue": "revenue", "settings": "settings",
	"blog": "blog posts", "wiki": "wiki pages", "dm": "direct messages",
	"api": "API", "ai": "AI", "mcp": "MCP", "seo": "SEO",
	"faq": "FAQs", "rag": "RAG", "hitl": "HITL review", "rss": "RSS",
	"cv": "CVs", "sop": "SOPs", "eca": "ECA", "gdpr": "GDPR",
	"oauth": "OAuth", "sso": "SSO", "asynq": "Asynq", "valkey": "Valkey",
	"seaweed": "SeaweedFS", "imgproxy": "imgproxy", "umami": "Umami",
	"meili": "Meilisearch", "geo": "geo", "ads": "ads", "ad": "ad",
	"totp": "TOTP", "2fa": "2FA", "totp_required": "TOTP requirement",
	"self_serve": "self-serve ads", "col": "cost-of-living data",
	"fx": "FX rates", "op": "operations", "ops": "operations",
	"gamification": "gamification", "mentorship": "mentorship",
	"mutual_aid": "mutual aid", "travel": "travel tools",
	"remittance": "remittance tools", "japa_stage": "Japa Stage",
	"pof_calculator": "the PoF calculator", "tax_calculator": "the tax calculator",
	"crs_calculator": "the CRS calculator", "col_calculator": "the cost-of-living calculator",
	"sop_generator": "the SOP generator", "scholarship_finder": "the scholarship finder",
	"language_test_hub": "the language test hub", "service_directory": "the service directory",
	"ama": "AMAs", "next_actions": "next actions", "jce": "JCE",
	"visa_routes": "visa routes", "scam_alert": "scam alerts",
	"service_coupons": "service coupons", "planning_group": "planning groups",
	"housing_guide": "housing guides", "webhooks": "webhooks",
	"agent": "the AI agent", "japabot": "Japabot", "japa": "Japa",
	"byok": "your own API keys", "rbac": "roles and permissions",
	"org": "organizations", "sitemap": "sitemaps", "openapi": "the OpenAPI spec",
	"graphql": "GraphQL", "webhook": "webhooks", "pwa": "the PWA",
	"cdn": "CDN", "ip": "IPs", "ipfs": "IPFS", "url": "URLs",
	"slug": "slugs", "uuid": "UUIDs", "id": "IDs", "qr": "QR codes",
	"pdf": "PDFs", "csv": "CSV files", "json": "JSON", "xml": "XML",
	"html": "HTML", "css": "CSS", "js": "JavaScript", "tts": "text-to-speech",
	"llm": "LLMs", "nft": "NFTs", "crypto": "crypto", "kyc": "KYC",
	"aml": "AML", "pii": "PII", "vat": "VAT", "ussd": "USSD",
	"sms": "SMS", "email": "emails", "mfa": "MFA",
	"self": "", // scope word — never a resource
	"own":  "", "any": "", "all": "",
}

// irregularPlurals covers the pluralization cases naive suffix rules get wrong.
var irregularPlurals = map[string]string{
	"person": "people", "child": "children", "persona": "personas",
	"analysis": "analyses", "status": "statuses", "quiz": "quizzes",
	"newsletter": "newsletters", "directory": "directories",
}

// irregularSingulars is the inverse used when an "any" scope needs a singular.
var irregularSingulars = map[string]string{
	"people": "people", "children": "children",
}

// nodePhrases are exact overrides for nodes whose generic derivation would be
// wrong or unreadable.
var nodePhrases = map[string]string{
	"ai.byok":                        "use your own API keys",
	"agent.use":                      "use the AI agent",
	"agent.admin":                    "administer the AI agent",
	"admin.hard_delete":              "permanently delete records",
	"admin.impersonate":              "impersonate users",
	"admin.moderation":               "access moderation tools",
	"admin.broadcast":                "send broadcasts",
	"admin.settings":                 "change admin settings",
	"admin.analytics":                "view analytics",
	"analytics.admin":                "administer analytics",
	"spaces.admin":                   "administer spaces",
	"directory.admin":                "administer directories",
	"content.admin":                  "administer content",
	"comments.admin":                 "administer comments",
	"control.panel.access":           "access the control panel",
	"content.spam_flagged":           "view spam-flagged content",
	"content.link_preview.blacklist": "manage the link-preview blacklist",
	"content.sanitization.allowed_iframe_hosts": "manage allowed iframe hosts",
	"custom_fields.values.set":                  "set custom field values",
	"dm.request":                                "send DM requests",
	"credential_checker.note":                   "add credential notes",
	"crs_calculator.history":                    "view CRS calculator history",
	"campaign.retry":                            "retry campaigns",
	"general.site_name":                         "change the site name",
	"mentorship.become_mentor":                  "become a mentor",
	"tips.buymeacoffee":                         "link a Buy Me a Coffee account",
	"tips.kofi":                                 "link a Ko-fi account",
	"tips.opencollective":                       "link an Open Collective account",
	"media.cdn.base_url":                        "manage the CDN base URL",
	"integrations.cdn.provider":                 "manage the CDN provider",
	"integrations.bunnycdn.api_key":             "manage the BunnyCDN API key",
	"integrations.bunnycdn.pull_zone_id":        "manage the BunnyCDN pull zone",
	"integrations.cloudflare.api_token":         "manage the Cloudflare API token",
	"integrations.cloudflare.zone_id":           "manage the Cloudflare zone",
	"security.totp_required_for_admin":          "change the admin 2FA requirement",
	"platform.db.health_view":                   "view database health",
	"platform.db.vacuum_trigger":                "trigger database vacuums",
	"wallet.admin.adjust":                       "adjust wallet balances",
	"wallet.admin.credits_refund":               "refund wallet credits",
	"wallet.points.balance":                     "view wallet point balances",
	"wallet.default_currency":                   "change the default wallet currency",
	"tickets.comment.internal":                  "post internal ticket comments",
	"spaces.wiki_rate_interval_ms":              "change the wiki rate limit",
	"intel.default_persona_id":                  "change the default intelligence persona",
	"intel.extraction_model":                    "change the extraction model",
	"intel.extraction_confidence_auto":          "change the auto-extraction threshold",
	"intel.extraction_confidence_review":        "change the review threshold",
	"intel.extraction_max_retries":              "change the extraction retry limit",
	"intel.probe.max_requests":                  "change the probe request limit",
	"intel.channels.crawl":                      "crawl intelligence channels",
	"intel.sources.crawl_now":                   "trigger source crawls",
	"intel.source_decision":                     "decide on intelligence sources",
	"link_preview.fetch":                        "fetch link previews",
	"push.receive":                              "receive push notifications",
	"verification.request":                      "request verification",
	"newsletter.subscribe":                      "subscribe to the newsletter",
	"jobs.post":                                 "post jobs",
	"mentorship.request":                        "request mentorship",
	"scholarship.save":                          "save scholarships",
	"remittance.fetch":                          "fetch remittance data",
	"wiki.structured_edits":                     "make structured wiki edits",
	"api.read.users.public":                     "read public user data via the API",
	"geo.lookup":                                "look up geo data",
	"mentorship.comparison":                     "compare mentorship options",
	"remittance.alert":                          "manage remittance alerts",
	"language_test_hub.story":                   "publish language test stories",
}

// scopeWords are trailing action qualifiers that qualify rather than name the
// action itself.
var scopeWords = map[string]string{
	"own": "your own ", "self": "your own ", "any": "any ", "all": "all ",
}

// noPluralize covers resource tail words that aren't count nouns — pluralizing
// them produces garbage ("enableds", "advanceds").
var noPluralize = map[string]bool{
	"me": true, "set": true, "values": true, "history": true,
	"request": true, "stats": true, "logs": true, "public": true,
	"panel": true, "access": true, "advanced": true, "all": true,
	"enabled": true, "disabled": true, "flagged": true,
}

// pluralize applies naive English pluralization for resource phrases.
func pluralize(word string) string {
	if p, ok := irregularPlurals[word]; ok {
		return p
	}

	if word == "" || strings.HasSuffix(word, "s") {
		return word
	}

	for _, suf := range []string{"ch", "sh", "x", "z"} {
		if strings.HasSuffix(word, suf) {
			return word + "es"
		}
	}

	if strings.HasSuffix(word, "y") && len(word) > 1 {
		c := word[len(word)-2]
		if !strings.ContainsRune("aeiou", rune(c)) {
			return word[:len(word)-1] + "ies"
		}
	}

	return word + "s"
}

// singularize strips a plural tail for "any X" phrases.
func singularize(word string) string {
	if s, ok := irregularSingulars[word]; ok {
		return s
	}

	if strings.HasSuffix(word, "ies") && len(word) > 3 {
		return word[:len(word)-3] + "y"
	}

	for _, suf := range []string{"ches", "shes", "xes", "zes", "ses"} {
		if strings.HasSuffix(word, suf) {
			return word[:len(word)-2]
		}
	}

	return strings.TrimSuffix(word, "s")
}

// resourceWords renders resource node segments ("academy.payout" →
// ["academy", "payout"]) as display words, honouring resourceNames
// overrides. The returned slice carries a flag per word telling the caller
// whether the word came from the overrides map (already display-ready) or
// is a raw segment (pluralizable).
func resourceWords(parts []string) (words []string, mapped []bool) {
	for _, p := range parts {
		key := strings.ReplaceAll(p, "_", " ")
		if m, ok := resourceNames[p]; ok {
			if m == "" {
				continue
			}

			words = append(words, m)
			mapped = append(mapped, true)
			continue
		}

		words = append(words, key)
		mapped = append(mapped, false)
	}

	return words, mapped
}

// resourcePhrase renders the middle node segments as a display phrase,
// pluralizing the tail word when it is a bare lowercase entity word.
func resourcePhrase(parts []string) string {
	words, mapped := resourceWords(parts)
	if len(words) == 0 {
		return ""
	}

	last := len(words) - 1
	if !mapped[last] && words[last] == strings.ToLower(words[last]) &&
		!strings.HasSuffix(words[last], "s") && !noPluralize[words[last]] &&
		!strings.HasSuffix(words[last], "ed") && !strings.HasSuffix(words[last], "ing") {
		words[last] = pluralize(words[last])
	}

	return strings.Join(words, " ")
}

// DescribePermission renders a permission node as a verb phrase naming the
// gated action, e.g. "add reactions", "manage API keys", "edit your own blog
// posts", "view the users directory". It falls back to "perform this action"
// for nodes it can't decompose. Scope suffixes are honoured: _own/_self →
// "your own", _any/_all → "any"/"all" with a singular resource.
func DescribePermission(permKey string) string {
	if phrase, ok := nodePhrases[permKey]; ok {
		return phrase
	}

	parts := strings.Split(strings.TrimSpace(permKey), ".")
	if len(parts) == 0 || parts[0] == "" || permKey == "*" {
		return "perform this action"
	}

	// directory.X.view → "view the X directory".
	if parts[0] == "directory" && len(parts) >= 2 {
		res := resourcePhrase(parts[1 : len(parts)-1])
		if res == "" {
			return "view this directory"
		}

		return fmt.Sprintf("view the %s directory", singularize(res))
	}

	// admin.X — the domain marks platform-admin tooling, not a resource.
	if parts[0] == "admin" && len(parts) > 1 {
		parts = parts[1:]
	}

	// api.read.X / api.write.X — public API scope nodes ("read content via
	// the API"). Checked before the generic verb/resource split since "read"
	// and "write" sit in the middle of the node.
	if parts[0] == "api" && len(parts) > 2 && (parts[1] == "read" || parts[1] == "write") {
		res := resourcePhrase(parts[2:])
		if res == "" {
			res = "data"
		}

		return parts[1] + " " + res + " via the API"
	}

	// feature.X — feature reachability flags gate access to a feature, not an
	// action on it.
	if parts[0] == "feature" && len(parts) > 1 {
		res := resourcePhrase(parts[1:])
		if res == "" {
			return "access this feature"
		}

		// Feature names read singular ("the reactions feature").
		words := strings.Fields(res)
		words[len(words)-1] = singularize(words[len(words)-1])

		return "access the " + strings.Join(words, " ") + " feature"
	}

	// x.y.admin — administer X (agent.admin, bots.admin, ...).
	if parts[len(parts)-1] == "admin" && len(parts) > 1 {
		if res := resourcePhrase(parts[:len(parts)-1]); res != "" {
			return "administer " + res
		}
	}

	// A trailing .enabled segment is a reachability flag, not an action —
	// drop it so the resource describes the gated feature.
	if parts[len(parts)-1] == "enabled" && len(parts) > 1 {
		parts = parts[:len(parts)-1]
	}

	// Trailing scope SEGMENTS (x.y.own / x.y.any / x.y.all / x.y.self).
	scope := ""
	for len(parts) > 1 {
		if s, ok := scopeWords[parts[len(parts)-1]]; ok {
			scope = s
			parts = parts[:len(parts)-1]
			continue
		}

		break
	}

	actionTok := parts[len(parts)-1]
	resourceParts := parts[:len(parts)-1]

	// Split the action token on underscores and peel off scope words and the
	// hard-delete modifier from either end.
	sub := strings.Split(actionTok, "_")
	for len(sub) > 0 {
		last := sub[len(sub)-1]
		if s, ok := scopeWords[last]; ok {
			if scope == "" {
				scope = s
			}

			sub = sub[:len(sub)-1]
			continue
		}

		break
	}

	hardDelete := false
	for len(sub) > 1 && sub[0] == "admin" {
		sub = sub[1:]
	}

	if len(sub) > 1 && sub[0] == "hard" {
		hardDelete = true
		sub = sub[1:]
	}

	if len(sub) > 1 && sub[0] == "self" {
		if scope == "" {
			scope = scopeWords["self"]
		}

		sub = sub[1:]
	}

	verbTok := strings.Join(sub, "_")
	verb, ok := permissionVerbs[verbTok]

	var remainder []string
	if !ok && len(sub) > 1 {
		// verb_noun action tokens (view_results, delete_entry): the verb leads
		// and the trailing words are the object.
		if v, ok2 := permissionVerbs[sub[0]]; ok2 {
			verb = v
			remainder = sub[1:]
			ok = true
		}
	}

	if !ok {
		// The last segment isn't an action — the whole node is a resource;
		// the gate is "access X".
		res := resourcePhrase(parts)
		if res == "" {
			return "perform this action"
		}

		if hardDelete {
			return fmt.Sprintf("permanently delete %s", res)
		}

		return fmt.Sprintf("access %s", res)
	}

	res := ""
	if len(remainder) > 0 {
		// The action carries its own object — the resource is a qualifier,
		// not the pluralized target ("view assessment results").
		base, _ := resourceWords(resourceParts)
		tail, _ := resourceWords(remainder)
		words := append(base, tail...)
		res = strings.Join(words, " ")
	} else {
		res = resourcePhrase(resourceParts)
	}
	if res == "" {
		if hardDelete {
			return "permanently delete records"
		}

		return verb
	}

	if scope == "any " && res != "" {
		words := strings.Fields(res)
		words[len(words)-1] = singularize(words[len(words)-1])
		res = strings.Join(words, " ")
	}

	if hardDelete {
		return fmt.Sprintf("permanently delete %s", res)
	}

	return verb + " " + scope + res
}

// DescribeAnyPermission renders the shared action behind an any-of permission
// set (typically `x.action_own`/`x.action_any` pairs) by stripping scope
// qualifiers before describing the first node.
func DescribeAnyPermission(permKeys ...string) string {
	if len(permKeys) == 0 {
		return "perform this action"
	}

	key := permKeys[len(permKeys)-1]
	parts := strings.Split(key, ".")
	last := parts[len(parts)-1]
	for _, suf := range []string{"_own", "_any", "_self", "_all"} {
		last = strings.TrimSuffix(last, suf)
	}

	parts[len(parts)-1] = last

	return DescribePermission(strings.Join(parts, "."))
}

// DenialMessage renders the full user-facing 403 message for a missing
// permission node.
func DenialMessage(permKey string) string {
	return fmt.Sprintf("Your account doesn't have permission to %s — contact an administrator if you need access.", DescribePermission(permKey))
}

// AnyDenialMessage is DenialMessage for RequireAnyPermission route sets.
func AnyDenialMessage(permKeys ...string) string {
	return fmt.Sprintf("Your account doesn't have permission to %s — contact an administrator if you need access.", DescribeAnyPermission(permKeys...))
}

// SpaceDenialMessage renders the 403 for a space-scoped permission denial.
// The resource is implied by the space context, so "spaces.moderate" reads
// "moderate this space".
func SpaceDenialMessage(permKey string) string {
	phrase := DescribePermission(permKey)

	// Drop a leading "spaces" resource — the space is the context, not the
	// object of the action.
	parts := strings.Split(permKey, ".")
	if len(parts) > 1 && parts[0] == "spaces" {
		verb := DescribePermission(strings.Join(parts[1:], "."))
		if !strings.HasPrefix(verb, "access") && !strings.HasPrefix(verb, "perform") {
			phrase = verb
		} else {
			phrase = strings.TrimPrefix(verb, "access ")
			if phrase == "perform this action" {
				phrase = "perform this action"
			}
		}
	}

	if phrase == "perform this action" {
		return "You don't have permission to perform this action in this space — contact a space moderator or administrator."
	}

	return fmt.Sprintf("You don't have permission to %s in this space — contact a space moderator or administrator.", phrase)
}
