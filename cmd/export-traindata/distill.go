// distill.go — synthetic Q&A generation via a teacher model. When real
// forum Q&A volume is thin, a frontier model generates diverse pairs from
// published guide/wiki/lesson content — the model learns the material
// without waiting for organic community activity.
//
// Reuses the ai_providers table for credentials: the teacher is an
// admin-managed provider row selected by name (-distill-provider), so no
// new credential plumbing is needed. The underlying call is a direct
// /chat/completions POST rather than ai.Provider.Send because Response.Raw
// doesn't expose assistant text in an exported shape — a cmd tool doesn't
// need the dispatcher's circuit-breaker envelope.
package main

import (
	"errors"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/ai"
)

type distillOptions struct {
	ProviderName string // ai_providers.name of the teacher model
	Model        string // optional model override; empty = provider row's model
	MaxDocs      int    // seed documents to process
	PairsPerDoc  int    // Q&A pairs requested per document
}

// seedDoc is one unit of distillation input — published content flattened
// to text with enough context for the teacher to phrase grounded questions.
type seedDoc struct {
	Kind    string // "guide", "article", "wiki", "lesson"
	Title   string
	Context string // e.g. destination space or track name
	Body    string
}

func distillSource(opts distillOptions) func(context.Context, *pgxpool.Pool, string, int) ([]example, error) {
	return func(ctx context.Context, pool *pgxpool.Pool, system string, _ int) ([]example, error) {
		apiKey, baseURL, model, err := loadTeacherConfig(ctx, pool, opts.ProviderName)
		if err != nil {
			return nil, err
		}

		if opts.Model != "" {
			model = opts.Model
		}

		docs, err := loadSeedDocuments(ctx, pool, opts.MaxDocs)
		if err != nil {
			return nil, err
		}

		if len(docs) == 0 {
			slog.Warn("distill: no seed documents found")
			return nil, nil
		}

		client := &http.Client{Timeout: 120 * time.Second}

		var exs []example

		for i, doc := range docs {
			pairs, err := distillDoc(ctx, client, baseURL, apiKey, model, doc, opts.PairsPerDoc)
			if err != nil {
				slog.Warn("distill: doc failed, skipping", "title", doc.Title, "error", err)
				continue
			}

			for _, p := range pairs {
				if ex, ok := newExample(system, p.Question, p.Answer); ok {
					exs = append(exs, ex)
				}
			}

			slog.Info("distill progress", "doc", i+1, "of", len(docs), "title", doc.Title, "pairs", len(pairs))
		}

		return exs, nil
	}
}

// loadTeacherConfig resolves credentials for the named ai_providers row.
// The key is decrypted server-side and never logged.
func loadTeacherConfig(ctx context.Context, pool *pgxpool.Pool, name string) (apiKey, baseURL, model string, err error) {
	store := ai.NewProviderStore(pool, pool)

	providers, err := store.ListProviders(ctx)
	if err != nil {
		return "", "", "", fmt.Errorf("listing providers: %w", err)
	}

	for _, p := range providers {
		if p.Name != name {
			continue
		}

		if !p.Enabled {
			return "", "", "", fmt.Errorf("provider %q is disabled", name)
		}

		key, err := store.GetDecryptedAPIKey(ctx, p.ID)
		if err != nil {
			return "", "", "", fmt.Errorf("decrypting provider key: %w", err)
		}

		base := p.BaseURL
		if base == "" {
			base = "https://api.openai.com/v1"
		}

		return key, strings.TrimRight(base, "/"), p.Model, nil
	}

	return "", "", "", fmt.Errorf("no ai_providers row named %q — register the teacher model in Control Panel first", name)
}

// loadSeedDocuments pulls the strongest published content as distillation
// seeds, most-viewed first so scarce API budget goes to content that
// matters.
func loadSeedDocuments(ctx context.Context, pool *pgxpool.Pool, maxDocs int) ([]seedDoc, error) {
	rows, err := pool.Query(ctx, `
		SELECT content_type, title, body FROM contents
		WHERE status = 'published' AND visibility = 'public'
		  AND content_type IN ('guide', 'article')
		  AND NOT auto_hidden AND length(body) > 500
		ORDER BY view_count DESC NULLS LAST, published_at DESC
		LIMIT $1
	`, maxDocs)
	if err != nil {
		return nil, fmt.Errorf("loading contents seeds: %w", err)
	}

	var docs []seedDoc

	for rows.Next() {
		var kind, title, body string
		if err := rows.Scan(&kind, &title, &body); err != nil {
			rows.Close()
			return nil, err
		}

		if t := stripTitlePrefix(bodyText(body), title); t != "" {
			docs = append(docs, seedDoc{Kind: kind, Title: title, Body: t})
		}
	}

	rows.Close()

	// Wiki pages — destination context makes the questions concrete.
	wrows, err := pool.Query(ctx, `
		SELECT p.title, r.content::text, s.name
		FROM wiki_pages p
		JOIN spaces s ON s.id = p.space_id
		JOIN LATERAL (
			SELECT content FROM wiki_revisions
			WHERE wiki_page_id = p.id AND status = 'approved'
			ORDER BY created_at DESC LIMIT 1
		) r ON true
		WHERE p.is_active AND NOT p.is_locked
		ORDER BY p.updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("loading wiki seeds: %w", err)
	}

	for wrows.Next() {
		var title, dest string

		var content []byte
		if err := wrows.Scan(&title, &content, &dest); err != nil {
			wrows.Close()
			return nil, err
		}

		if t := stripTitlePrefix(bodyText(string(content)), title); t != "" {
			docs = append(docs, seedDoc{Kind: "wiki", Title: title, Context: dest, Body: t})
		}
	}

	wrows.Close()

	// Lessons — curriculum text, strongest teaching signal per token.
	lrows, err := pool.Query(ctx, `
		SELECT l.title, l.content, t.title
		FROM academy_lessons l
		JOIN academy_modules m ON m.id = l.module_id
		JOIN academy_tracks t ON t.id = m.track_id
		WHERE l.is_published AND t.is_published AND l.lesson_type = 'article'
		  AND length(l.content) > 500
	`)
	if err != nil {
		return nil, fmt.Errorf("loading lesson seeds: %w", err)
	}

	for lrows.Next() {
		var title, track string

		var content *string
		if err := lrows.Scan(&title, &content, &track); err != nil {
			lrows.Close()
			return nil, err
		}

		if content != nil {
			if t := stripTitlePrefix(bodyText(*content), title); t != "" {
				docs = append(docs, seedDoc{Kind: "lesson", Title: title, Context: track, Body: t})
			}
		}
	}

	lrows.Close()

	return docs, nil
}

type qaPair struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// distillPrompt instructs the teacher to emit grounded, varied Q&A pairs
// as JSON. Grounding is explicit: answers may only use facts present in
// the document — the teacher rephrases, never invents.
func distillPrompt(doc seedDoc, n int) (sys, user string) {
	sys = `You generate fine-tuning data for Japa AI, the assistant on JapaConnect — a platform helping Africans relocate abroad. Output ONLY a JSON object: {"pairs":[{"question":"...","answer":"..."}]}`

	var ctxLine string
	if doc.Context != "" {
		ctxLine = fmt.Sprintf("\nContext: %s\n", doc.Context)
	}

	user = fmt.Sprintf(`From the %s below, write %d diverse question-answer pairs a user might ask Japa AI.

Rules:
- Vary question style: factual, how-to, scenario ("I'm moving to...", "my agent said..."), comparison
- Answers use ONLY facts in the document — never invent figures, fees, dates, or requirements
- Answers are warm, direct, practical — the Japa AI voice
- Keep answers 50–250 words
- If the document lacks facts for a plausible question, don't include that pair

Title: %s%s
Document:
%s`, doc.Kind, n, doc.Title, ctxLine, doc.Body)

	return sys, user
}

// distillDoc calls the teacher's /chat/completions endpoint and parses
// the JSON pairs array.
func distillDoc(ctx context.Context, client *http.Client, baseURL, apiKey, model string, doc seedDoc, n int) ([]qaPair, error) {
	sys, usr := distillPrompt(doc, n)

	reqBody, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": usr},
		},
		"temperature":     0.8, // some spread across pairs
		"max_tokens":      4096,
		"response_format": map[string]string{"type": "json_object"},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("teacher API error (status %d): %s", resp.StatusCode, string(respBody[:min(300, len(respBody))]))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parsing teacher response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, errors.New("teacher returned no choices")
	}

	var out struct {
		Pairs []qaPair `json:"pairs"`
	}

	if err := json.Unmarshal([]byte(chatResp.Choices[0].Message.Content), &out); err != nil {
		return nil, fmt.Errorf("teacher returned non-JSON pairs: %w", err)
	}

	return out.Pairs, nil
}
