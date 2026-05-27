package engine

import "testing"

func TestRender(t *testing.T) {
	ctx := renderContext{
		Vars:    map[string]string{"test_cmd": "go test ./..."},
		Scalars: map[string]string{"task": "build it", "prev": "the plan", "iteration": "2"},
	}
	cases := map[string]string{
		"do {{task}}":                 "do build it",
		"run {{vars.test_cmd}}":       "run go test ./...",
		"prev={{prev}} iter={{iteration}}": "prev=the plan iter=2",
		"unknown {{nope}} here":       "unknown  here",
		"spaces {{ task }}":           "spaces build it",
	}
	for in, want := range cases {
		if got := render(in, ctx); got != want {
			t.Errorf("render(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContainsMarker(t *testing.T) {
	if !containsMarker("blah __NEXT:test__ blah", "__NEXT:test__") {
		t.Error("should find marker in surrounding text")
	}
	if containsMarker("no marker here", "__DONE__") {
		t.Error("should not find absent marker")
	}
	if containsMarker("anything", "  ") {
		t.Error("blank marker should never match")
	}
}
