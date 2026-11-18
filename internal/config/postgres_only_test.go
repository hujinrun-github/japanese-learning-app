package config

import "testing"

func TestLoadRejectsUnsupportedDatabase(t *testing.T) {
	t.Setenv("RELATIONAL_STORE", "sqlite")
	if _, err := Load(""); err == nil {
		t.Fatal("SQLite must not remain an available runtime")
	}
}
