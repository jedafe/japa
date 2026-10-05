package spaces

// §80 Wiki-Only Space Types — pure unit coverage for the forced permission
// axes and the cross-package sentinel marker. IsWikiOnlyType and the
// CanPostInSpace branch need a store; they're exercised by the
// handler/integration path.

import (
	"errors"
	"testing"
)

func TestWikiOnlyPermAxes(t *testing.T) {
	a := WikiOnlyPermAxes()
	if a.Visibility != VisibilityPublic ||
		a.Posting != PermAdmins || a.Replying != PermAdmins || a.Commenting != PermAdmins ||
		a.Joining != JoiningInvite {
		t.Fatalf("unexpected wiki-only axes: %+v", a)
	}
}

func TestErrWikiOnlySpace_Marker(t *testing.T) {
	var m interface{ WikiOnlySpace() bool }
	if !errors.As(ErrWikiOnlySpace, &m) {
		t.Fatal("ErrWikiOnlySpace must satisfy the WikiOnlySpace marker interface")
	}
}
