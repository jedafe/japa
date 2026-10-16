// Package ai — tools_calculators.go implements the deterministic calculator
// tools for the AI Agent Executor (Section 103 tool catalog).
//
// Tools implemented here:
//
//   - crs.calculate — Express Entry Comprehensive Ranking System score
//     (Section 107). Calls the deterministic scoring engine; when called by
//     an authenticated agent user the result is also persisted to score
//     history, matching POST /crs/calculate.
//   - pof.calculate — Proof of Funds requirement for a destination + visa
//     type + family size (Section 108).
//   - fx.convert    — currency conversion against the mid-market rate store
//     (Section 112).
//
// These are the "ideal deterministic agent tools": pure functions over
// reference data — no LLM judgement involved — so the executor hands the
// result straight back to the model for use in a larger answer.
//
// Ref: japaconnect-engineering/sections/107-crs-calculator.md
// Ref: japaconnect-engineering/sections/108-pof-calculator.md
// Ref: japaconnect-engineering/sections/112-fx-converter.md
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// fx.convert (Section 112)
// ---------------------------------------------------------------------------

// FXConvertHandler returns a ToolHandler that converts an amount between two
// ISO 4217 currencies using the stored mid-market rate.
func FXConvertHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.FXConverter == nil {
			return errorToolResult("fx_unavailable", "fx service is not configured"), nil
		}

		var a struct {
			Amount float64 `json:"amount"`
			From   string  `json:"from"`
			To     string  `json:"to"`
		}

		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.Amount <= 0 {
			return errorToolResult("invalid_amount", "amount must be greater than zero"), nil
		}

		if a.From == "" || a.To == "" {
			return errorToolResult("missing_currency", "from and to ISO currency codes are required"), nil
		}

		result, err := deps.FXConverter.Convert(ctx, a.Amount, a.From, a.To)
		if err != nil {
			return errorToolResult("fx_convert_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}

// ---------------------------------------------------------------------------
// crs.calculate (Section 107)
// ---------------------------------------------------------------------------

// CRSCalculateHandler returns a ToolHandler that runs the Express Entry CRS
// scoring engine. The argument JSON matches the POST /crs/calculate request
// contract; the wiring-layer adapter unmarshals it into
// crscalculator.CalculateRequest so the two callers share validation.
func CRSCalculateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.CRSCalculator == nil {
			return errorToolResult("crs_unavailable", "crs calculator is not configured"), nil
		}

		if len(args) == 0 {
			return errorToolResult("missing_arguments", "a CRS profile object is required"), nil
		}

		result, err := deps.CRSCalculator.Calculate(ctx, &userID, args)
		if err != nil {
			return errorToolResult("crs_calculate_failed", err.Error()), nil
		}

		return successToolResult(result), nil
	}
}

// ---------------------------------------------------------------------------
// pof.calculate (Section 108)
// ---------------------------------------------------------------------------

// POFCalculateHandler returns a ToolHandler that computes the proof-of-funds
// requirement for a destination space, visa type, and household size.
func POFCalculateHandler(deps *ToolDeps) ToolHandler {
	return func(ctx context.Context, args json.RawMessage, userID pgtype.UUID) (*ToolResult, error) {
		if deps == nil || deps.POFCalculator == nil {
			return errorToolResult("pof_unavailable", "pof calculator is not configured"), nil
		}

		var a POFCalculateInput
		if err := json.Unmarshal(args, &a); err != nil {
			return errorToolResult("invalid_arguments", fmt.Sprintf("invalid arguments: %v", err)), nil
		}

		if a.DestSpaceID == "" {
			return errorToolResult("missing_dest_space_id", "dest_space_id (UUID or geo code) is required"), nil
		}

		if a.VisaType == "" {
			return errorToolResult("missing_visa_type", "visa_type is required"), nil
		}

		results, err := deps.POFCalculator.Calculate(ctx, a)
		if err != nil {
			return errorToolResult("pof_calculate_failed", err.Error()), nil
		}

		return successToolResult(map[string]any{"requirements": results, "count": len(results)}), nil
	}
}
