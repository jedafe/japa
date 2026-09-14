// Package scoring implements the JapaScore engine for the Japa Comparison Engine.
// // Ref: docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md §6.5
package scoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/jce"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Engine scores JCE comparisons using profiles, score factors, and item refs.
type Engine struct {
	read     *gen.Queries
	write    *gen.Queries
	database *db.DB
	compSvc  *comparison.Service
	r        *resolver.Registry
	settings *config.Settings
	vk       valkeycompat.Cmdable
}

// NewEngine creates a scoring engine backed by the given pools and services.
func NewEngine(database *db.DB, compSvc *comparison.Service, r *resolver.Registry, settings *config.Settings, vk valkeycompat.Cmdable) *Engine {
	return &Engine{
		read:     gen.New(database.Reader),
		write:    gen.New(database.Writer),
		database: database,
		compSvc:  compSvc,
		r:        r,
		settings: settings,
		vk:       vk,
	}
}

// typeConfig is the subset of comparison_type_definitions.config that the
// scoring engine needs. It is kept internal so the engine can evolve the shape
// without changing the public domain types.
type typeConfig struct {
	AllowedGoals            []allowedGoal
	DefaultWeights          map[string]float64
	ProfileDefaults         jce.ComparisonProfile
	EligibilityRules        []map[string]any
	GoalFactorMap           map[string][]string
	SmartModes              map[string]smartModeConfig
	RecommendationTemplates []map[string]any
	ScoringEnabled          bool
}

type allowedGoal struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	Description   string  `json:"description"`
	DefaultWeight float64 `json:"default_weight"`
}

type rawTypeConfig struct {
	AllowedGoals            []allowedGoal              `json:"allowed_goals"`
	DefaultWeights          map[string]float64         `json:"default_weights"`
	ProfileDefaults         rawProfileDefaults         `json:"profile_defaults"`
	EligibilityRules        []map[string]any           `json:"eligibility_rules"`
	GoalFactorMap           map[string][]string        `json:"goal_factor_map"`
	SmartModes              map[string]smartModeConfig `json:"smart_modes"`
	RecommendationTemplates []map[string]any           `json:"recommendation_templates"`
	ScoringEnabled          *bool                      `json:"scoring_enabled"`
}

type rawProfileDefaults struct {
	Demographics jce.ProfileDemographics `json:"demographics"`
	Goals        []jce.ProfileGoal       `json:"goals"`
	Preferences  map[string]float64      `json:"preferences"`
}

type smartModeConfig struct {
	FilterIneligible  bool               `json:"filter_ineligible"`
	FactorMultipliers map[string]float64 `json:"factor_multipliers"`
}

// scoredItem carries a comparison item through the scoring pipeline.
type scoredItem struct {
	id       string
	title    string
	ref      map[string]any
	resolved resolver.ResolvedItem
	values   map[string]itemValue
}

type itemValue struct {
	dimensionID pgtype.UUID
	value       *string
	valueRating pgtype.Numeric
	valueBool   *bool
}

type scoredFactor struct {
	factor         jce.ScoreFactor
	score          float64
	max            float64
	weight         float64
	explanation    string
	eligibleStatus string
	dimensionID    pgtype.UUID
	missing        bool
}

type itemResult struct {
	id             string
	title          string
	japaScore      float64
	eligibleStatus string
	factors        []scoredFactor
}

type scoreRunResult struct {
	scoreResult jce.ScoreResult
	items       []itemResult
}

// Score computes and persists JapaScores and recommendations for a comparison.
// // Ref: docs/superpowers/specs/2026-08-25-jce-subspec-2-design.md §6.5.
func (e *Engine) Score(ctx context.Context, input jce.ScoreInput) (jce.ScoreResult, error) {
	_ = stringToUUID("")

	contentID, err := parseUUID(input.ContentID)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("parsing content_id: %w", err)
	}

	cmpRow, err := e.read.GetComparisonBySlugOrID(ctx, gen.GetComparisonBySlugOrIDParams{

		ID:   contentID,
		Slug: nil,
	})
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading comparison: %w", err)
	}

	publicPerm := input.PublicPerm
	if publicPerm == "" {
		publicPerm = "jce.score.recalculate"
	}

	if !e.canScoreContent(ctx, cmpRow, input.UserID, input.PermissionChecker, publicPerm) {
		return jce.ScoreResult{}, errpkg.ErrForbidden
	}

	items, err := e.read.GetComparisonItemsForScoring(ctx,
		contentID,
	)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading comparison items: %w", err)
	}

	var typeID pgtype.UUID

	for _, it := range items {
		if it.TypeID.Valid {
			typeID = it.TypeID
			break
		}
	}

	var typeCfg typeConfig

	if typeID.Valid {
		typeDef, err := e.read.GetComparisonTypeByID(ctx,
			typeID,
		)
		if err != nil {
			return jce.ScoreResult{}, fmt.Errorf("loading comparison type: %w", err)
		}

		typeCfg, err = parseTypeConfig(typeDef.Config)
		if err != nil {
			return jce.ScoreResult{}, fmt.Errorf("parsing comparison type config: %w", err)
		}

		if !typeCfg.ScoringEnabled {
			return jce.ScoreResult{}, errpkg.ErrScoringDisabled
		}
	}

	profile, profileID, err := e.resolveProfile(ctx, input.UserID, input.ProfileID, typeCfg, input.PermissionChecker)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("resolving comparison profile: %w", err)
	}

	// Force=true bypasses any cached result for the current score_version before
	// recalculating. This is the public cache-bypass contract for Sub-spec 2.
	if input.Force && e.vk != nil {
		_ = e.vk.Del(ctx, jce.ScoreResultKey(contentID, cmpRow.ScoreVersion, profileID.String())).Err()
	}

	// If the caller does not force a refresh, try to return the existing cached
	// result for the current score_version before doing any recomputation.
	if !input.Force && e.vk != nil {
		cacheKey := jce.ScoreResultKey(contentID, cmpRow.ScoreVersion, profileID.String())

		data, err := e.vk.Get(ctx, cacheKey).Bytes()
		if err == nil {
			var cached jce.ScoreResult
			if err := json.Unmarshal(data, &cached); err == nil {
				return cached, nil
			}
		}
	}

	// Load dimension values for dimension-backed factors.
	valuesByItem := make(map[string]map[string]itemValue)

	if len(items) > 0 {
		itemIDs := make([]pgtype.UUID, len(items))
		for i, it := range items {
			itemIDs[i] = it.ID
		}

		valueRows, err := e.read.GetComparisonValuesForScoring(ctx, gen.GetComparisonValuesForScoringParams{

			ContentID: contentID,
			ItemIds:   itemIDs,
		})
		if err != nil {
			return jce.ScoreResult{}, fmt.Errorf("loading comparison values: %w", err)
		}

		for _, v := range valueRows {
			if v.DimensionSlug == nil {
				continue
			}

			itemKey := v.ItemID.String()
			if valuesByItem[itemKey] == nil {
				valuesByItem[itemKey] = make(map[string]itemValue)
			}

			valuesByItem[itemKey][*v.DimensionSlug] = itemValue{
				dimensionID: v.DimensionID,
				value:       v.Value,
				valueRating: v.ValueRating,
				valueBool:   v.ValueBoolean,
			}
		}
	}

	factors, err := e.listScoreFactors(ctx, typeID)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading score factors: %w", err)
	}

	// Merge type default_weights with factor default_weights so new factors are
	// weighted even when the type config has not been updated.
	runCfg := typeCfg
	runCfg.DefaultWeights = make(map[string]float64, len(typeCfg.DefaultWeights)+len(factors))

	for _, f := range factors {
		runCfg.DefaultWeights[f.FactorKey] = f.DefaultWeight
	}

	for k, v := range typeCfg.DefaultWeights {
		runCfg.DefaultWeights[k] = v
	}

	scoredItems := make([]scoredItem, len(items))

	for i, it := range items {
		ref, _ := resolver.CoerceMap(it.ItemRef)
		if ref == nil {
			ref = map[string]any{}
		}

		scoredItems[i] = scoredItem{
			id:     it.ID.String(),
			title:  it.Title,
			ref:    ref,
			values: valuesByItem[it.ID.String()],
		}
	}

	if err := e.resolveItems(ctx, scoredItems); err != nil {
		return jce.ScoreResult{}, fmt.Errorf("resolving item refs: %w", err)
	}

	smartMode := input.SmartMode
	if smartMode == "" {
		if cmpRow.SmartMode != nil {
			smartMode = *cmpRow.SmartMode
		} else {
			smartMode = comparison.SmartModeNone
		}
	}

	newVersion := cmpRow.ScoreVersion + 1

	var result jce.ScoreResult

	// Persist scores, recommendations, and metadata inside a transaction.
	factorIDByKey := make(map[string]pgtype.UUID)
	for _, f := range factors {
		factorIDByKey[f.FactorKey] = stringToUUID(f.ID)
	}

	var savedRecs []jce.ComparisonRecommendation

	err = db.WithTx(ctx, e.database.Writer, func(q *gen.Queries) error {
		if err := q.DeleteComparisonScoresByContent(ctx, gen.DeleteComparisonScoresByContentParams{

			ContentID: contentID,
			ProfileID: profileID,
		}); err != nil {
			return fmt.Errorf("deleting existing scores: %w", err)
		}

		if err := q.DeleteComparisonRecommendationsByContent(ctx, gen.DeleteComparisonRecommendationsByContentParams{

			ContentID: contentID,
			ProfileID: profileID,
		}); err != nil {
			return fmt.Errorf("deleting existing recommendations: %w", err)
		}

		// Recompute inside the transaction so the scoredFactor metadata is
		// available for persistence (dimension_id, eligible_status, etc.).
		txResult, err := e.scoreRun(ctx, runCfg, factors, profile, scoredItems, smartMode)
		if err != nil {
			return fmt.Errorf("recomputing scores in transaction: %w", err)
		}

		for _, ir := range txResult.items {
			itemID := stringToUUID(ir.id)

			for _, sf := range ir.factors {
				factorID, ok := factorIDByKey[sf.factor.FactorKey]
				if !ok || !factorID.Valid {
					continue
				}

				if _, err := q.UpsertComparisonScore(ctx, gen.UpsertComparisonScoreParams{

					ContentID:      contentID,
					ItemID:         itemID,
					FactorID:       factorID,
					DimensionID:    sf.dimensionID,
					ProfileID:      profileID,
					ScoreValue:     numericFromFloat(sf.score),
					MaxValue:       numericFromFloat(sf.max),
					Explanation:    toNullableString(sf.explanation),
					EligibleStatus: toNullableString(sf.eligibleStatus),
					Version:        newVersion,
				}); err != nil {
					return fmt.Errorf("upserting score for %s: %w", sf.factor.FactorKey, err)
				}
			}
		}

		for _, rec := range txResult.scoreResult.Recommendations {
			itemID := stringToUUID(ptrString(rec.ItemID))

			row, err := q.CreateComparisonRecommendation(ctx, gen.CreateComparisonRecommendationParams{

				ContentID:  contentID,
				ItemID:     itemID,
				ProfileID:  profileID,
				FactorID:   stringToUUID(ptrString(rec.FactorID)),
				Text:       rec.Text,
				ActionType: rec.ActionType,
				ActionUrl:  rec.ActionURL,
				Priority:   safeInt32(rec.Priority),
			})
			if err != nil {
				return fmt.Errorf("creating recommendation: %w", err)
			}

			savedRecs = append(savedRecs, jce.ComparisonRecommendation{
				ID:         row.ID.String(),
				ItemID:     uuidPtr(row.ItemID),
				FactorID:   uuidPtr(row.FactorID),
				Text:       row.Text,
				ActionType: row.ActionType,
				ActionURL:  row.ActionUrl,
				Priority:   int(row.Priority),
			})
		}

		configMap := map[string]any{}
		if len(cmpRow.MetadataConfig) > 0 && string(cmpRow.MetadataConfig) != "null" {
			if err := json.Unmarshal(cmpRow.MetadataConfig, &configMap); err != nil {
				return fmt.Errorf("parsing metadata config: %w", err)
			}
		}

		if configMap == nil {
			configMap = map[string]any{}
		}

		var profileIDAny any
		if profileID.Valid {
			profileIDAny = profileID.String()
		} else {
			profileIDAny = nil
		}

		configMap["profile_id"] = profileIDAny
		configMap["weights_snapshot"] = txResult.scoreResult.WeightsSnapshot
		configMap["recommendation_summary"] = txResult.scoreResult.RecommendationSummary
		configMap["last_scored_at"] = txResult.scoreResult.RecommendationSummary["last_scored_at"]

		configBytes, err := json.Marshal(configMap)
		if err != nil {
			return fmt.Errorf("marshaling metadata config: %w", err)
		}

		depth := cmpRow.Depth
		if err := q.UpdateComparisonContentMetadata(ctx, gen.UpdateComparisonContentMetadataParams{
			Depth:          &depth,
			PromotedFromID: cmpRow.PromotedFromID,
			PromotedAt:     cmpRow.PromotedAt,
			SmartMode:      &smartMode,
			ScoreVersion:   &newVersion,
			Config:         configBytes,
			ContentID:      contentID,
		}); err != nil {
			return fmt.Errorf("updating comparison metadata: %w", err)
		}

		result = txResult.scoreResult

		return nil
	})
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("persisting score result: %w", err)
	}

	result.ContentID = cmpRow.ID.String()
	result.Version = int(newVersion)
	result.SmartMode = smartMode

	if len(savedRecs) > 0 {
		result.Recommendations = savedRecs
	}

	if profileID.Valid {
		s := profileID.String()
		result.ProfileID = &s
	}

	// Cache the result and track it in a profile-scoped set so that profile
	// mutations can invalidate every cached score result for this profile.
	if e.vk != nil {
		cacheKey := jce.ScoreResultKey(contentID, newVersion, profileID.String())

		data, err := json.Marshal(result)
		if err != nil {
			return jce.ScoreResult{}, fmt.Errorf("marshaling score result for cache: %w", err)
		}

		ttl := 15 * time.Minute
		if e.settings != nil {
			ttl = time.Duration(e.settings.GetIntOr(ctx, "jce.score_cache_ttl_minutes", 15)) * time.Minute
		}

		_ = e.vk.Set(ctx, cacheKey, data, ttl).Err()
		if profileID.Valid {
			_ = e.vk.SAdd(ctx, jce.ScoreResultSetKey(profileID), cacheKey).Err()
		}
	}

	return result, nil
}

// GetScore returns a previously computed ScoreResult from cache or DB.
func (e *Engine) GetScore(ctx context.Context, input jce.GetScoreInput) (jce.ScoreResult, error) {
	_ = stringToUUID("")

	contentID, err := parseUUID(input.ContentID)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("parsing content_id: %w", err)
	}

	cmpRow, err := e.read.GetComparisonBySlugOrID(ctx, gen.GetComparisonBySlugOrIDParams{

		ID:   contentID,
		Slug: nil,
	})
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading comparison: %w", err)
	}

	publicPerm := input.PublicPerm
	if publicPerm == "" {
		publicPerm = "jce.score.view"
	}

	if !e.canScoreContent(ctx, cmpRow, input.UserID, input.PermissionChecker, publicPerm) {
		return jce.ScoreResult{}, errpkg.ErrForbidden
	}

	// Resolve the profile ID to use for the query and enforce ownership
	// before reading any cached or persisted data.
	profileID := e.profileIDFromMetadata(input, cmpRow.MetadataConfig)
	if err := e.ensureProfileAccess(ctx, input.UserID, profileID, input.PermissionChecker); err != nil {
		return jce.ScoreResult{}, err
	}

	// Check Valkey cache second, after access is established.
	if e.vk != nil {
		cacheKey := jce.ScoreResultKey(contentID, cmpRow.ScoreVersion, profileID.String())

		data, err := e.vk.Get(ctx, cacheKey).Bytes()
		if err == nil {
			var cached jce.ScoreResult
			if err := json.Unmarshal(data, &cached); err == nil {
				return cached, nil
			}
		}
	}

	// Load persisted scores and recommendations.
	scores, err := e.read.ListComparisonScoresByContent(ctx, gen.ListComparisonScoresByContentParams{

		ContentID: contentID,
		ProfileID: profileID,
	})
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading comparison scores: %w", err)
	}

	recs, err := e.read.ListComparisonRecommendationsByContent(ctx, gen.ListComparisonRecommendationsByContentParams{

		ContentID: contentID,
		ProfileID: profileID,
	})
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading comparison recommendations: %w", err)
	}

	// GetScore is a read path: it returns a previously computed result from
	// cache or DB. If no result has been computed for the requested profile,
	// the caller must explicitly POST /jce/comparisons/{id}/score to trigger
	// a recalculation. Falling back to Score() here caused the last computed
	// comparison metadata (profile_id, score_version) to be overwritten by
	// accidental GETs (e.g. the JCE detail page loading with no profile).
	if len(scores) == 0 && len(recs) == 0 {
		return jce.ScoreResult{}, errpkg.ErrNotFound
	}

	// Load items and type to build factor metadata.
	items, err := e.read.GetComparisonItemsForScoring(ctx,
		contentID,
	)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading comparison items: %w", err)
	}

	var typeID pgtype.UUID

	for _, it := range items {
		if it.TypeID.Valid {
			typeID = it.TypeID
			break
		}
	}

	factors, err := e.listScoreFactors(ctx, typeID)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading score factors: %w", err)
	}

	factorByID := make(map[string]jce.ScoreFactor)
	for _, f := range factors {
		factorByID[f.ID] = f
	}

	weightsSnapshot, summary := e.parseMetadataConfig(cmpRow.MetadataConfig)

	itemByID := make(map[string]gen.GetComparisonItemsForScoringRow)
	for _, it := range items {
		itemByID[it.ID.String()] = it
	}

	scoresByItem := make(map[string][]jce.FactorScore)
	japaByItem := make(map[string]float64)
	statusByItem := make(map[string]string)

	for _, row := range scores {
		factor, ok := factorByID[row.FactorID.String()]
		if !ok {
			continue
		}

		itemID := row.ItemID.String()
		exp := ptrString(row.Explanation)
		fs := jce.FactorScore{
			FactorKey:    factor.FactorKey,
			Label:        factor.Label,
			ScoreValue:   numericToFloat64(row.ScoreValue),
			MaxValue:     numericToFloat64(row.MaxValue),
			Weight:       weightsSnapshot[factor.FactorKey],
			Explanation:  exp,
			Missing:      isMissingExplanation(exp),
			MissingScore: factor.Config.MissingScore,
		}
		scoresByItem[itemID] = append(scoresByItem[itemID], fs)

		if factor.FactorKey == "japa_score" {
			japaByItem[itemID] = numericToFloat64(row.ScoreValue)

			if row.EligibleStatus != nil {
				statusByItem[itemID] = *row.EligibleStatus
			}
		}

		if factor.FactorKey == "eligibility" && row.EligibleStatus != nil {
			statusByItem[itemID] = *row.EligibleStatus
		}
	}

	// The scoring engine does not persist a separate synthetic japa_score
	// factor row, so read paths that fall through the Valkey cache must
	// reconstruct the overall JapaScore from the persisted factor scores.
	for itemID, fss := range scoresByItem {
		if _, ok := japaByItem[itemID]; ok {
			continue
		}

		var sumScore, sumWeight float64

		for _, fs := range fss {
			if fs.FactorKey == "japa_score" || fs.FactorKey == "eligibility" {
				continue
			}

			if !fs.Missing && fs.Weight > 0 {
				sumScore += fs.ScoreValue * fs.Weight
				sumWeight += fs.Weight
			}
		}

		if sumWeight > 0 {
			japaByItem[itemID] = sumScore / sumWeight
		}
	}

	itemIDs := make([]string, 0, len(scoresByItem))
	for id := range scoresByItem {
		itemIDs = append(itemIDs, id)
	}

	sort.Slice(itemIDs, func(i, j int) bool {
		ii, oki := itemByID[itemIDs[i]]
		ij, okj := itemByID[itemIDs[j]]

		if !oki || !okj {
			return itemIDs[i] < itemIDs[j]
		}

		return ii.Position < ij.Position
	})

	japaScores := make([]jce.JapaScore, 0, len(itemIDs))

	for _, itemID := range itemIDs {
		it := itemByID[itemID]
		japaScore := japaByItem[itemID]
		japaScores = append(japaScores, jce.JapaScore{
			ItemID:         itemID,
			ItemTitle:      it.Title,
			JapaScore:      japaScore,
			MaxValue:       100,
			EligibleStatus: statusByItem[itemID],
			FactorScores:   scoresByItem[itemID],
			Explanation:    fmt.Sprintf("Overall JapaScore: %.1f", japaScore),
		})
	}

	recommendations := make([]jce.ComparisonRecommendation, 0, len(recs))
	for _, row := range recs {
		recommendations = append(recommendations, jce.ComparisonRecommendation{
			ID:         row.ID.String(),
			ItemID:     uuidPtr(row.ItemID),
			FactorID:   uuidPtr(row.FactorID),
			Text:       row.Text,
			ActionType: row.ActionType,
			ActionURL:  row.ActionUrl,
			Priority:   int(row.Priority),
		})
	}

	smartMode := ""
	if cmpRow.SmartMode != nil {
		smartMode = *cmpRow.SmartMode
	}

	var resultProfileID *string

	if profileID.Valid {
		s := profileID.String()
		resultProfileID = &s
	}

	return jce.ScoreResult{
		ContentID:             cmpRow.ID.String(),
		ProfileID:             resultProfileID,
		SmartMode:             smartMode,
		Scores:                japaScores,
		Recommendations:       recommendations,
		WeightsSnapshot:       weightsSnapshot,
		RecommendationSummary: summary,
		Version:               int(cmpRow.ScoreVersion),
	}, nil
}

// PreviewScoreForItems scores a list of item refs without creating content.
func (e *Engine) PreviewScoreForItems(ctx context.Context, input jce.ScorePreviewInput) (jce.ScoreResult, error) {
	_ = stringToUUID("")

	typeID, err := parseUUID(input.TypeID)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("parsing type_id: %w", err)
	}

	typeDef, err := e.read.GetComparisonTypeByID(ctx,
		typeID,
	)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading comparison type: %w", err)
	}

	typeCfg, err := parseTypeConfig(typeDef.Config)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("parsing comparison type config: %w", err)
	}

	if !typeCfg.ScoringEnabled {
		return jce.ScoreResult{}, errpkg.ErrScoringDisabled
	}

	// Preview is treated as public content: the caller must be authenticated and
	// have jce.score.view, or have comparison.edit_any.
	if !e.canScoreContent(ctx, gen.GetComparisonBySlugOrIDRow{Visibility: content.VisibilityPublic}, input.UserID, input.PermissionChecker, "jce.score.view") {
		return jce.ScoreResult{}, errpkg.ErrForbidden
	}

	var profile jce.ComparisonProfile
	if input.ProfileInput != nil {
		profile = profileFromInput(*input.ProfileInput)
	} else {
		profile = defaultProfileFromTypeConfig(typeCfg)
	}

	scoredItems := make([]scoredItem, len(input.ItemRefs))

	for i, ref := range input.ItemRefs {
		if ref == nil {
			ref = map[string]any{}
		}

		scoredItems[i] = scoredItem{ref: ref}
	}

	if err := e.resolveItems(ctx, scoredItems); err != nil {
		return jce.ScoreResult{}, fmt.Errorf("resolving item refs: %w", err)
	}

	for i := range scoredItems {
		si := &scoredItems[i]
		if si.resolved.ID != "" {
			si.id = si.resolved.ID
		} else if id, ok := si.ref["id"].(string); ok {
			si.id = id
		} else {
			si.id = uuid.New().String()
		}

		if si.resolved.Title != "" {
			si.title = si.resolved.Title
		} else if t, ok := si.ref["title"].(string); ok {
			si.title = t
		}
	}

	factors, err := e.listScoreFactors(ctx, typeID)
	if err != nil {
		return jce.ScoreResult{}, fmt.Errorf("loading score factors: %w", err)
	}

	// Merge type and factor default weights.
	runCfg := typeCfg
	runCfg.DefaultWeights = make(map[string]float64, len(typeCfg.DefaultWeights)+len(factors))

	for _, f := range factors {
		runCfg.DefaultWeights[f.FactorKey] = f.DefaultWeight
	}

	for k, v := range typeCfg.DefaultWeights {
		runCfg.DefaultWeights[k] = v
	}

	runResult, err := e.scoreRun(ctx, runCfg, factors, profile, scoredItems, input.SmartMode)
	if err != nil {
		return jce.ScoreResult{}, err
	}

	result := runResult.scoreResult
	result.ContentID = ""
	result.Version = 0
	result.ProfileID = nil
	result.SmartMode = input.SmartMode

	return result, nil
}

// scoreRun is the core scoring pipeline. It is split out so tests and the
// preview path can drive it with in-memory data.
func (e *Engine) scoreRun(ctx context.Context, cfg typeConfig, factors []jce.ScoreFactor, profile jce.ComparisonProfile, items []scoredItem, smartMode string) (scoreRunResult, error) {
	weights := buildWeightMap(cfg, profile, smartMode)

	factorByKey := make(map[string]jce.ScoreFactor)
	for _, f := range factors {
		factorByKey[f.FactorKey] = f
	}

	// Ensure a japa_score aggregate factor exists.
	japaFactor, ok := factorByKey["japa_score"]
	if !ok {
		japaFactor = jce.ScoreFactor{
			FactorKey:     "japa_score",
			Label:         "JapaScore",
			DataSource:    "aggregate",
			DefaultWeight: 0,
			Config:        jce.ScoreFactorConfig{IsOutput: true, Max: 100},
		}
	}

	itemResults := make([]itemResult, len(items))
	topItemID := ""
	topScore := -1.0

	for i, it := range items {
		factorsList := make([]scoredFactor, 0, len(factors))

		var eligibleStatus string

		for _, f := range factors {
			if f.DataSource == "aggregate" {
				continue
			}

			sf := e.computeFactorScore(profile, it, f, cfg, weights)
			if f.DataSource == "eligibility" && sf.eligibleStatus != "" {
				eligibleStatus = sf.eligibleStatus
			}

			factorsList = append(factorsList, sf)
		}

		if eligibleStatus == "" {
			eligibleStatus = "eligible"
		}

		// Compute weighted JapaScore from non-aggregate factor scores.
		var sumScore, sumWeight float64

		for _, sf := range factorsList {
			if sf.factor.DataSource == "aggregate" || sf.missing {
				continue
			}

			sumScore += sf.score * sf.weight
			sumWeight += sf.weight
		}

		var japaScore float64
		if sumWeight > 0 {
			japaScore = sumScore / sumWeight
		}

		if japaScore < 0 {
			japaScore = 0
		} else if japaScore > 100 {
			japaScore = 100
		}

		japaSF := scoredFactor{
			factor:         japaFactor,
			score:          japaScore,
			max:            100,
			weight:         weights[japaFactor.FactorKey],
			explanation:    fmt.Sprintf("JapaScore: %.1f", japaScore),
			eligibleStatus: eligibleStatus,
		}
		factorsList = append(factorsList, japaSF)

		itemResults[i] = itemResult{
			id:             it.id,
			title:          it.title,
			japaScore:      japaScore,
			eligibleStatus: eligibleStatus,
			factors:        factorsList,
		}

		if eligibleStatus != "ineligible" && japaScore > topScore {
			topScore = japaScore
			topItemID = it.id
		}
	}

	// When the active smart mode requests it, drop ineligible items from the
	// returned result and recommendations only. The full itemResults list is
	// still persisted so later GetScore calls can render grey/collapsed
	// ineligible items with explanations.
	displayedItems := itemResults

	if sm, ok := cfg.SmartModes[smartMode]; ok && sm.FilterIneligible {
		filtered := make([]itemResult, 0, len(itemResults))

		for _, ir := range itemResults {
			if ir.eligibleStatus != "ineligible" {
				filtered = append(filtered, ir)
			}
		}

		displayedItems = filtered
	}

	// Persist a normalized weights snapshot so the UI and metadata always
	// show a map that sums to 1, while the scoring math uses the raw weights.
	normalizedWeights := normalizeWeights(weights)

	// Build domain scores. Use the normalized weight snapshot so the UI and
	// cached GetScore results expose the same scale.
	scores := make([]jce.JapaScore, 0, len(displayedItems))

	for _, ir := range displayedItems {
		fs := make([]jce.FactorScore, len(ir.factors))

		for j, sf := range ir.factors {
			weight := normalizedWeights[sf.factor.FactorKey]
			if sf.factor.DataSource == "aggregate" {
				weight = 0
			}

			fs[j] = jce.FactorScore{
				FactorKey:    sf.factor.FactorKey,
				Label:        sf.factor.Label,
				ScoreValue:   sf.score,
				MaxValue:     sf.max,
				Weight:       weight,
				Explanation:  sf.explanation,
				Missing:      sf.missing,
				MissingScore: sf.factor.Config.MissingScore,
			}
		}

		scores = append(scores, jce.JapaScore{
			ItemID:         ir.id,
			ItemTitle:      ir.title,
			JapaScore:      ir.japaScore,
			MaxValue:       100,
			EligibleStatus: ir.eligibleStatus,
			FactorScores:   fs,
			Explanation:    fmt.Sprintf("Overall JapaScore: %.1f", ir.japaScore),
		})
	}

	// Generate recommendations from templates.
	var recommendations []jce.ComparisonRecommendation

	for _, ir := range displayedItems {
		for _, sf := range ir.factors {
			recs := generateRecommendations(ir.id, sf.factor.ID, sf.factor.FactorKey, sf.score, ir.eligibleStatus, cfg.RecommendationTemplates)
			recommendations = append(recommendations, recs...)
		}
	}

	if topItemID != "" {
		recommendations = append(recommendations, jce.ComparisonRecommendation{
			ItemID:   &topItemID,
			Text:     bestFitRecommendationText(cfg.RecommendationTemplates),
			Priority: 100,
		})
	}

	summary := map[string]any{
		"top_item_id":           topItemID,
		"top_japa_score":        topScore,
		"recommendations_count": len(recommendations),
		"last_scored_at":        time.Now().UTC().Format(time.RFC3339),
	}

	return scoreRunResult{
		scoreResult: jce.ScoreResult{
			Scores:                scores,
			Recommendations:       recommendations,
			WeightsSnapshot:       normalizedWeights,
			RecommendationSummary: summary,
		},
		items: itemResults,
	}, nil
}

// computeFactorScore evaluates one factor for one item.
func (e *Engine) computeFactorScore(profile jce.ComparisonProfile, it scoredItem, factor jce.ScoreFactor, cfg typeConfig, weights map[string]float64) scoredFactor {
	sf := scoredFactor{
		factor:      factor,
		max:         100,
		weight:      weights[factor.FactorKey],
		score:       factor.Config.MissingScore,
		explanation: factor.FactorKey + ": missing input",
		missing:     true,
	}

	inputs, _ := it.resolved.Metadata["score_inputs"].(map[string]any)
	if inputs == nil {
		inputs = map[string]any{}
	}

	switch factor.DataSource {
	case "dimension":
		if factor.DimensionSlug == nil {
			return sf
		}

		iv, ok := it.values[*factor.DimensionSlug]
		if !ok {
			sf.explanation = factor.FactorKey + ": no dimension value"
			return sf
		}

		val, ok := valueToFloat(iv, factor.Config)
		if !ok {
			sf.explanation = factor.FactorKey + ": invalid dimension value"
			return sf
		}

		sf.score = normalizeNumber(val, factor.Config.Min, factor.Config.Max, factor.Config.Ideal, factor.Config.Direction)
		sf.dimensionID = iv.dimensionID
		sf.explanation = fmt.Sprintf("%s: dimension score %.1f", factor.FactorKey, sf.score)
		sf.missing = false

	case "resolver_metadata":
		v, ok := inputs[factor.Config.ValueKey]
		if !ok || v == nil || isMissingResolverValue(v, factor.Config) {
			sf.explanation = factor.FactorKey + ": No data"
			return sf
		}

		sf.score = scoreFromResolverValue(v, factor.Config)
		sf.explanation = fmt.Sprintf("%s: score %.1f", factor.FactorKey, sf.score)
		sf.missing = false

	case "profile":
		profileVal := getProfileValue(profile, factor.Config.ProfileKey)
		itemVal, ok := inputs[factor.Config.ItemKey]

		if !ok || itemVal == nil || isMissingValue(profileVal) || isMissingValue(itemVal) {
			sf.explanation = factor.FactorKey + ": No data"
			return sf
		}

		sf.score = compareProfileItem(profileVal, itemVal, factor.Config)
		sf.explanation = fmt.Sprintf("%s: match score %.1f", factor.FactorKey, sf.score)
		sf.missing = false

	case "eligibility":
		status, score, exp := evaluateEligibilityRules(profile, inputs, cfg.EligibilityRules, factor.Config)
		sf.score = score
		sf.eligibleStatus = status
		sf.explanation = exp
		sf.missing = false
	}

	return sf
}

// resolveItems resolves item_ref maps through the resolver registry.
func (e *Engine) resolveItems(ctx context.Context, items []scoredItem) error {
	if e.r == nil {
		for i := range items {
			if items[i].resolved.Metadata == nil {
				items[i].resolved.Metadata = map[string]any{}
			}
		}

		return nil
	}

	for i := range items {
		if len(items[i].ref) == 0 {
			items[i].resolved = resolver.ResolvedItem{Metadata: map[string]any{}}
			continue
		}

		resolved, err := e.r.Resolve(ctx, items[i].ref)
		if err != nil {
			resolved = resolver.ResolvedItem{Metadata: map[string]any{}}
		} else if resolved.Metadata == nil {
			resolved.Metadata = map[string]any{}
		}

		items[i].resolved = resolved
	}

	return nil
}

// listScoreFactors loads type-specific and global score factors, preferring
// type-specific overrides for the same factor_key.
func (e *Engine) listScoreFactors(ctx context.Context, typeID pgtype.UUID) ([]jce.ScoreFactor, error) {
	includeGlobal := true

	rows, err := e.read.ListScoreFactorsByType(ctx, gen.ListScoreFactorsByTypeParams{

		ComparisonTypeID: typeID,
		IncludeGlobal:    &includeGlobal,
		DataSource:       nil,
	})
	if err != nil {
		return nil, fmt.Errorf("listing score factors: %w", err)
	}

	byKey := make(map[string]jce.ScoreFactor)

	for _, row := range rows {
		f, err := jce.ScoreFactorFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("mapping score factor: %w", err)
		}

		if existing, ok := byKey[f.FactorKey]; ok {
			// Prefer the type-specific override over the global fallback.
			if f.ComparisonTypeID != nil && existing.ComparisonTypeID == nil {
				byKey[f.FactorKey] = f
			}

			continue
		}

		byKey[f.FactorKey] = f
	}

	factors := make([]jce.ScoreFactor, 0, len(byKey))
	for _, f := range byKey {
		factors = append(factors, f)
	}

	sort.Slice(factors, func(i, j int) bool { return factors[i].FactorKey < factors[j].FactorKey })

	return factors, nil
}

// resolveProfile loads the requested profile or builds one from type defaults.
func (e *Engine) resolveProfile(ctx context.Context, userID, profileID *string, cfg typeConfig, check func(string) bool) (jce.ComparisonProfile, pgtype.UUID, error) {
	if profileID == nil || *profileID == "" {
		return defaultProfileFromTypeConfig(cfg), pgtype.UUID{}, nil
	}

	pid, err := parseUUID(*profileID)
	if err != nil {
		return jce.ComparisonProfile{}, pgtype.UUID{}, fmt.Errorf("parsing profile_id: %w", err)
	}

	// Optional Valkey cache for profiles.
	if e.vk != nil && userID != nil && *userID != "" {
		uid, err := parseUUID(*userID)
		if err == nil {
			cacheKey := jce.ProfileCacheKey(uid, pid)

			data, err := e.vk.Get(ctx, cacheKey).Bytes()
			if err == nil {
				var cached jce.ComparisonProfile
				if err := json.Unmarshal(data, &cached); err == nil {
					if e.canUseProfile(cached, userID, check) {
						return cached, pid, nil
					}
				}
			}
		}
	}

	row, err := e.read.GetComparisonProfile(ctx,
		pid,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return jce.ComparisonProfile{}, pgtype.UUID{}, errpkg.ErrNotFound
		}

		return jce.ComparisonProfile{}, pgtype.UUID{}, fmt.Errorf("loading comparison profile: %w", err)
	}

	profile, err := jce.ProfileFromRow(row)
	if err != nil {
		return jce.ComparisonProfile{}, pgtype.UUID{}, fmt.Errorf("mapping comparison profile: %w", err)
	}

	if !e.canUseProfile(profile, userID, check) {
		return jce.ComparisonProfile{}, pgtype.UUID{}, errpkg.ErrForbidden
	}

	if e.vk != nil && userID != nil && *userID != "" {
		uid, err := parseUUID(*userID)
		if err == nil {
			cacheKey := jce.ProfileCacheKey(uid, pid)
			data, _ := json.Marshal(profile)

			ttl := 5 * time.Minute
			if e.settings != nil {
				ttl = time.Duration(e.settings.GetIntOr(ctx, "jce.profile_cache_ttl_minutes", 5)) * time.Minute
			}

			_ = e.vk.Set(ctx, cacheKey, data, ttl).Err()
		}
	}

	return profile, pid, nil
}

// canScoreContent performs a content-visibility and permission check.
// It is not an RBAC check itself: the service/handler layer (Task 5)
// populates input.PermissionChecker with a callback against internal/rbac.
// // - the comparison owner is always allowed;
// - callers with comparison.edit_any are always allowed;
// - public content may be scored/viewed by authenticated callers with the
// supplied public permission (jce.score.recalculate for Score,
// jce.score.view for GetScore and PreviewScoreForItems);
// - members and private content require the caller to be the author or have
// comparison.edit_any.
// // A nil PermissionChecker is treated as a deny-all fallback for non-owners.
func (e *Engine) canScoreContent(ctx context.Context, cmp gen.GetComparisonBySlugOrIDRow, userID *string, check func(string) bool, publicPerm string) bool {
	if userID == nil || *userID == "" {
		return false
	}

	uid, err := parseUUID(*userID)
	if err == nil && cmp.AuthorID.Valid && cmp.AuthorID == uid {
		return true
	}

	if check == nil {
		return false
	}

	if check("comparison.edit_any") {
		return true
	}

	if cmp.Visibility == content.VisibilityPublic && check(publicPerm) {
		return true
	}

	return false
}

// canUseProfile decides whether a profile may be used for scoring by the caller.
// Templates are public within the tenant; private profiles are restricted to
// their owner or callers with comparison.edit_any.
func (e *Engine) canUseProfile(profile jce.ComparisonProfile, userID *string, check func(string) bool) bool {
	if profile.IsTemplate {
		return true
	}

	if userID == nil || *userID == "" {
		return false
	}

	if profile.UserID != nil && *profile.UserID == *userID {
		return true
	}

	if check != nil && check("comparison.edit_any") {
		return true
	}

	return false
}

// ensureProfileAccess loads a profile and verifies the caller may use it.
func (e *Engine) ensureProfileAccess(ctx context.Context, userID *string, profileID pgtype.UUID, check func(string) bool) error {
	if !profileID.Valid {
		return nil
	}

	row, err := e.read.GetComparisonProfile(ctx,
		profileID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errpkg.ErrNotFound
		}

		return fmt.Errorf("loading profile for access check: %w", err)
	}

	profile, err := jce.ProfileFromRow(row)
	if err != nil {
		return fmt.Errorf("mapping profile for access check: %w", err)
	}

	if !e.canUseProfile(profile, userID, check) {
		return errpkg.ErrForbidden
	}

	return nil
}

// profileIDFromMetadata extracts the stored profile id from metadata config.
func (e *Engine) profileIDFromMetadata(input jce.GetScoreInput, config []byte) pgtype.UUID {
	if input.ProfileID != nil && *input.ProfileID != "" {
		return stringToUUID(*input.ProfileID)
	}

	if len(config) == 0 || string(config) == "null" || string(config) == "{}" {
		return pgtype.UUID{}
	}

	var m map[string]any
	if err := json.Unmarshal(config, &m); err != nil {
		return pgtype.UUID{}
	}

	v, ok := m["profile_id"]
	if !ok {
		return pgtype.UUID{}
	}

	s, ok := v.(string)
	if !ok || s == "" {
		return pgtype.UUID{}
	}

	return stringToUUID(s)
}

// parseMetadataConfig returns the weights snapshot and recommendation summary
// stored in comparison_content_metadata.config.
func (e *Engine) parseMetadataConfig(config []byte) (map[string]float64, map[string]any) {
	weights := make(map[string]float64)
	summary := make(map[string]any)

	if len(config) == 0 || string(config) == "null" || string(config) == "{}" {
		return weights, summary
	}

	var m map[string]any
	if err := json.Unmarshal(config, &m); err != nil {
		return weights, summary
	}

	if v, ok := m["weights_snapshot"].(map[string]any); ok {
		for k, val := range v {
			if f, ok := anyToFloat(val); ok {
				weights[k] = f
			}
		}
	}

	if v, ok := m["recommendation_summary"].(map[string]any); ok {
		summary = v
	}

	return weights, summary
}

// normalizeNumber converts a raw number to a 0–100 score based on min/max,
// ideal, and direction. For "closer_to_ideal" it delegates to closerToIdealScore.
func normalizeNumber(value, min, max, ideal float64, direction string) float64 {
	if direction == "closer_to_ideal" {
		return closerToIdealScore(value, min, max, ideal)
	}

	if min == 0 && max == 0 {
		max = 100
	}

	if min == max {
		return 50
	}

	if value < min {
		value = min
	}

	if value > max {
		value = max
	}

	var score float64

	switch direction {
	case "lower_is_better":
		score = (max - value) / (max - min) * 100
	default:
		// higher_is_better and any unknown direction.
		score = (value - min) / (max - min) * 100
	}

	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	return score
}

// closerToIdealScore returns 100 at the ideal value and 0 at the farthest end
// of the [min, max] range from ideal, linear in between. The input value is
// clamped to [min, max] first.
func closerToIdealScore(value, min, max, ideal float64) float64 {
	if value < min {
		value = min
	}

	if value > max {
		value = max
	}

	scale := math.Abs(max - ideal)
	if d := math.Abs(ideal - min); d > scale {
		scale = d
	}

	if scale == 0 {
		return 50
	}

	distance := math.Abs(value - ideal)

	score := (1 - distance/scale) * 100
	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	return score
}

// matchExact reports a 0–100 exact-match score. String values are compared
// after trimming and lower-casing so that "English" and "english" match.
// Array values match when the scalar is a normalized member of the array,
// or vice-versa.
func matchExact(a, b any) float64 {
	if a == nil || b == nil {
		return 0
	}

	// Normalize []string slices to []any so that matching against []any is
	// symmetric and recurses through the same path.
	if as, ok := a.([]string); ok {
		av := make([]any, len(as))
		for i, s := range as {
			av[i] = s
		}

		a = av
	}

	if bs, ok := b.([]string); ok {
		bv := make([]any, len(bs))
		for i, s := range bs {
			bv[i] = s
		}

		b = bv
	}

	if isEqual(a, b) {
		return 100
	}

	normalized := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

	switch x := a.(type) {
	case string:
		if y, ok := b.(string); ok {
			if normalized(x) == normalized(y) {
				return 100
			}

			return 0
		}

		return matchExact(b, x)
	case []any:
		for _, v := range x {
			if matchExact(v, b) == 100 {
				return 100
			}
		}

		return 0
	}

	if y, ok := b.(string); ok {
		if normalized(fmt.Sprint(a)) == normalized(y) {
			return 100
		}
	}

	return 0
}

// toStringSet normalizes a scalar, slice, or comma-separated string into a set
// of lower-cased, trimmed strings. It is nil-safe and returns nil for empty
// or unrepresentable input.
func toStringSet(a any) []string {
	if a == nil {
		return nil
	}

	switch v := a.(type) {
	case []string:
		out := make([]string, 0, len(v))

		for _, s := range v {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, strings.ToLower(s))
			}
		}

		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			out = append(out, toStringSet(e)...)
		}

		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}

		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))

		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, strings.ToLower(s))
			}
		}

		return out
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" {
			return nil
		}

		return []string{strings.ToLower(s)}
	}
}

// compareProfileItem scores a profile value against an item value using the
// configured match strategy and range parameters.
func compareProfileItem(profileVal, itemVal any, cfg jce.ScoreFactorConfig) float64 {
	switch cfg.MatchStrategy {
	case "exact":
		return matchExact(profileVal, itemVal)
	case "contains", "in":
		if containsValue(profileVal, itemVal) || containsValue(itemVal, profileVal) {
			return 100
		}

		return 0
	case "budget_lte":
		p, ok1 := anyToFloat(profileVal)
		i, ok2 := anyToFloat(itemVal)

		if !ok1 || !ok2 {
			return 0
		}

		if i <= p {
			return 100
		}

		return 0
	case "budget_gte":
		p, ok1 := anyToFloat(profileVal)
		i, ok2 := anyToFloat(itemVal)

		if !ok1 || !ok2 {
			return 0
		}

		if i >= p {
			return 100
		}

		return 0
	case "match_set":
		profileSet := toStringSet(profileVal)
		if len(profileSet) == 0 {
			return cfg.MissingScore
		}

		itemSet := toStringSet(itemVal)
		if len(itemSet) == 0 {
			return 0
		}

		itemMap := make(map[string]struct{}, len(itemSet))
		for _, s := range itemSet {
			itemMap[s] = struct{}{}
		}

		found := 0

		for _, s := range profileSet {
			if _, ok := itemMap[s]; ok {
				found++
			}
		}

		return float64(found) / float64(len(profileSet)) * 100
	case "intersection":
		pSet := toStringSet(profileVal)
		iSet := toStringSet(itemVal)

		if len(pSet) == 0 && len(iSet) == 0 {
			return 0
		}
		// One scalar contained in the other set: full match if membership
		// is found, otherwise none.
		if len(pSet) == 1 && len(iSet) > 1 {
			if stringSetContains(iSet, pSet[0]) {
				return 100
			}

			return 0
		}

		if len(iSet) == 1 && len(pSet) > 1 {
			if stringSetContains(pSet, iSet[0]) {
				return 100
			}

			return 0
		}

		if len(pSet) == 1 && len(iSet) == 1 {
			if pSet[0] == iSet[0] {
				return 100
			}

			return 0
		}

		union := make(map[string]struct{})
		for _, s := range pSet {
			union[s] = struct{}{}
		}

		for _, s := range iSet {
			union[s] = struct{}{}
		}

		if len(union) == 0 {
			return 0
		}

		pMap := make(map[string]struct{}, len(pSet))
		for _, s := range pSet {
			pMap[s] = struct{}{}
		}

		inter := 0

		for _, s := range iSet {
			if _, ok := pMap[s]; ok {
				inter++
			}
		}

		return float64(inter) / float64(len(union)) * 100
	case "proximity":
		p, ok1 := anyToFloat(profileVal)
		i, ok2 := anyToFloat(itemVal)

		if !ok1 || !ok2 {
			if containsValue(profileVal, itemVal) || containsValue(itemVal, profileVal) {
				return 100
			}

			return 0
		}

		rng := cfg.Max - cfg.Min
		if rng == 0 {
			return 50
		}

		diff := math.Abs(p - i)

		score := 100 - (diff/rng)*100
		if score < 0 {
			score = 0
		}

		return score
	}

	return 0
}

// stringSetContains reports whether a normalized string set contains the given
// normalized string.
func stringSetContains(set []string, s string) bool {
	for _, v := range set {
		if v == s {
			return true
		}
	}

	return false
}

// evaluateEligibilityRules evaluates type-level eligibility rules for an item.
func evaluateEligibilityRules(profile jce.ComparisonProfile, inputs map[string]any, rules []map[string]any, config jce.ScoreFactorConfig) (status string, score float64, explanation string) {
	status = "eligible"
	score = 100

	var failed []string

	penaltyIneligible := 0.0
	if config.PenaltyIneligible != nil {
		penaltyIneligible = *config.PenaltyIneligible
	}

	penaltyLimited := 50.0
	if config.PenaltyLimited != nil {
		penaltyLimited = *config.PenaltyLimited
	}

	for _, rule := range rules {
		onFail := getString(rule, "on_fail")
		if onFail == "" {
			onFail = "ineligible"
		}

		profileVal := getProfileValue(profile, getString(rule, "profile_key"))

		itemVal, ok := inputs[getString(rule, "item_key")]
		if !ok {
			if v, ok := rule["value"]; ok {
				itemVal = v
			}
		}

		passed := evaluateRule(profileVal, itemVal, getString(rule, "operator"))
		if passed {
			continue
		}

		switch onFail {
		case "ineligible":
			status = "ineligible"
			score = penaltyIneligible
		case "limited":
			if status == "eligible" {
				status = "limited"
				score = penaltyLimited
			}
		}

		if msg := getString(rule, "explanation"); msg != "" {
			failed = append(failed, msg)
		} else {
			failed = append(failed, getString(rule, "profile_key")+" rule failed")
		}
	}

	if len(failed) > 0 {
		explanation = strings.Join(failed, "; ")
	} else {
		explanation = "Eligible"
	}

	switch status {
	case "ineligible":
		explanation = "Ineligible: " + explanation
	case "limited":
		explanation = "Limited: " + explanation
	}

	return status, score, explanation
}

// buildWeightMap combines type defaults, profile preferences, goal factor boosts,
// and smart-mode multipliers into a raw weight map for one scoring run.
func buildWeightMap(typeConfig typeConfig, profile jce.ComparisonProfile, smartMode string) map[string]float64 {
	weights := make(map[string]float64, len(typeConfig.DefaultWeights))
	for k, v := range typeConfig.DefaultWeights {
		weights[k] = v
	}

	for k, v := range profile.Preferences {
		weights[k] = v
	}

	for _, goal := range profile.Goals {
		mapped, ok := typeConfig.GoalFactorMap[goal.Key]
		if !ok {
			continue
		}

		for _, factorKey := range mapped {
			weights[factorKey] += goal.Weight
		}
	}

	if sm, ok := typeConfig.SmartModes[smartMode]; ok {
		for k, mult := range sm.FactorMultipliers {
			weights[k] *= mult
		}
	}

	return weights
}

// normalizeWeights returns a copy of weights scaled so the non-negative values
// sum to 1. Weights of 0 are preserved. If the total is 0, the copy is returned
// unchanged so the snapshot still reflects the raw intent.
func normalizeWeights(weights map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(weights))

	var total float64

	for k, v := range weights {
		if v < 0 {
			v = 0
		}

		out[k] = v
		total += v
	}

	if total > 0 {
		for k := range out {
			out[k] = out[k] / total
		}
	}

	return out
}

// generateRecommendations produces recommendations triggered by a factor score
// against the type's recommendation templates. The generating factor's ID is
// attached to each recommendation so the persistence layer can populate
// comparison_recommendations.factor_id.
func generateRecommendations(itemID, factorID, factorKey string, factorScore float64, eligibleStatus string, templates []map[string]any) []jce.ComparisonRecommendation {
	var recs []jce.ComparisonRecommendation

	for _, t := range templates {
		if getString(t, "factor_key") != factorKey {
			continue
		}
		// Backward compatibility: legacy templates stored condition/threshold/statuses
		// inside a nested "conditions" object.
		condition := getString(t, "condition")
		threshold := getFloat(t, "threshold")
		statuses := getStringSlice(t, "statuses")

		if condition == "" {
			if c, ok := t["conditions"].(map[string]any); ok {
				condition = getString(c, "condition")
				threshold = getFloat(c, "threshold")
				statuses = getStringSlice(c, "statuses")
			}
		}

		matched := false

		switch condition {
		case "score_lt":
			matched = factorScore < threshold
		case "score_gt":
			matched = factorScore > threshold
		case "score_lte":
			matched = factorScore <= threshold
		case "score_gte":
			matched = factorScore >= threshold
		case "status_in":
			for _, s := range statuses {
				if s == eligibleStatus {
					matched = true
					break
				}
			}
		}

		if !matched {
			continue
		}

		text := getString(t, "text")
		if text == "" {
			continue
		}

		rec := jce.ComparisonRecommendation{
			ItemID:     &itemID,
			Text:       text,
			ActionType: getStringPtr(t, "action_type"),
			ActionURL:  getStringPtr(t, "action_url"),
			Priority:   safeFloatToInt(getFloat(t, "priority")),
		}
		if factorID != "" {
			rec.FactorID = &factorID
		}

		recs = append(recs, rec)
	}

	return recs
}

// bestFitRecommendationText returns the top-item recommendation text.
// It first looks for a recommendation template keyed to the japa_score factor
// with a "top" condition; if none is found, it falls back to a sensible
// default. i18n is out of scope for the engine and is handled by Task 5/6.
func bestFitRecommendationText(templates []map[string]any) string {
	for _, t := range templates {
		if getString(t, "factor_key") == "japa_score" && getString(t, "condition") == "top" {
			if text := getString(t, "text"); text != "" {
				return text
			}
		}
	}
	// Return a dotted i18n key so the frontend can localize the fallback text.
	return "jce.recommendations.best_fit"
}

// -- helpers -------------------------------------------------------------

func parseTypeConfig(b []byte) (typeConfig, error) {
	var cfg typeConfig
	if len(b) == 0 || string(b) == "{}" || string(b) == "null" {
		return cfg, nil
	}

	var raw rawTypeConfig
	if err := json.Unmarshal(b, &raw); err != nil {
		return cfg, fmt.Errorf("unmarshaling type config: %w", err)
	}

	cfg.AllowedGoals = raw.AllowedGoals
	cfg.DefaultWeights = raw.DefaultWeights

	if cfg.DefaultWeights == nil {
		cfg.DefaultWeights = map[string]float64{}
	}

	cfg.EligibilityRules = raw.EligibilityRules
	if cfg.EligibilityRules == nil {
		cfg.EligibilityRules = []map[string]any{}
	}

	cfg.GoalFactorMap = raw.GoalFactorMap
	if cfg.GoalFactorMap == nil {
		cfg.GoalFactorMap = map[string][]string{}
	}

	cfg.SmartModes = raw.SmartModes
	if cfg.SmartModes == nil {
		cfg.SmartModes = map[string]smartModeConfig{}
	}

	cfg.RecommendationTemplates = raw.RecommendationTemplates
	if cfg.RecommendationTemplates == nil {
		cfg.RecommendationTemplates = []map[string]any{}
	}

	if raw.ScoringEnabled == nil {
		cfg.ScoringEnabled = true
	} else {
		cfg.ScoringEnabled = *raw.ScoringEnabled
	}

	cfg.ProfileDefaults = jce.ComparisonProfile{
		Goals:        raw.ProfileDefaults.Goals,
		Preferences:  raw.ProfileDefaults.Preferences,
		Demographics: raw.ProfileDefaults.Demographics,
	}
	if cfg.ProfileDefaults.Preferences == nil {
		cfg.ProfileDefaults.Preferences = map[string]float64{}
	}

	return cfg, nil
}

func defaultProfileFromTypeConfig(cfg typeConfig) jce.ComparisonProfile {
	p := cfg.ProfileDefaults
	if p.Preferences == nil {
		p.Preferences = map[string]float64{}
	}

	return p
}

func profileFromInput(in jce.ComparisonProfileInput) jce.ComparisonProfile {
	p := jce.ComparisonProfile{
		Title:          in.Title,
		SourceSpaceID:  in.SourceSpaceID,
		TargetSpaceIDs: in.TargetSpaceIDs,
		Goals:          in.Goals,
		Preferences:    in.Preferences,
		Demographics:   in.Demographics,
	}
	if p.Preferences == nil {
		p.Preferences = map[string]float64{}
	}

	return p
}

func scoreFromResolverValue(v any, cfg jce.ScoreFactorConfig) float64 {
	if cfg.Kind == "text" || cfg.Kind == "select" {
		s, ok := v.(string)
		if !ok {
			return cfg.MissingScore
		}

		for _, ideal := range cfg.IdealTexts {
			if strings.EqualFold(s, ideal) {
				return 100
			}
		}

		return 0
	}

	if cfg.Kind == "boolean" || cfg.Kind == "checkbox" {
		b, ok := v.(bool)
		if !ok {
			return cfg.MissingScore
		}

		if b {
			return 100
		}

		return 0
	}

	val, ok := anyToFloat(v)
	if !ok {
		return cfg.MissingScore
	}

	if cfg.Direction == "closer_to_ideal" {
		return closerToIdealScore(val, cfg.Min, cfg.Max, cfg.Ideal)
	}

	return normalizeNumber(val, cfg.Min, cfg.Max, cfg.Ideal, cfg.Direction)
}

func valueToFloat(iv itemValue, cfg jce.ScoreFactorConfig) (float64, bool) {
	if cfg.Kind == "boolean" || cfg.Kind == "checkbox" {
		if iv.valueBool != nil {
			if *iv.valueBool {
				return 100, true
			}

			return 0, true
		}

		if iv.value != nil {
			b, err := strconv.ParseBool(*iv.value)
			if err == nil {
				if b {
					return 100, true
				}

				return 0, true
			}
		}
	}

	if iv.valueRating.Valid {
		f, _ := iv.valueRating.Float64Value()
		return f.Float64, true
	}

	if iv.value != nil {
		f, err := strconv.ParseFloat(*iv.value, 64)
		if err == nil {
			return f, true
		}
	}

	if iv.valueBool != nil {
		if *iv.valueBool {
			return 100, true
		}

		return 0, true
	}

	return 0, false
}

func getProfileValue(profile jce.ComparisonProfile, key string) any {
	if key == "" {
		return nil
	}

	parts := strings.Split(key, ".")
	if len(parts) == 2 && parts[0] == "demographics" {
		switch parts[1] {
		case "age":
			return profile.Demographics.Age
		case "occupation":
			return profile.Demographics.Occupation
		case "family_status":
			return profile.Demographics.FamilyStatus
		case "language_proficiency":
			return profile.Demographics.LanguageProficiency
		case "budget_min":
			return profile.Demographics.BudgetMin
		case "budget_max":
			return profile.Demographics.BudgetMax
		case "budget_currency":
			return profile.Demographics.BudgetCurrency
		}
	}

	switch parts[0] {
	case "target_space_ids":
		return profile.TargetSpaceIDs
	case "source_space_id":
		return profile.SourceSpaceID
	case "preferences":
		if len(parts) == 2 {
			return profile.Preferences[parts[1]]
		}
	case "goals":
		return profile.Goals
	case "demographics":
		return profile.Demographics
	}

	return nil
}

// isMissingValue reports whether a profile or item value is effectively absent
// (nil, empty string, or an empty slice/map).
func isMissingValue(v any) bool {
	if v == nil {
		return true
	}

	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []string:
		return len(t) == 0
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}

	return false
}

// isMissingResolverValue reports whether a resolver metadata value cannot be
// scored because its type or shape does not match the factor's kind.
func isMissingResolverValue(v any, cfg jce.ScoreFactorConfig) bool {
	switch cfg.Kind {
	case "text", "select":
		s, ok := v.(string)
		return !ok || strings.TrimSpace(s) == ""
	case "boolean", "checkbox":
		_, ok := v.(bool)
		return !ok
	case "number", "rating", "numeric":
		_, ok := anyToFloat(v)
		return !ok
	}

	return false
}

func evaluateRule(profileVal, itemVal any, op string) bool {
	switch op {
	case "eq":
		return isEqual(profileVal, itemVal)
	case "gte":
		p, ok1 := anyToFloat(profileVal)
		i, ok2 := anyToFloat(itemVal)

		return ok1 && ok2 && p >= i
	case "lte":
		p, ok1 := anyToFloat(profileVal)
		i, ok2 := anyToFloat(itemVal)

		return ok1 && ok2 && p <= i
	case "gt":
		p, ok1 := anyToFloat(profileVal)
		i, ok2 := anyToFloat(itemVal)

		return ok1 && ok2 && p > i
	case "lt":
		p, ok1 := anyToFloat(profileVal)
		i, ok2 := anyToFloat(itemVal)

		return ok1 && ok2 && p < i
	case "in", "contains":
		return containsValue(profileVal, itemVal) || containsValue(itemVal, profileVal)
	}

	return false
}

func isEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if fmt.Sprint(a) == fmt.Sprint(b) {
		return true
	}

	af, ok1 := anyToFloat(a)
	bf, ok2 := anyToFloat(b)

	return ok1 && ok2 && af == bf
}

func containsValue(container, val any) bool {
	if s, ok := container.(string); ok {
		if t, ok2 := val.(string); ok2 {
			return strings.Contains(strings.ToLower(s), strings.ToLower(t))
		}
	}

	if arr, ok := container.([]string); ok {
		target := fmt.Sprint(val)
		for _, s := range arr {
			if s == target {
				return true
			}
		}
	}

	if arr, ok := container.([]any); ok {
		for _, v := range arr {
			if isEqual(v, val) {
				return true
			}
		}
	}

	return false
}

func anyToFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err == nil {
			return f, true
		}
	case bool:
		if x {
			return 1, true
		}

		return 0, true
	case *float64:
		if x == nil {
			return 0, false
		}

		return *x, true
	case *float32:
		if x == nil {
			return 0, false
		}

		return float64(*x), true
	case *int:
		if x == nil {
			return 0, false
		}

		return float64(*x), true
	case *int64:
		if x == nil {
			return 0, false
		}

		return float64(*x), true
	case *bool:
		if x == nil {
			return 0, false
		}

		if *x {
			return 1, true
		}

		return 0, true
	case pgtype.Numeric:
		if !x.Valid {
			return 0, false
		}

		f, _ := x.Float64Value()

		return f.Float64, true
	}

	return 0, false
}

// safeInt32 converts an int to int32, clamping to [math.MinInt32, math.MaxInt32]
// to prevent integer overflow (gosec G115).
func safeInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}

	if n < math.MinInt32 {
		return math.MinInt32
	}

	return int32(n)
}

// safeFloatToInt clamps a float64 to the range of a signed 64-bit int before
// converting, avoiding implementation-defined overflow on float->int casts.
func safeFloatToInt(v float64) int {
	if v > float64(math.MaxInt64) {
		return math.MaxInt64
	}

	if v < float64(math.MinInt64) {
		return math.MinInt64
	}

	return int(v)
}

func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}

	if s, ok := v.(string); ok {
		return s
	}

	return fmt.Sprint(v)
}

func getFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}

	if f, ok := anyToFloat(v); ok {
		return f
	}

	return 0
}

func getStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}

	if arr, ok := v.([]any); ok {
		out := make([]string, 0, len(arr))

		for _, item := range arr {
			if s, ok := item.(string); ok {
				out = append(out, s)
			} else {
				out = append(out, fmt.Sprint(item))
			}
		}

		return out
	}

	if arr, ok := v.([]string); ok {
		return arr
	}

	return nil
}

func getStringPtr(m map[string]any, key string) *string {
	v, ok := m[key]
	if !ok {
		return nil
	}

	if s, ok := v.(string); ok {
		if s == "" {
			return nil
		}

		return &s
	}

	return nil
}

func parseUUID(s string) (pgtype.UUID, error) {
	return resolver.ParseUUID(s)
}

func stringToUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if s == "" {
		return u
	}

	if err := u.Scan(s); err != nil {
		return u
	}

	return u
}

func numericFromFloat(f float64) pgtype.Numeric {
	n := pgtype.Numeric{}
	_ = n.Scan(strconv.FormatFloat(f, 'f', -1, 64))

	return n
}

func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}

	f, err := n.Float64Value()
	if err != nil {
		return 0
	}

	return f.Float64
}

func toNullableString(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// isMissingExplanation reports whether a persisted explanation indicates that
// the factor's input was unavailable or unusable for scoring.
func isMissingExplanation(exp string) bool {
	lower := strings.ToLower(exp)

	return strings.Contains(lower, "missing") ||
		strings.Contains(lower, "no data") ||
		strings.Contains(lower, "no dimension value") ||
		strings.Contains(lower, "invalid dimension value")
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}

	return *p
}

func uuidPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}

	s := u.String()

	return &s
}

func init() {
	jce.RegisterScoringEngineFactory(func(database *db.DB, compSvc *comparison.Service, r *resolver.Registry, settings *config.Settings, vk valkeycompat.Cmdable) jce.ScoringEngine {
		return NewEngine(database, compSvc, r, settings, vk)
	})
}
