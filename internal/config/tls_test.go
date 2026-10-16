package config

import (
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestNormalizeMySQLTLS(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "preferred"},
		{"preferred", "preferred"},
		{"PREFERRED", "preferred"},
		{"  required  ", "skip-verify"},
		{"required", "skip-verify"},
		{"require", "skip-verify"},
		{"REQUIRED", "skip-verify"},
		{"skip-verify", "skip-verify"},
		{"skip_verify", "skip-verify"},
		{"true", "true"},
		{"verify-full", "true"},
		{"verify-ca", "true"},
		{"disabled", "false"},
		{"false", "false"},
		{"none", "false"},
		{"some-typo", "preferred"}, // unknown falls back to safe default
	}
	for _, c := range cases {
		if got := NormalizeMySQLTLS(c.in); got != c.want {
			t.Errorf("NormalizeMySQLTLS(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizedTLSAcceptedByDriver proves the reason the mapping exists: the
// go-sql-driver only accepts a fixed set of tls= values, and the values the
// settings UI used to save ("required"/"disabled") made ParseDSN fail with
// "invalid value / unknown config name".
func TestNormalizedTLSAcceptedByDriver(t *testing.T) {
	// Raw friendly values must still be rejected by the driver, otherwise this
	// guard has become meaningless.
	for _, raw := range []string{"required", "disabled"} {
		if _, err := mysql.ParseDSN("u:p@tcp(127.0.0.1:3306)/db?tls=" + raw); err == nil {
			t.Errorf("driver unexpectedly accepted raw tls=%s", raw)
		}
	}
	// Everything we emit must be accepted.
	for _, in := range []string{"", "preferred", "required", "disabled", "skip-verify", "true", "false", "verify-full"} {
		norm := NormalizeMySQLTLS(in)
		dsn := fmt.Sprintf("u:p@tcp(127.0.0.1:3306)/db?tls=%s", norm)
		if _, err := mysql.ParseDSN(dsn); err != nil {
			t.Errorf("normalized %q -> tls=%s rejected by driver: %v", in, norm, err)
		}
	}
}

// TestLoadParsesDATABASEURLTLS verifies the full Aiven-style connection string
// (ssl-mode=REQUIRED) is understood and turned into a driver-safe TLS mode.
func TestLoadParsesDATABASEURLTLS(t *testing.T) {
	old := App
	t.Cleanup(func() { App = old })

	t.Setenv("DATABASE_URL", "mysql://avnadmin:secret@mysql-x.aivencloud.com:19160/defaultdb?ssl-mode=REQUIRED")
	Load()

	if App.DBHost != "mysql-x.aivencloud.com" || App.DBPort != "19160" || App.DBName != "defaultdb" {
		t.Fatalf("DATABASE_URL not parsed: host=%q port=%q db=%q", App.DBHost, App.DBPort, App.DBName)
	}
	if App.DBUser != "avnadmin" || App.DBPass != "secret" {
		t.Fatalf("credentials not parsed: user=%q pass=%q", App.DBUser, App.DBPass)
	}
	if got := NormalizeMySQLTLS(App.DBTLS); got != "skip-verify" {
		t.Fatalf("ssl-mode=REQUIRED normalized to %q, want skip-verify", got)
	}
}
