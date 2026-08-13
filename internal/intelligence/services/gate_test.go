package services

import "testing"

func TestClassify(t *testing.T) {
	const (
		autoThreshold   = 0.85
		reviewThreshold = 0.50
	)

	tests := []struct {
		name      string
		conf      float64
		trust     int16
		want      GateDecision
	}{
		{"high conf tier1 -> auto", 0.90, 1, GateAutoPublish},
		{"high conf tier3 -> review", 0.90, 3, GateReviewRequired},
		{"mid conf tier1 -> review", 0.60, 1, GateReviewRequired},
		{"low conf tier3 -> reject", 0.40, 3, GateReject},
		{"at auto threshold tier2 -> auto", 0.85, 2, GateAutoPublish},
		{"low conf tier4 -> review (conservative default)", 0.50, 4, GateReviewRequired},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := GateInput{
				ConfidenceOverall: tc.conf,
				TrustLevel:        tc.trust,
				AutoThreshold:     autoThreshold,
				ReviewThreshold:   reviewThreshold,
			}
			got := Classify(in)
			if got != tc.want {
				t.Fatalf("Classify(%+v) = %d, want %d", in, got, tc.want)
			}
		})
	}
}
