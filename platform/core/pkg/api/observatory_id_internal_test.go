package api

import "testing"

func TestValidAtlasOperationID(t *testing.T) {
	for _, id := range []string{"op_123", "Op-abc_9", "a"} {
		if !validAtlasOperationID(id) {
			t.Errorf("valid id %q refused", id)
		}
	}
	for _, id := range []string{"", "../etc/passwd", "a/b", "a?b", "a b", "a.b", "a%2f", string(make([]byte, 129))} {
		if validAtlasOperationID(id) {
			t.Errorf("invalid id %q accepted", id)
		}
	}
}
