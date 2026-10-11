// Command export-traindata extracts fine-tuning examples from JapaConnect
// content tables into chat-format JSONL for LoRA/QLoRA training of a
// "Japa model" (e.g. Llama 3.1 8B via Unsloth/Axolotl).
//
// Each output line is {"messages":[{system},{user},{assistant}]} — the
// chat format both frameworks accept directly. Sources:
//
//   - hand-written synthetic examples for the §103 behavior rules
//     (behavior.go — refusals, citations, tool routing, privacy)
//   - solved forum threads + AMA-approved replies (real user language)
//   - completed ai_agent_sessions — production prompts → frontier-model
//     answers (self-distillation from real traffic)
//   - structured guide tables rendered deterministically to Q&A
//     (housing, ECA, tax obligations, arrival checklists)
//   - confirmed scam reports aggregated per (type, destination)
//   - wiki pages + latest approved revision
//   - published academy lessons
//   - published long-form contents (guides, articles)
//
// Post-processing: near-duplicate answers across sources are removed via
// 5-word shingle Jaccard dedup (first-seen wins — sources run in priority
// order), then a deterministic hash split holds out eval examples.
//
// Only aggregate/published content is emitted — no user IDs, emails, or
// handles ever reach the dataset. Anonymous threads contribute language
// patterns only.
//
// Usage:
//
//	source .env && go run ./cmd/export-traindata/ -out train.jsonl
//	go run ./cmd/export-traindata/ -out train.jsonl -min-votes 3 -sources forum,wiki
//	go run ./cmd/export-traindata/ -out train.jsonl -eval-out eval.jsonl -eval-frac 0.05
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/rag"
)

const defaultSystemPrompt = "You are Japa AI, the intelligence layer of JapaConnect — a platform helping Africans relocate, settle, and thrive abroad. You answer with warm, direct, practical authority. You never fabricate visa fees, processing times, or exchange rates — when unsure, say so and point to the relevant JapaConnect tool."

// Bounds on emitted example sizes — tiny rows carry no signal and huge
// bodies blow past typical 4-8k training context windows.
const (
	minTextLen = 40
	maxTextLen = 6000
)

type message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

// toolCall is the OpenAI/Llama tool-call shape carried on assistant
// messages in trajectory examples. The HF chat template renders it as a
// native tool invocation during training.
type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name          string          `json:"name"`
		ArgumentsJSON json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type example struct {
	Messages []message `json:"messages"`
}

// labeledExample carries its source through dedup for per-source drop
// stats. The label is never serialized into the JSONL output.
type labeledExample struct {
	ex  example
	src string
}

func newExample(system, user, assistant string) (example, bool) {
	u := strings.TrimSpace(user)
	a := strings.TrimSpace(assistant)
	if len(u) < minTextLen || len(a) < minTextLen {
		return example{}, false
	}
	if len(a) > maxTextLen {
		a = strings.TrimSpace(a[:maxTextLen])
	}
	return example{Messages: []message{
		{Role: "system", Content: system},
		{Role: "user", Content: u},
		{Role: "assistant", Content: a},
	}}, true
}

// finalAnswer returns the last assistant message's text — the training
// signal for both single-turn pairs and tool trajectories.
func finalAnswer(ex example) string {
	for i := len(ex.Messages) - 1; i >= 0; i-- {
		if ex.Messages[i].Role == "assistant" && ex.Messages[i].Content != "" {
			return ex.Messages[i].Content
		}
	}
	return ""
}

// bodyText flattens a TipTap/HTML/markdown body to plain text. Forum and
// comment bodies are stored as jsonb — sometimes a {"type":"doc"} object,
// sometimes a bare block array, which we wrap into a doc so
// rag.ExtractPlainText can walk it.
func bodyText(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		raw = `{"type":"doc","content":` + raw + `}`
	}
	return rag.ExtractPlainText(raw)
}

// renderJSON renders arbitrary JSONB guide fields (pathway_steps,
// rent_ranges, action_checklist, ...) as readable markdown-ish text.
// Shapes vary per row, so this walks generic values instead of decoding
// into fixed structs. Map keys are sorted so output is deterministic.
func renderJSON(v any, depth int) string {
	var b strings.Builder
	renderValue(&b, v, depth)
	return b.String()
}

func renderValue(b *strings.Builder, v any, depth int) {
	indent := strings.Repeat("  ", depth)

	switch t := v.(type) {
	case nil:
	case []any:
		for _, item := range t {
			if obj, ok := item.(map[string]any); ok {
				fmt.Fprintf(b, "%s- %s", indent, inlineObject(obj))
				continue
			}
			if arr, ok := item.([]any); ok {
				renderValue(b, arr, depth)
				continue
			}
			fmt.Fprintf(b, "%s- %v\n", indent, item)
		}
	case map[string]any:
		for _, k := range sortedKeys(t) {
			val := t[k]
			switch val.(type) {
			case map[string]any, []any:
				fmt.Fprintf(b, "%s%s:\n", indent, k)
				renderValue(b, val, depth+1)
			default:
				fmt.Fprintf(b, "%s%s: %v\n", indent, k, val)
			}
		}
	default:
		fmt.Fprintf(b, "%s%v\n", indent, t)
	}
}

// inlineObject renders an array-item object on one line:
// "step: 1, title: Create WES account". Nested values render recursively
// on following lines.
func inlineObject(obj map[string]any) string {
	var b strings.Builder

	first := true
	for _, k := range sortedKeys(obj) {
		val := obj[k]
		switch val.(type) {
		case map[string]any, []any:
			continue // nested values handled below
		default:
			if !first {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s: %v", k, val)
			first = false
		}
	}
	b.WriteString("\n")

	for _, k := range sortedKeys(obj) {
		val := obj[k]
		switch val.(type) {
		case map[string]any, []any:
			fmt.Fprintf(&b, "  %s:\n", k)
			renderValue(&b, val, 2)
		}
	}

	return b.String()
}

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

// stripTitlePrefix drops a leading repeat of the item's title — bodies
// commonly open with an H1 duplicating the title line. Strips the longest
// common word-prefix when ≥3 leading words match, which tolerates minor
// drift like a dropped "(2026)" suffix.
func stripTitlePrefix(text, title string) string {
	titleWords := strings.Fields(title)
	textWords := strings.Fields(text)
	if len(titleWords) == 0 {
		return text
	}

	norm := func(w string) string {
		return strings.TrimRight(strings.ToLower(w), ".,:;!?—–-·\"'()")
	}

	n := 0
	for n < len(titleWords) && n < len(textWords) && norm(titleWords[n]) == norm(textWords[n]) {
		n++
	}
	if n < 3 {
		return text
	}

	return strings.TrimSpace(strings.Join(textWords[n:], " "))
}

func jsonField(raw []byte) any {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return v
}

type source struct {
	name string
	fn   func(ctx context.Context, pool *pgxpool.Pool, system string, minVotes int) ([]example, error)
}

func main() {
	outPath := flag.String("out", "train.jsonl", "output JSONL path")
	evalPath := flag.String("eval-out", "eval.jsonl", "eval holdout JSONL path")
	evalFrac := flag.Float64("eval-frac", 0.05, "fraction of examples held out for eval (0 = disabled)")
	minVotes := flag.Int("min-votes", 2, "minimum reply vote_score for forum Q&A")
	// 0.8 drops boilerplate clones (same guide seeded per country) while
	// keeping template rows whose field values genuinely differ.
	dedupThreshold := flag.Float64("dedup", 0.8, "Jaccard similarity above which near-duplicate answers are dropped (0 = disabled)")
	system := flag.String("system", defaultSystemPrompt, "system prompt baked into each example")
	sourcesFlag := flag.String("sources", "", "comma-separated source filter (behavior,forum,traces,sessions,housing,eca,tax,checklist,scam,wiki,academy,learn,content,distill); empty = all DB sources")
	distillProvider := flag.String("distill-provider", "", "ai_providers.name of a teacher model — enables the distill source (off by default; costs API calls)")
	distillModel := flag.String("distill-model", "", "override the provider row's model")
	distillMaxDocs := flag.Int("distill-max-docs", 50, "seed documents for distillation")
	distillPairs := flag.Int("distill-pairs", 4, "Q&A pairs requested per document")
	includeStaff := flag.Bool("sessions-include-staff", false, "include sessions from staff-role users and AI personas (off by default — dev/test traffic pollutes)")
	toolRole := flag.String("trace-tool-role", "ipython", "role label for tool-result messages in traces — 'ipython' for Llama 3.x templates, 'tool' for Qwen/OpenAI-style templates")
	flag.Parse()

	opts := distillOptions{
		ProviderName: *distillProvider,
		Model:        *distillModel,
		MaxDocs:      *distillMaxDocs,
		PairsPerDoc:  *distillPairs,
	}

	if err := run(*outPath, *evalPath, *evalFrac, *minVotes, *dedupThreshold, *system, *sourcesFlag, opts, *includeStaff, *toolRole); err != nil {
		slog.Error("export-traindata failed", "error", err)
		os.Exit(1)
	}
}

func run(outPath, evalPath string, evalFrac float64, minVotes int, dedupThreshold float64, system, sourcesFlag string, dopts distillOptions, includeStaff bool, toolRole string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx := context.Background()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer database.Close()

	// Order matters for dedup: first-seen wins, so hand-curated and
	// human-verified sources run before bulk/template-rendered ones.
	// traces runs BEFORE sessions so multi-turn tool trajectories win over
	// their own flattened prompt→answer duplicates.
	sources := []source{
		{"behavior", func(_ context.Context, _ *pgxpool.Pool, sys string, _ int) ([]example, error) {
			return behaviorExamples(sys), nil
		}},
		{"forum", forumQA},
		{"traces", traceSource(includeStaff, toolRole)},
		{"sessions", sessionSource(includeStaff)},
		{"scam", scamReports},
		{"checklist", arrivalChecklists},
		{"housing", housingGuides},
		{"eca", ecaGuides},
		{"tax", taxGuides},
		{"wiki", wikiPages},
		{"academy", lessons},
		{"learn", lessons}, // legacy alias for "academy"
		{"content", publishedContents},
	}

	// Distillation is opt-in — it costs teacher-model API calls. It runs
	// after DB sources so its synthetic pairs dedup against real data
	// rather than the other way around.
	if dopts.ProviderName != "" {
		sources = append(sources, source{"distill", distillSource(dopts)})
	}

	var enabled map[string]bool
	if sourcesFlag != "" {
		enabled = map[string]bool{}
		for _, s := range strings.Split(sourcesFlag, ",") {
			enabled[strings.TrimSpace(s)] = true
		}
	}

	var all []labeledExample
	for _, src := range sources {
		if enabled != nil && !enabled[src.name] {
			continue
		}

		exs, err := src.fn(ctx, database.Reader, system, minVotes)
		if err != nil {
			return fmt.Errorf("source %s: %w", src.name, err)
		}

		for _, ex := range exs {
			all = append(all, labeledExample{ex: ex, src: src.name})
		}
		slog.Info("source extracted", "source", src.name, "examples", len(exs))
	}

	if dedupThreshold > 0 {
		var dropped map[string]int
		all, dropped = dedup(all, dedupThreshold)
		for src, n := range dropped {
			if n > 0 {
				slog.Info("dedup dropped near-duplicates", "source", src, "dropped", n)
			}
		}
	}

	// Deterministic eval holdout — hash the user turn so re-runs split
	// identically regardless of row ordering or added sources.
	var train, eval []example
	for _, l := range all {
		if evalFrac > 0 && float64(fnvString(l.ex.Messages[1].Content)%1000)/1000 < evalFrac {
			eval = append(eval, l.ex)
		} else {
			train = append(train, l.ex)
		}
	}

	if err := writeJSONL(outPath, train); err != nil {
		return err
	}
	slog.Info("train written", "file", outPath, "examples", len(train))

	if evalFrac > 0 {
		if err := writeJSONL(evalPath, eval); err != nil {
			return err
		}
		slog.Info("eval written", "file", evalPath, "examples", len(eval))
	}

	return nil
}

func writeJSONL(path string, exs []example) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, ex := range exs {
		if err := enc.Encode(ex); err != nil {
			return fmt.Errorf("writing example: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Dedup — 5-word shingle Jaccard similarity over normalized answer text.
// An inverted index (shingle → kept examples) avoids O(n²) comparisons.
// ---------------------------------------------------------------------------

func dedup(in []labeledExample, threshold float64) ([]labeledExample, map[string]int) {
	var kept []labeledExample
	var keptShingles []map[uint64]struct{}
	index := map[uint64][]int{} // shingle hash → kept example indexes
	dropped := map[string]int{}

	for _, l := range in {
		sh := shingleSet(normalizeWords(finalAnswer(l.ex)))

		// Only compare against examples sharing at least one shingle.
		candidates := map[int]struct{}{}
		for h := range sh {
			for _, i := range index[h] {
				candidates[i] = struct{}{}
			}
		}

		dup := false
		for i := range candidates {
			if jaccard(sh, keptShingles[i]) >= threshold {
				dup = true
				break
			}
		}
		if dup {
			dropped[l.src]++
			continue
		}

		idx := len(kept)
		kept = append(kept, l)
		keptShingles = append(keptShingles, sh)
		for h := range sh {
			index[h] = append(index[h], idx)
		}
	}

	return kept, dropped
}

// normalizeWords lowercases and splits on non-alphanumeric boundaries so
// punctuation and markdown noise don't fragment shingles.
func normalizeWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// shingleSet hashes overlapping 5-word windows. Documents shorter than
// the window collapse to a single shingle of the whole text.
func shingleSet(words []string) map[uint64]struct{} {
	const n = 5
	out := map[uint64]struct{}{}
	if len(words) == 0 {
		return out
	}
	if len(words) <= n {
		out[fnvString(strings.Join(words, " "))] = struct{}{}
		return out
	}
	for i := 0; i+n <= len(words); i++ {
		out[fnvString(strings.Join(words[i:i+n], " "))] = struct{}{}
	}
	return out
}

func jaccard(a, b map[uint64]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	smaller, larger := a, b
	if len(smaller) > len(larger) {
		smaller, larger = larger, smaller
	}
	inter := 0
	for k := range smaller {
		if _, ok := larger[k]; ok {
			inter++
		}
	}
	return float64(inter) / float64(len(a)+len(b)-inter)
}

func fnvString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// ---------------------------------------------------------------------------
// Source: solved forum threads — title+body as the question, best reply
// (AMA-approved first, then top-voted) as the answer.
// ---------------------------------------------------------------------------

func forumQA(ctx context.Context, pool *pgxpool.Pool, system string, minVotes int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.title, t.body::text, r.body::text
		FROM forum_threads t
		JOIN LATERAL (
			SELECT body FROM forum_replies
			WHERE thread_id = t.id AND deleted_at IS NULL
			ORDER BY (ama_approved_at IS NOT NULL) DESC, vote_score DESC
			LIMIT 1
		) r ON true
		WHERE t.solved = true
		  AND t.deleted_at IS NULL
		  AND t.vote_score >= $1
		ORDER BY t.vote_score DESC
	`, minVotes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exs []example
	for rows.Next() {
		var title, qBody, aBody string
		if err := rows.Scan(&title, &qBody, &aBody); err != nil {
			return nil, err
		}

		question := strings.TrimSpace(title + "\n\n" + bodyText(qBody))
		if ex, ok := newExample(system, question, bodyText(aBody)); ok {
			exs = append(exs, ex)
		}
	}
	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Source: housing guides — structured rent/scam fields rendered to Q&A.
// ---------------------------------------------------------------------------

func housingGuides(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT ds.name, cs.name, g.accommodation_type,
		       g.rent_ranges::text, g.landlord_requirements,
		       g.search_platforms::text, g.scam_categories, g.tips
		FROM housing_guides g
		JOIN spaces ds ON ds.id = g.destination_space_id
		LEFT JOIN spaces cs ON cs.id = g.city_space_id
		WHERE g.status = 'published' AND g.is_active
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exs []example
	for rows.Next() {
		var dest string
		var city, accType, landlordReqs, tips *string
		var rentRanges, platforms []byte
		var scamCats []string
		if err := rows.Scan(&dest, &city, &accType, &rentRanges, &landlordReqs, &platforms, &scamCats, &tips); err != nil {
			return nil, err
		}

		where := dest
		if city != nil && *city != "" {
			where = *city + ", " + dest
		}

		var ans strings.Builder
		if accType != nil && *accType != "" {
			fmt.Fprintf(&ans, "Accommodation type: %s\n\n", *accType)
		}
		if rent := renderJSON(jsonField(rentRanges), 0); rent != "" {
			fmt.Fprintf(&ans, "Typical rent ranges:\n%s\n", rent)
		}
		if landlordReqs != nil && *landlordReqs != "" {
			fmt.Fprintf(&ans, "What landlords usually require: %s\n\n", *landlordReqs)
		}
		if p := renderJSON(jsonField(platforms), 0); p != "" {
			fmt.Fprintf(&ans, "Where to search:\n%s\n", p)
		}
		if len(scamCats) > 0 {
			fmt.Fprintf(&ans, "Scams to watch for: %s\n\n", strings.Join(scamCats, ", "))
		}
		if tips != nil && *tips != "" {
			fmt.Fprintf(&ans, "Tips: %s\n", *tips)
		}

		questions := []string{
			fmt.Sprintf("How do I find housing in %s?", where),
			fmt.Sprintf("What should I know about renting in %s?", where),
		}
		for _, q := range questions {
			if ex, ok := newExample(system, q, ans.String()); ok {
				exs = append(exs, ex)
			}
		}
	}
	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Source: ECA guides — credential assessment pathways per destination.
// ---------------------------------------------------------------------------

func ecaGuides(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT ds.name, p.name, g.purpose, g.pathway_steps::text,
		       g.fee_estimate, g.currency, g.timeline_estimate,
		       g.documents_required::text, g.official_url
		FROM eca_guides g
		JOIN spaces ds ON ds.id = g.destination_space_id
		JOIN eca_providers p ON p.id = g.provider_id
		WHERE g.status = 'published' AND g.is_active
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exs []example
	for rows.Next() {
		var dest, provider, purpose string
		var fee, currency, timeline, officialURL *string
		var steps, docs []byte
		if err := rows.Scan(&dest, &provider, &purpose, &steps, &fee, &currency, &timeline, &docs, &officialURL); err != nil {
			return nil, err
		}

		var ans strings.Builder
		fmt.Fprintf(&ans, "For %s in %s, you can use %s.\n\n", purpose, dest, provider)
		if s := renderJSON(jsonField(steps), 0); s != "" {
			fmt.Fprintf(&ans, "Steps:\n%s\n", s)
		}
		if fee != nil && *fee != "" {
			feeStr := *fee
			// fee_estimate often already embeds the currency ("CAD 258") —
			// only append the ISO code when the value is bare numerals.
			if currency != nil && *currency != "" && !strings.ContainsFunc(feeStr, unicode.IsLetter) {
				feeStr = *currency + " " + feeStr
			}
			fmt.Fprintf(&ans, "Estimated fee: %s (verify the current fee — these change).\n", feeStr)
		}
		if timeline != nil && *timeline != "" {
			fmt.Fprintf(&ans, "Typical timeline: %s\n", *timeline)
		}
		if d := renderJSON(jsonField(docs), 0); d != "" {
			fmt.Fprintf(&ans, "Documents required:\n%s\n", d)
		}
		if officialURL != nil && *officialURL != "" {
			fmt.Fprintf(&ans, "Official source: %s\n", *officialURL)
		}

		q := fmt.Sprintf("How do I get my credentials assessed for %s?", dest)
		if ex, ok := newExample(system, q, ans.String()); ok {
			exs = append(exs, ex)
		}
	}
	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Source: tax obligation guides — origin→destination tax exposure.
// ---------------------------------------------------------------------------

func taxGuides(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT os.name, ds.name, g.employment_status, g.residency_trigger,
		       g.worldwide_income_taxed, g.first_year_obligations, g.action_checklist::text
		FROM tax_obligation_guides g
		JOIN spaces os ON os.id = g.origin_space_id
		JOIN spaces ds ON ds.id = g.destination_space_id
		WHERE g.status = 'published' AND g.is_active
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exs []example
	for rows.Next() {
		var origin, dest, empStatus string
		var trigger, firstYear *string
		var worldwide *bool
		var checklist []byte
		if err := rows.Scan(&origin, &dest, &empStatus, &trigger, &worldwide, &firstYear, &checklist); err != nil {
			return nil, err
		}

		var ans strings.Builder
		fmt.Fprintf(&ans, "Moving from %s to %s as %s:\n\n", origin, dest, empStatus)
		if trigger != nil && *trigger != "" {
			fmt.Fprintf(&ans, "Tax residency trigger: %s\n", *trigger)
		}
		if worldwide != nil {
			if *worldwide {
				ans.WriteString("Your worldwide income may be taxable in your destination country.\n")
			} else {
				ans.WriteString("Generally only income sourced in your destination country is taxed there.\n")
			}
		}
		if firstYear != nil && *firstYear != "" {
			fmt.Fprintf(&ans, "\nFirst-year obligations: %s\n", *firstYear)
		}
		if c := renderJSON(jsonField(checklist), 0); c != "" {
			fmt.Fprintf(&ans, "\nAction checklist:\n%s", c)
		}
		ans.WriteString("\nTax rules change — confirm with the official tax authority or a licensed adviser.")

		q := fmt.Sprintf("Do I still pay tax in %s after moving to %s as %s?", origin, dest, empStatus)
		if ex, ok := newExample(system, q, ans.String()); ok {
			exs = append(exs, ex)
		}
	}
	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Source: arrival checklists — items grouped per guide, rendered by phase.
// ---------------------------------------------------------------------------

func arrivalChecklists(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT g.id, ds.name, g.visa_category, i.phase, i.title, i.description, i.official_link
		FROM arrival_checklist_guides g
		JOIN spaces ds ON ds.id = g.destination_space_id
		JOIN arrival_checklist_items i ON i.guide_id = g.id
		WHERE g.status = 'published' AND g.is_active AND NOT i.is_flagged
		ORDER BY g.id, i.sort_order
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type item struct {
		phase, title, desc, link string
	}
	var exs []example
	var curID, curDest string
	var curVisa *string
	var items []item

	flush := func() {
		if len(items) == 0 {
			return
		}
		var ans strings.Builder
		lastPhase := ""
		for _, it := range items {
			if it.phase != lastPhase {
				fmt.Fprintf(&ans, "\n%s:\n", it.phase)
				lastPhase = it.phase
			}
			fmt.Fprintf(&ans, "- %s", it.title)
			if it.desc != "" {
				fmt.Fprintf(&ans, " — %s", it.desc)
			}
			if it.link != "" {
				fmt.Fprintf(&ans, " (%s)", it.link)
			}
			ans.WriteString("\n")
		}

		visa := ""
		if curVisa != nil && *curVisa != "" {
			visa = " on a " + *curVisa + " visa"
		}
		q := fmt.Sprintf("I just arrived in %s%s. What do I need to do first?", curDest, visa)
		if ex, ok := newExample(system, q, ans.String()); ok {
			exs = append(exs, ex)
		}
		items = items[:0]
	}

	for rows.Next() {
		var id, dest, phase, title string
		var visa, desc, link *string
		if err := rows.Scan(&id, &dest, &visa, &phase, &title, &desc, &link); err != nil {
			return nil, err
		}
		if id != curID {
			flush()
			curID, curDest, curVisa = id, dest, visa
		}
		it := item{phase: phase, title: title}
		if desc != nil {
			it.desc = *desc
		}
		if link != nil {
			it.link = *link
		}
		items = append(items, it)
	}
	flush()

	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Source: confirmed scam reports aggregated per (type, destination) — the
// model learns scam patterns, not individual accusations.
// ---------------------------------------------------------------------------

func scamReports(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT r.scam_type, s.name, count(*) AS n,
		       array_agg(LEFT(r.description, 300) ORDER BY r.created_at DESC)
		FROM scam_reports r
		JOIN spaces s ON s.id = r.destination_space_id
		WHERE r.status = 'confirmed' AND r.deleted_at IS NULL
		GROUP BY r.scam_type, s.name
		HAVING count(*) >= 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exs []example
	for rows.Next() {
		var scamType, dest string
		var n int
		var descs []string
		if err := rows.Scan(&scamType, &dest, &n, &descs); err != nil {
			return nil, err
		}

		var ans strings.Builder
		fmt.Fprintf(&ans, "There %s %d confirmed report(s) of %s scams targeting people relocating to %s.\n\nCommon patterns:\n",
			map[bool]string{true: "is", false: "are"}[n == 1], n, scamType, dest)
		for i, d := range descs {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&ans, "- %s\n", strings.TrimSpace(d))
		}
		ans.WriteString("\nIf you've encountered this scam, report it on the JapaConnect scam alert board so others are warned.")

		q := fmt.Sprintf("What do %s scams look like in %s?", scamType, dest)
		if ex, ok := newExample(system, q, ans.String()); ok {
			exs = append(exs, ex)
		}
	}
	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Source: wiki pages + latest approved revision.
// ---------------------------------------------------------------------------

func wikiPages(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.title, r.content::text, s.name
		FROM wiki_pages p
		JOIN spaces s ON s.id = p.space_id
		JOIN LATERAL (
			SELECT content FROM wiki_revisions
			WHERE wiki_page_id = p.id AND status = 'approved'
			ORDER BY created_at DESC LIMIT 1
		) r ON true
		WHERE p.is_active AND NOT p.is_locked
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exs []example
	for rows.Next() {
		var title, dest string
		var content []byte
		if err := rows.Scan(&title, &content, &dest); err != nil {
			return nil, err
		}

		q := fmt.Sprintf("What should I know about %s in %s?", title, dest)
		if ex, ok := newExample(system, q, stripTitlePrefix(bodyText(string(content)), title)); ok {
			exs = append(exs, ex)
		}
	}
	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Source: published academy lessons (article-type lessons only — quizzes
// and videos don't flatten to text).
// ---------------------------------------------------------------------------

func lessons(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT l.title, l.content, t.title
		FROM academy_lessons l
		JOIN academy_modules m ON m.id = l.module_id
		JOIN academy_tracks t ON t.id = m.track_id
		WHERE l.is_published AND t.is_published AND l.lesson_type = 'article'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exs []example
	for rows.Next() {
		var title, trackTitle string
		var content *string
		if err := rows.Scan(&title, &content, &trackTitle); err != nil {
			return nil, err
		}
		if content == nil {
			continue
		}

		q := fmt.Sprintf("Explain %s.", title)
		if ex, ok := newExample(system, q, stripTitlePrefix(bodyText(*content), title)); ok {
			exs = append(exs, ex)
		}
	}
	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Source: published guides/articles — long-form voice examples.
// ---------------------------------------------------------------------------

func publishedContents(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
	rows, err := pool.Query(ctx, `
		SELECT title, body FROM contents
		WHERE status = 'published' AND visibility = 'public'
		  AND content_type IN ('guide', 'article')
		  AND NOT auto_hidden
		  AND length(body) > 500
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exs []example
	for rows.Next() {
		var title, body string
		if err := rows.Scan(&title, &body); err != nil {
			return nil, err
		}

		q := fmt.Sprintf("Tell me about %s.", title)
		if ex, ok := newExample(system, q, stripTitlePrefix(bodyText(body), title)); ok {
			exs = append(exs, ex)
		}
	}
	return exs, rows.Err()
}

// ---------------------------------------------------------------------------
// Sources: ai_agent_sessions — self-distillation from production traffic.
//
// "sessions" emits prompt → final-answer pairs. "traces" emits the full
// tool trajectory (assistant tool_call → ipython/tool result → answer) for
// sessions whose calls were all read-only — mutating tools (content.create*
// etc.) carry session-scoped IDs that would teach the model to memorize a
// specific persona UUID instead of echoing the one in context.
//
// Both exclude staff accounts (super_admin/admin/moderator role slugs) and
// AI personas by default — dev/test traffic is the main corpus pollutant.
// -sessions-include-staff lifts the filter for local verification.
//
// Sessions are ordered grounded-first so shingle dedup keeps the answer
// that actually ran a successful tool call when questions repeat.
// ---------------------------------------------------------------------------

// sessionStaffFilter excludes sessions owned by staff-role accounts or AI
// personas. Appended to session queries unless -sessions-include-staff.
const sessionStaffFilter = `
	  AND NOT EXISTS (
	    SELECT 1
	    FROM user_group_memberships m
	    JOIN group_roles gr ON gr.group_id = m.group_id
	    JOIN user_roles r ON r.id = gr.role_id
	    WHERE m.user_id = s.user_id
	      AND r.slug = ANY ('{super_admin,super_admin_role,admin-group-role,admin,staff,moderator}'::text[])
	  )
	  AND NOT EXISTS (SELECT 1 FROM ai_personas ap WHERE ap.user_id = s.user_id)`

func sessionSource(includeStaff bool) func(context.Context, *pgxpool.Pool, string, int) ([]example, error) {
	return func(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
		filter := sessionStaffFilter
		if includeStaff {
			filter = ""
		}

		rows, err := pool.Query(ctx, `
			SELECT s.prompt, s.result_summary
			FROM ai_agent_sessions s
			WHERE s.status = 'completed'
			  AND s.error_message IS NULL
			  AND s.result_summary IS NOT NULL
			  AND length(s.prompt) >= 20
			  AND length(s.result_summary) >= 40
			`+filter+`
			ORDER BY EXISTS (
			    SELECT 1 FROM ai_agent_tool_calls t
			    WHERE t.session_id = s.id AND t.status = 'success'
			) DESC, s.created_at DESC
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var exs []example
		for rows.Next() {
			var prompt, summary string
			if err := rows.Scan(&prompt, &summary); err != nil {
				return nil, err
			}
			if ex, ok := newExample(system, prompt, summary); ok {
				exs = append(exs, ex)
			}
		}
		return exs, rows.Err()
	}
}

// isReadOnlyTool reports whether a tool call is safe to include in a
// trajectory: read tools only. Write tools embed session-scoped IDs
// (persona author_id, content_id) that would teach ID memorization.
func isReadOnlyTool(name string) bool {
	for _, suf := range []string{".search", ".list", ".get", ".read", ".lookup", ".find", ".check"} {
		if strings.HasSuffix(name, suf) {
			return true
		}
	}
	return strings.HasPrefix(name, "external.") ||
		strings.HasPrefix(name, "knowledge.") ||
		strings.HasPrefix(name, "presearch")
}

// truncateRunes bounds tool-result payloads — external.search results can
// run tens of KB; the trajectory needs the shape, not the full payload.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func traceSource(includeStaff bool, toolRole string) func(context.Context, *pgxpool.Pool, string, int) ([]example, error) {
	return func(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
		filter := sessionStaffFilter
		if includeStaff {
			filter = ""
		}

		rows, err := pool.Query(ctx, `
			SELECT s.id, s.prompt, s.result_summary,
			       t.tool_name, t.arguments, t.result, t.status, t.error_message
			FROM ai_agent_sessions s
			JOIN ai_agent_tool_calls t ON t.session_id = s.id
			WHERE s.status = 'completed'
			  AND s.error_message IS NULL
			  AND s.result_summary IS NOT NULL
			  AND length(s.prompt) >= 20
			  AND length(s.result_summary) >= 40
			`+filter+`
			ORDER BY s.id, t.created_at ASC
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		type call struct {
			name   string
			args   []byte
			result []byte
			status string
			errMsg string
		}
		type sess struct {
			prompt, summary string
			calls           []call
		}

		var sessions []sess
		var curID string

		for rows.Next() {
			var id, prompt, summary, name, status string
			var args, result []byte
			var errMsg *string
			if err := rows.Scan(&id, &prompt, &summary, &name, &args, &result, &status, &errMsg); err != nil {
				return nil, err
			}
			if id != curID {
				sessions = append(sessions, sess{prompt: prompt, summary: summary})
				curID = id
			}
			c := call{name: name, args: args, result: result, status: status}
			if errMsg != nil {
				c.errMsg = *errMsg
			}
			cur := &sessions[len(sessions)-1]
			cur.calls = append(cur.calls, c)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}

		var exs []example
		for _, s := range sessions {
			// Trajectories only when every call is read-only — mixed or
			// mutating sessions fall back to the plain sessions source.
			readOnly := true
			for _, c := range s.calls {
				if !isReadOnlyTool(c.name) {
					readOnly = false
					break
				}
			}
			if !readOnly {
				continue
			}

			msgs := []message{
				{Role: "system", Content: system},
				{Role: "user", Content: strings.TrimSpace(s.prompt)},
			}

			for i, c := range s.calls {
				tc := toolCall{ID: fmt.Sprintf("call_%d", i+1), Type: "function"}
				tc.Function.Name = c.name
				tc.Function.ArgumentsJSON = json.RawMessage(c.args)
				if len(tc.Function.ArgumentsJSON) == 0 {
					tc.Function.ArgumentsJSON = json.RawMessage(`{}`)
				}
				msgs = append(msgs, message{Role: "assistant", ToolCalls: []toolCall{tc}})

				// Uniform result envelope — status is always visible so
				// error/privacy_blocked outcomes teach stop-and-report.
				resultText := "{}"
				if len(c.result) > 0 {
					resultText = string(c.result)
				}
				envelope, _ := json.Marshal(map[string]any{
					"status": c.status,
					"result": json.RawMessage(resultText),
					"error":  c.errMsg,
				})
				msgs = append(msgs, message{Role: toolRole, Content: truncateRunes(string(envelope), 2000)})
			}

			summary := strings.TrimSpace(s.summary)
			if len(summary) > maxTextLen {
				summary = strings.TrimSpace(summary[:maxTextLen])
			}
			msgs = append(msgs, message{Role: "assistant", Content: summary})

			if len(s.prompt) >= minTextLen && len(summary) >= minTextLen {
				exs = append(exs, example{Messages: msgs})
			}
		}
		return exs, nil
	}
}
