package config

import "testing"

func TestOptionalEnvReturnsDefault(t *testing.T) {
	t.Setenv("OPTIONAL_TEST_ENV", "")
	got := OptionalEnv("OPTIONAL_TEST_ENV", "default")
	if got != "default" {
		t.Fatalf("expected default value, got %q", got)
	}
}

func TestOptionalEnvReturnsValue(t *testing.T) {
	t.Setenv("OPTIONAL_TEST_ENV", "value")
	got := OptionalEnv("OPTIONAL_TEST_ENV", "default")
	if got != "value" {
		t.Fatalf("expected env value, got %q", got)
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Fatalf("expected to find value in slice")
	}
	if contains([]string{"a", "b", "c"}, "z") {
		t.Fatalf("did not expect to find value in slice")
	}
}

func TestValidateEnvDefault(t *testing.T) {
	t.Setenv("CROWDSEC_BOUNCER_BAN_RESPONSE_CODE", "403")
	ValidateEnv()
}
