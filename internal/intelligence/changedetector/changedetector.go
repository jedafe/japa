// Package changedetector compares content hashes to determine if a source
// has changed since the last crawl.
package changedetector

type Decision int

const (
	DecisionChanged    Decision = iota // proceed to extraction
	DecisionUnchanged                  // skip extraction, update last_crawled_at only
	DecisionFirstCrawl                 // no prior hash, proceed to extraction
)

// Compare returns the change decision given the new hash and the prior hash
// (empty string if no prior).
func Compare(newHash, priorHash string) Decision {
	if priorHash == "" {
		return DecisionFirstCrawl
	}
	if newHash == priorHash {
		return DecisionUnchanged
	}
	return DecisionChanged
}

// String returns a human-readable label for the decision, used in logs.
func (d Decision) String() string {
	switch d {
	case DecisionChanged:
		return "changed"
	case DecisionUnchanged:
		return "unchanged"
	case DecisionFirstCrawl:
		return "first_crawl"
	default:
		return "unknown"
	}
}
