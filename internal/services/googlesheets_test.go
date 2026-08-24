package services

import (
	"strings"
	"testing"
)

func TestExtractSheetID(t *testing.T) {
	cases := map[string]string{
		"https://docs.google.com/spreadsheets/d/1YGa-PE8rr41mf5aaOhM24N3BtJib1kLDXecf8JyYiu8/edit?gid=0#gid=0": "1YGa-PE8rr41mf5aaOhM24N3BtJib1kLDXecf8JyYiu8",
		"1YGa-PE8rr41mf5aaOhM24N3BtJib1kLDXecf8JyYiu8":                                                        "1YGa-PE8rr41mf5aaOhM24N3BtJib1kLDXecf8JyYiu8",
		"":                                                                                                    "",
	}
	for in, want := range cases {
		if got := ExtractSheetID(in); got != want {
			t.Errorf("ExtractSheetID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractGid(t *testing.T) {
	if got := ExtractGid("https://docs.google.com/spreadsheets/d/X/edit?gid=123#gid=123"); got != 123 {
		t.Errorf("gid = %d, want 123", got)
	}
	if got := ExtractGid("no-gid"); got != 0 {
		t.Errorf("gid = %d, want 0", got)
	}
}

func TestParseCSVMapping(t *testing.T) {
	csvData := "Nomor Arsip,Nama Arsip,Uraian,Jenis Arsip,Kode Klasifikasi,Unit Kerja,Tanggal,Jumlah,Satuan\n001,Laporan Keuangan 2026,Dana BOS Semester 1,SPJ,300,Bagian Keuangan,15/08/2026,3,Berkas\n"
	headers, rows, err := ParseCSV([]byte(csvData))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	nama := colValue(headers, rows[0], "nama_arsip", "nama")
	if nama != "Laporan Keuangan 2026" {
		t.Errorf("nama = %q", nama)
	}
	kk := colValue(headers, rows[0], "kode_klasifikasi", "kode")
	if kk != "300" {
		t.Errorf("kode = %q", kk)
	}
	d := parseFlexibleDate(colValue(headers, rows[0], "tanggal"))
	if d == nil || d.Format("2006-01-02") != "2026-08-15" {
		t.Errorf("tanggal = %v", d)
	}
	if !strings.Contains(normalizeHeader("Kode Klasifikasi"), "kode_klasifikasi") {
		t.Error("normalizeHeader failed")
	}
}
