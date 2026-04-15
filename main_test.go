package main

import (
	"testing"
	"time"
)

func TestParseFlags_Defaults(t *testing.T) {
	cfg, err := parseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval != 60*time.Second || cfg.Limit != 50 || !cfg.Notify {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestParseFlags_Validation(t *testing.T) {
	if _, err := parseFlags([]string{"-interval", "1s"}); err == nil {
		t.Fatal("expected error for tiny interval")
	}
	if _, err := parseFlags([]string{"-limit", "0"}); err == nil {
		t.Fatal("expected error for zero limit")
	}
	if _, err := parseFlags([]string{"-limit", "200"}); err == nil {
		t.Fatal("expected error for excessive limit")
	}
}

func TestParseFlags_NoNotify(t *testing.T) {
	cfg, err := parseFlags([]string{"-no-notify"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Notify {
		t.Fatalf("expected Notify=false, got %+v", cfg)
	}
}
