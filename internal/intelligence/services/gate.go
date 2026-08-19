package services

// GateDecision is the outcome of the intelligence publication gate.
type GateDecision int

const (
	// GateAutoPublish means the extraction can be published without human review.
	GateAutoPublish GateDecision = iota
	// GateReviewRequired means a human must review before publishing.
	GateReviewRequired
	// GateReject means the extraction does not meet the confidence/trust bar.
	GateReject
)

// GateInput contains the signals used to decide whether an intel record may
// be auto-published, sent to review, or rejected.
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
	"visa":               true, // immigration
	"remittance":         true, // financial
	"banking":            true, // financial
	"grants":             true, // grants
	"health_insurance":   true, // health
	"admission":          true, // immigration/admissions (ADR-088)
	"credential_pathway": true, // professional licensing — high-stakes immigration/career path (§94)
}

// isHighStakes returns true if the entity type is in the high-stakes set
// (immigration, financial, grants, health, professional licensing). The check
// is unconditional and cannot be bypassed via config (ADR-088).
func isHighStakes(entityType string) bool {
	return highStakesEntityTypes[entityType]
}

// Classify returns the publication decision for an intel record given its
// entity type, confidence, source trust level, and configured thresholds.
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
