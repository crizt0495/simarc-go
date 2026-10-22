package syncdb

import "testing"

// TestIsTrulyGeneratedColumn mengunci perilaku pemilahan kolom MySQL:
// hanya VIRTUAL/STORED GENERATED yang dilewati. "DEFAULT_GENERATED" TIDAK boleh
// dilewati — kolom `id char(36) DEFAULT (uuid())` di MySQL 8/Aiven memakainya,
// dan bila ikut terbuang maka PRIMARY KEY-nya hilang sehingga tabel (mis.
// `arsip`) dilewati sepenuhnya saat sinkron. Lihat regresi "data klien 0".
func TestIsTrulyGeneratedColumn(t *testing.T) {
	cases := []struct {
		extra string
		want  bool
	}{
		{"virtual generated", true},
		{"VIRTUAL GENERATED", true},
		{"stored generated", true},
		{"STORED GENERATED", true},
		// Nilai nyata dari Aiven MySQL untuk kolom PK ber-DEFAULT:
		{"DEFAULT_GENERATED", false},
		{"default_generated", false},
		{"", false},
		{"auto_increment", false},
		{"on update current_timestamp", false},
	}
	for _, c := range cases {
		if got := isTrulyGeneratedColumn(c.extra); got != c.want {
			t.Errorf("isTrulyGeneratedColumn(%q) = %v, ingin %v", c.extra, got, c.want)
		}
	}
}
