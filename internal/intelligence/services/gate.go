package services

type GateDecision int

const (
	GateAutoPublish GateDecision = iota
	GateReviewRequired
	GateReject
)

type GateInput struct {
	EntityType        string
	ConfidenceOverall float64
	TrustLevel        int16 // sources.TrustLevel is int16; using int16 to avoid cross-package dep
	AutoThreshold     float64
	ReviewThreshold   float64
}

// highStakesEntityTypes are the entity types that require unconditional HITL
// review before publication, regardless of confidence or trust level. Per
// ADR-088, the high-stakes gate (immigration/financial/grants/health) cannot
// be bypassed via config.
var highStakesEntityTypes = map[string]bool{
	"visa":             true, // immigration
	"remittance":       true, // financial
	"banking":          true, // financial
	"grants":           true, // grants
	"health_insurance": true, // health
}

// isHighStakes returns true if the entity type is in the high-stakes set
// (immigration, financial, grants, health). The check is unconditional and
// cannot be bypassed via config (ADR-088).
func isHighStakes(entityType string) bool {
	return highStakesEntityTypes[entityType]
}

func Classify(in GateInput) GateDecision {
	// High-stakes gate (ADR-088): immigration, financial, grants, and health
	// content ALWAYS require human review, regardless of confidence or trust
	// level. This gate is unconditional and cannot be bypassed via config.
	if isHighStakes(in.EntityType) {
		return GateReviewRequired
	}
	if in.ConfidenceOverall >= in.AutoThreshold && in.TrustLevel <= 2 {
		return GateAutoPublish
	}
	if in.ConfidenceOverall >= in.ReviewThreshold {
		return GateReviewRequired
	}
	if in.TrustLevel >= 3 {
		return GateReject
	}
	return GateReviewRequired // conservative default
}
