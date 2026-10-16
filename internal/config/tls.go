package config

import "strings"

// NormalizeMySQLTLS maps the human-friendly TLS modes used across .env files,
// the settings UI (which mirrors MySQL CLI / Aiven wording) and DATABASE_URL
// query parameters onto the exact strings accepted by go-sql-driver/mysql.
//
// The driver only understands "", "false", "true", "skip-verify" and
// "preferred" — plus any name previously registered with
// mysql.RegisterTLSConfig. Anything else makes ParseDSN fail with
// "invalid value / unknown config name", so a very natural "required" or
// "disabled" (the values the UI used to save) would break cloud (Aiven)
// connections instead of enabling them.
//
// Mapping:
//
//	preferred / ""        -> preferred   (TLS if the server supports it, else plain)
//	required / skip-verify-> skip-verify (TLS mandatory, certificate not verified)
//	verify-ca/full/true   -> true        (TLS mandatory, certificate verified)
//	disabled / false      -> false       (plain, no TLS)
func NormalizeMySQLTLS(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "preferred", "prefer", "auto":
		return "preferred"
	case "required", "require", "skip-verify", "skip_verify", "skipverify", "insecure":
		return "skip-verify"
	case "verify-ca", "verify_ca", "verify-identity", "verify_identity",
		"verify-full", "verify_full", "verify", "true", "yes", "on":
		return "true"
	case "disabled", "disable", "none", "off", "false", "no":
		return "false"
	default:
		// Unknown mode: fall back to the safe default instead of making the
		// whole application fail to connect because of a typo.
		return "preferred"
	}
}
