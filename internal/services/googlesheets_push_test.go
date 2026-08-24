package services

import (
	"testing"

	"arsippro/internal/models"
)

const fakeSAJSON = `{"type":"service_account","client_email":"sa@test.iam.gserviceaccount.com"}`

func TestSheetsPushEnabled(t *testing.T) {
	cases := map[string]bool{
		"": false, "0": false, "false": false,
		"true": true, "1": true, "TRUE": true, "on": true,
	}
	for in, want := range cases {
		t.Setenv("GOOGLE_SHEETS_SYNC_ENABLED", in)
		if got := SheetsPushEnabled(); got != want {
			t.Errorf("SheetsPushEnabled(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveSheetsConfig(t *testing.T) {
	t.Setenv("GOOGLE_SHEETS_CREDENTIALS_JSON", "")
	t.Setenv("GOOGLE_SHEETS_SPREADSHEET_ID", "")

	m := &models.Integration{
		Type:    "google_sheets",
		BaseURL: "https://docs.google.com/spreadsheets/d/1AbCdEfGhIjKlMnOpQrStUvWxYz1234567890abcd/edit#gid=0",
		ApiKey:  fakeSAJSON,
	}

	if _, _, err := ResolveSheetsConfig(m); err != nil {
		t.Fatalf("resolve from integration fields failed: %v", err)
	}

	// Env overrides integration fields.
	t.Setenv("GOOGLE_SHEETS_CREDENTIALS_JSON", fakeSAJSON)
	t.Setenv("GOOGLE_SHEETS_SPREADSHEET_ID", "ss-from-env")
	_, ssID, err := ResolveSheetsConfig(&models.Integration{Type: "google_sheets"})
	if err != nil || ssID != "ss-from-env" {
		t.Fatalf("env override failed: ssID=%q err=%v", ssID, err)
	}

	// Missing credentials -> clear error.
	t.Setenv("GOOGLE_SHEETS_CREDENTIALS_JSON", "")
	if _, _, err := ResolveSheetsConfig(&models.Integration{Type: "google_sheets"}); err == nil {
		t.Fatal("expected error when no credentials configured")
	}
}
