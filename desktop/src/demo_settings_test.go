package main

import (
    "path/filepath"
    "testing"
)

func TestDemoDefaultSettings(t *testing.T) {
    s := defaultSettings()
    if s.Locale != LocaleZH || s.Port != 18088 {
        t.Fatalf("unexpected demo settings: %+v", s)
    }
}

func TestDemoHomeIsolatedFromStandardOctop(t *testing.T) {
    t.Setenv("OCTOP_HOME", "")
    if got := filepath.Base(octopHome()); got != ".octop-demo" {
        t.Fatalf("unexpected demo home: %q", got)
    }
}

func TestDemoHomeExplicitOverride(t *testing.T) {
    dir := t.TempDir()
    t.Setenv("OCTOP_HOME", dir)
    if octopHome() != dir {
        t.Fatalf("OCTOP_HOME override not respected")
    }
}
