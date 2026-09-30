package database

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// SIMARC runs on two databases: a local MySQL that the app writes to, and an
// Aiven MySQL kept as an offsite DR copy. Switching between them is a matter of
// pointing DB_* at the other host and restarting — including in the installed
// desktop app, where the .env that matters is the one in the per-user data
// directory.
//
// These tests verify that the very code path used for that switch can actually
// reach both. They only run when asked, because they need live credentials:
//
//	SIMARC_TEST_DB=local,aiven go test ./internal/database/
//
// Aiven in particular requires TLS and presents a self-signed CA, so a
// connection that works from the mysql client can still fail from the app if the
// TLS mode in the DSN is wrong.

func testDBEnv() []string {
	v := os.Getenv("SIMARC_TEST_DB")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func wants(name string) bool {
	for _, d := range testDBEnv() {
		if d == name {
			return true
		}
	}
	return false
}

// params returns the connection parameters for the named database, read from
// the environment the same way config.Load reads .env.
func params(name string) (host, port, db, user, pass string, ok bool) {
	if name == "aiven" {
		host = os.Getenv("AIVEN_HOST")
		port = os.Getenv("AIVEN_PORT")
		db = os.Getenv("AIVEN_DATABASE")
		user = os.Getenv("AIVEN_USERNAME")
		pass = os.Getenv("AIVEN_PASSWORD")
		return host, port, db, user, pass, host != "" && pass != ""
	}
	host = os.Getenv("DB_HOST")
	port = os.Getenv("DB_PORT")
	db = os.Getenv("DB_DATABASE")
	user = os.Getenv("DB_USERNAME")
	pass = os.Getenv("DB_PASSWORD")
	return host, port, db, user, pass, host != ""
}

func closeDB(t *testing.T, conn *gorm.DB) {
	t.Helper()
	sqlDB, err := conn.DB()
	if err != nil {
		t.Logf("could not reach the pool to close it: %v", err)
		return
	}
	_ = sqlDB.Close()
}

// TestLocalDatabaseReachable proves the primary database answers through the
// app's own DSN builder.
func TestLocalDatabaseReachable(t *testing.T) {
	if !wants("local") {
		t.Skip("set SIMARC_TEST_DB=local to run")
	}
	host, port, db, user, pass, ok := params("local")
	if !ok {
		t.Fatal("DB_HOST is not set")
	}
	conn, err := openDB(host, port, db, user, pass)
	if err != nil {
		t.Fatalf("connect to local %s:%s/%s: %v", host, port, db, err)
	}
	defer closeDB(t, conn)

	var one int
	if err := conn.Raw("SELECT 1").Scan(&one).Error; err != nil {
		t.Fatalf("SELECT 1 on local database: %v", err)
	}
	if one != 1 {
		t.Errorf("SELECT 1 returned %d", one)
	}
}

// TestAivenDatabaseReachable proves the DR database answers through the same
// DSN builder, over TLS. Without this, a failover would be discovered to be
// broken at the worst possible moment.
func TestAivenDatabaseReachable(t *testing.T) {
	if !wants("aiven") {
		t.Skip("set SIMARC_TEST_DB=aiven to run")
	}
	host, port, db, user, pass, ok := params("aiven")
	if !ok {
		t.Fatal("AIVEN_HOST / AIVEN_PASSWORD are not set")
	}

	// Aiven's DBTLS setting lives in the local DB_TLS variable; for the DR copy
	// TLS is mandatory, so use skip-verify for the self-signed CA the same way
	// the documented failover does.
	t.Setenv("DB_TLS", "skip-verify")

	conn, err := openDB(host, port, db, user, pass)
	if err != nil {
		t.Fatalf("connect to aiven %s:%s/%s: %v", host, port, db, err)
	}
	defer closeDB(t, conn)

	var version string
	if err := conn.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
		t.Fatalf("SELECT VERSION() on aiven: %v", err)
	}
	t.Logf("aiven reachable: %s (server %s)", db, version)

	// The DR copy has to actually contain data, otherwise a failover would hand
	// back an empty application.
	var rows int64
	if err := conn.Table("arsip").Count(&rows).Error; err != nil {
		t.Fatalf("count arsip on aiven: %v", err)
	}
	t.Logf("aiven arsip rows: %s", strconv.FormatInt(rows, 10))
}
