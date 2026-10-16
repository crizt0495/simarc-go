package syncdb

import (
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestDSNNormalizesTLS guards the hub connection: values saved by the settings
// UI ("required", "disabled") must be normalized before they reach
// go-sql-driver, otherwise the hub connection fails with
// "invalid value / unknown config name".
func TestDSNNormalizesTLS(t *testing.T) {
	for _, in := range []string{"", "preferred", "required", "disabled", "skip-verify", "true", "false"} {
		got := dsn("hub.local", "3306", "simarc", "user", "pass", in)
		if _, err := mysql.ParseDSN(got); err != nil {
			t.Errorf("dsn(tls=%q) rejected by driver: %v", in, err)
		}
	}
	// Sanity: "required" becomes skip-verify, not the raw word.
	if got := dsn("hub.local", "3306", "simarc", "u", "p", "required"); !strings.Contains(got, "tls=skip-verify") {
		t.Errorf("tls=required not normalized in %q", got)
	}
}
