package handlers

import "testing"

func TestLooksLikeAlternateVersion(t *testing.T) {
	alternates := []string{
		"part of “DJ Khaled: 2022 MegaMix” DJ‐mix",
		"live",
		"radio edit",
		"instrumental",
		"sped up",
	}
	for _, value := range alternates {
		if !looksLikeAlternateVersion(value) {
			t.Errorf("expected %q to be treated as an alternate version", value)
		}
	}

	for _, value := range []string{"", "album version", "explicit"} {
		if looksLikeAlternateVersion(value) {
			t.Errorf("expected %q not to be treated as an alternate version", value)
		}
	}
}

func TestDurationDelta(t *testing.T) {
	if _, ok := durationDelta(207000, 207); !ok {
		t.Error("exact duration should match")
	}
	if _, ok := durationDelta(133000, 207); ok {
		t.Error("DJ-mix length should not match a full-length recording")
	}
	if _, ok := durationDelta(0, 207); !ok {
		t.Error("missing length should be allowed")
	}
	if _, ok := durationDelta(207000, 0); !ok {
		t.Error("missing fingerprint duration should be allowed")
	}
}

func TestLuceneEscape(t *testing.T) {
	if got := luceneEscape(`a "quoted" \ title`); got != `a \"quoted\" \\ title` {
		t.Errorf("luceneEscape = %q", got)
	}
}
