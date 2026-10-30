package wallet

import (
	"testing"
)

func TestConstants(t *testing.T) {
	t.Parallel()
	// Test ledger types
	if LedgerTypePoints != "points" {
		t.Errorf("Expected LedgerTypePoints to be 'points', got '%s'", LedgerTypePoints)
	}

	if LedgerTypeCredits != "credits" {
		t.Errorf("Expected LedgerTypeCredits to be 'credits', got '%s'", LedgerTypeCredits)
	}

	// Test operations
	if OperationEarn != "earn" {
		t.Errorf("Expected OperationEarn to be 'earn', got '%s'", OperationEarn)
	}

	if OperationSpend != "spend" {
		t.Errorf("Expected OperationSpend to be 'spend', got '%s'", OperationSpend)
	}

	if OperationPurchase != "purchase" {
		t.Errorf("Expected OperationPurchase to be 'purchase', got '%s'", OperationPurchase)
	}

	if OperationConvert != "convert" {
		t.Errorf("Expected OperationConvert to be 'convert', got '%s'", OperationConvert)
	}

	if OperationRefund != "refund" {
		t.Errorf("Expected OperationRefund to be 'refund', got '%s'", OperationRefund)
	}

	if OperationClawback != "clawback" {
		t.Errorf("Expected OperationClawback to be 'clawback', got '%s'", OperationClawback)
	}

	if OperationAdminAdjust != "admin_adjust" {
		t.Errorf("Expected OperationAdminAdjust to be 'admin_adjust', got '%s'", OperationAdminAdjust)
	}
}

func TestActivityKeys(t *testing.T) {
	t.Parallel()

	if ActivityKeyContentCreated != "content.created" {
		t.Errorf("Expected ActivityKeyContentCreated to be 'content.created', got '%s'", ActivityKeyContentCreated)
	}

	if ActivityKeyCommentCreated != "comment.created" {
		t.Errorf("Expected ActivityKeyCommentCreated to be 'comment.created', got '%s'", ActivityKeyCommentCreated)
	}

	if ActivityKeyReactionAdded != "reaction.added" {
		t.Errorf("Expected ActivityKeyReactionAdded to be 'reaction.added', got '%s'", ActivityKeyReactionAdded)
	}
}

func TestPaginationDefaults(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("Expected DefaultPageSize to be 20, got %d", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("Expected MaxPageSize to be 100, got %d", MaxPageSize)
	}
}

func TestStrPtr(t *testing.T) {
	t.Parallel()

	s := "test"

	p := strPtr(s)
	if p == nil {
		t.Fatal("Expected strPtr to return non-nil pointer")
	}

	if *p != s {
		t.Errorf("Expected *strPtr to be '%s', got '%s'", s, *p)
	}
}

func TestLedgerTypeValidation(t *testing.T) {
	t.Parallel()

	validTypes := []string{LedgerTypePoints, LedgerTypeCredits}
	for _, lt := range validTypes {
		if lt != "points" && lt != "credits" {
			t.Errorf("Invalid ledger type: %s", lt)
		}
	}
}

func TestOperationValidation(t *testing.T) {
	t.Parallel()

	validOps := []string{
		OperationEarn, OperationSpend, OperationPurchase,
		OperationConvert, OperationRefund, OperationClawback, OperationAdminAdjust,
	}
	for _, op := range validOps {
		if op == "" {
			t.Error("Empty operation found")
		}
	}
}
