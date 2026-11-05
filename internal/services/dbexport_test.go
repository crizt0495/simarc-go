package services

import "testing"

func TestIsBinaryColumn(t *testing.T) {
	binary := []string{"longblob", "blob", "mediumblob", "tinyblob", "binary(16)", "varbinary(255)", "bit(1)"}
	for _, c := range binary {
		if !isBinaryColumn(c) {
			t.Errorf("%q should be binary", c)
		}
	}
	text := []string{"varchar(255)", "char(36)", "longtext", "text", "int(11)", "datetime(3)"}
	for _, c := range text {
		if isBinaryColumn(c) {
			t.Errorf("%q should not be binary", c)
		}
	}
}

func TestFormatSQLValueBinaryIsHex(t *testing.T) {
	// A LONGBLOB must round-trip byte-for-byte via a 0x hex literal.
	got := formatSQLValue([]byte{0x00, 0x01, 0x1a, 0xff}, "longblob")
	if got != "0x00011aff" {
		t.Fatalf("binary not hex-encoded: %q", got)
	}
	if got := formatSQLValue("hi", "longblob"); got != "0x6869" {
		t.Fatalf("string binary not hex-encoded: %q", got)
	}
	if got := formatSQLValue(nil, "longblob"); got != "NULL" {
		t.Fatalf("nil binary should be NULL, got %q", got)
	}
}

func TestFormatSQLValueText(t *testing.T) {
	if got := formatSQLValue("it's", "varchar(255)"); got != `'it\'s'` {
		t.Fatalf("quote escaping wrong: %q", got)
	}
	// NUL and Ctrl-Z must be emitted as escapes, never silently dropped.
	if got := formatSQLValue("a\x00b\x1a", "text"); got != `'a\0b\Z'` {
		t.Fatalf("control chars escaped wrong: %q", got)
	}
	if got := formatSQLValue([]byte("plain"), "varchar(255)"); got != "'plain'" {
		t.Fatalf("[]byte text wrong: %q", got)
	}
}

func TestFormatSQLValueNumbers(t *testing.T) {
	if got := formatSQLValue(int64(5), "int(11)"); got != "5" {
		t.Fatalf("int64 wrong: %q", got)
	}
	if got := formatSQLValue(true, "tinyint(1)"); got != "1" {
		t.Fatalf("bool wrong: %q", got)
	}
}
