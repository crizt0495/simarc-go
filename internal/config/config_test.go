package config

import (
	"os"
	"strings"
	"testing"
)

func TestUpdateEnv(t *testing.T) {
	// Work in a temp dir so the real project .env is never touched
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)

	initial := "# SIMARC config\nAPP_NAME=\"SIMARC Arsippro\"\nDB_HOST=127.0.0.1\nDB_PASSWORD=secret\nSESSION_KEY=abc\n"
	if err := os.WriteFile(EnvFilePath(), []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	if err := UpdateEnv(map[string]string{
		"DB_HOST":     "10.0.0.5",
		"DB_DATABASE": "new_db",
		"DB_PASSWORD": "",
	}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_DATABASE")
		os.Unsetenv("DB_PASSWORD")
	}()

	got, err := os.ReadFile(EnvFilePath())
	if err != nil {
		t.Fatal(err)
	}
	content := string(got)

	// Comments preserved
	if !strings.Contains(content, "# SIMARC config") {
		t.Errorf("comment lost:\n%s", content)
	}
	// Existing key updated in place
	if !strings.Contains(content, "DB_HOST=10.0.0.5") {
		t.Errorf("DB_HOST not updated:\n%s", content)
	}
	// New key appended
	if !strings.Contains(content, "DB_DATABASE=new_db") {
		t.Errorf("DB_DATABASE not appended:\n%s", content)
	}
	// Unrelated keys untouched
	if !strings.Contains(content, "APP_NAME=\"SIMARC Arsippro\"") {
		t.Errorf("APP_NAME changed:\n%s", content)
	}
	if !strings.Contains(content, "SESSION_KEY=abc") {
		t.Errorf("SESSION_KEY changed:\n%s", content)
	}

	// Process environment reflected (used by mysqldump backup)
	if os.Getenv("DB_HOST") != "10.0.0.5" {
		t.Errorf("process env DB_HOST = %q, want 10.0.0.5", os.Getenv("DB_HOST"))
	}
	if os.Getenv("DB_PASSWORD") != "" {
		t.Errorf("process env DB_PASSWORD should be empty, got %q", os.Getenv("DB_PASSWORD"))
	}
}

func TestQuoteEnvValue(t *testing.T) {
	cases := []struct{ in, want string }{
		{"simple", "simple"},
		{"", ""},
		{"with space", "\"with space\""},
		{"has#hash", "\"has#hash\""},
		{"has;semi", "\"has;semi\""},
		{"quote\"inside", "\"quote\\\"inside\""},
	}
	for _, c := range cases {
		if got := quoteEnvValue(c.in); got != c.want {
			t.Errorf("quoteEnvValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
