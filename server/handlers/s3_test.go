package handlers

import (
	"net"
	"strings"
	"testing"
)

func TestAWSEncode(t *testing.T) {
	cases := map[string]string{
		"a b":        "a%20b",
		"a/b":        "a%2Fb",
		"a+b":        "a%2Bb",
		"a-_.~":      "a-_.~",
		"folder/a b": "folder%2Fa%20b",
	}
	for input, want := range cases {
		if got := awsEncode(input); got != want {
			t.Errorf("awsEncode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseS3Endpoint(t *testing.T) {
	parsed, err := parseS3Endpoint("https://host.example/s3/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.origin != "https://host.example" {
		t.Errorf("origin = %q", parsed.origin)
	}
	if parsed.prefix != "/s3" {
		t.Errorf("prefix = %q", parsed.prefix)
	}

	for _, invalid := range []string{"ftp://host", "host.example", "https://user:pass@host"} {
		if _, err := parseS3Endpoint(invalid); err == nil {
			t.Errorf("expected %q to be rejected", invalid)
		}
	}
}

func TestCanonicalS3URI(t *testing.T) {
	parsed, err := parseS3Endpoint("https://host.example/s3/")
	if err != nil {
		t.Fatal(err)
	}
	got := canonicalS3URI(parsed, "music", "folder/a b.mp3")
	want := "/s3/music/folder/a%20b.mp3"
	if got != want {
		t.Errorf("canonicalS3URI = %q, want %q", got, want)
	}
}

func TestGeneratePresignedURL(t *testing.T) {
	url, err := generatePresignedURL(
		"https://host.example/s3/",
		"music",
		"us-east-1",
		"AKIAEXAMPLE",
		"secret",
		"token/with+chars",
		"folder/a b.mp3",
		"GET",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"https://host.example/s3/music/folder/a%20b.mp3?",
		"X-Amz-Security-Token=token%2Fwith%2Bchars",
		"X-Amz-Signature=",
	} {
		if !strings.Contains(url, want) {
			t.Errorf("URL %q does not contain %q", url, want)
		}
	}
}

func TestBlockedIP(t *testing.T) {
	blocked := []string{"169.254.169.254", "fe80::1", "0.0.0.0", "fd00:ec2::254"}
	for _, raw := range blocked {
		if err := blockedIP(net.ParseIP(raw), false); err == nil {
			t.Errorf("expected %s to be blocked", raw)
		}
	}

	if err := blockedIP(net.ParseIP("127.0.0.1"), false); err != nil {
		t.Errorf("loopback should be allowed when blockPrivate is false: %v", err)
	}
	if err := blockedIP(net.ParseIP("127.0.0.1"), true); err == nil {
		t.Error("loopback should be blocked when blockPrivate is true")
	}
	if err := blockedIP(net.ParseIP("93.184.216.34"), true); err != nil {
		t.Errorf("public address should be allowed: %v", err)
	}
}
