package handler

import (
	"net/http"
	"testing"

	"github.com/ShotaKitazawa/glidepath/internal/database/sqlcgen"
)

// TestRegister_NoRoutePatternConflicts guards against a class of bug that
// only surfaces at server startup: net/http.ServeMux panics if two
// registered patterns are ambiguous for some path (e.g. "/family/{id}/x"
// vs "/family/y/{id}"). Unit tests on individual handlers never exercise
// mux.HandleFunc, so this is the only test that would have caught it.
func TestRegister_NoRoutePatternConflicts(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Register panicked (likely a route pattern conflict): %v", r)
		}
	}()

	mux := http.NewServeMux()
	Register(mux, (*sqlcgen.Queries)(nil))
}
