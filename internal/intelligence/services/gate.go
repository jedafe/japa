package services

type GateDecision int

const (
	GateAutoPublish GateDecision = iota
	GateReviewRequired
	GateReject
)

type GateInput struct {
	ConfidenceOverall float64
	TrustLevel        int16 // sources.TrustLevel is int16; using int16 to avoid cross-package dep
	AutoThreshold     float64
	ReviewThreshold   float64
}

func Classify(in GateInput) GateDecision {
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
