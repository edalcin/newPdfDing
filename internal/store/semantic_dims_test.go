package store

import "testing"

// TestDotProductRejectsDimensionMismatch guards the model-switch path: a
// vector written by another embedding model has another dimensionality, and
// scoring it against the current query vector over the common prefix would
// produce a fake similarity able to clear semanticFloor. Mismatched lengths
// must score 0 so those rows stay out of the results until re-embedded.
func TestDotProductRejectsDimensionMismatch(t *testing.T) {
	same := normalizeL2([]float32{1, 0, 0, 0})
	if got := dotProduct(same, same); got < 0.999 {
		t.Fatalf("identical vectors should score ~1, got %v", got)
	}

	shorter := normalizeL2([]float32{1, 0})
	if got := dotProduct(same, shorter); got != 0 {
		t.Fatalf("mismatched dimensions must score 0, got %v", got)
	}
	if got := dotProduct(shorter, same); got != 0 {
		t.Fatalf("mismatched dimensions must score 0 either way, got %v", got)
	}
}
