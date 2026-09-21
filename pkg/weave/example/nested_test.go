package example

import "testing"

func TestNewServiceMaxNestingDepthDefault(t *testing.T) {
	svc := NewService(newFakeStore(), fakeViews{})
	if svc.maxDepth != defaultMaxNestingDepth {
		t.Fatalf("maxDepth = %d, want default %d", svc.maxDepth, defaultMaxNestingDepth)
	}
}

func TestNewServiceMaxNestingDepthOption(t *testing.T) {
	svc := NewService(newFakeStore(), fakeViews{}, WithMaxNestingDepth(3))
	if svc.maxDepth != 3 {
		t.Fatalf("maxDepth = %d, want 3", svc.maxDepth)
	}
}

func TestNewServiceMaxNestingDepthOptionZeroKeepsDefault(t *testing.T) {
	svc := NewService(newFakeStore(), fakeViews{}, WithMaxNestingDepth(0))
	if svc.maxDepth != defaultMaxNestingDepth {
		t.Fatalf("maxDepth = %d, want default %d", svc.maxDepth, defaultMaxNestingDepth)
	}
}
