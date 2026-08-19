package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestHandleListProposedSources_NilSources(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/intel/sources/proposed", nil)
	h.HandleListProposedSources(rr, r)
	assertIntelUnavailable(t, rr)
}

func TestHandleApproveProposedSource_NilSources(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	rr := httptest.NewRecorder()
	r := requestWithURLParam("POST", "/api/v1/intel/sources/00000000-0000-0000-0000-000000000001/approve", "id", "00000000-0000-0000-0000-000000000001", "")
	h.HandleApproveProposedSource(rr, r)
	assertIntelUnavailable(t, rr)
}
