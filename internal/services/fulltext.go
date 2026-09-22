package services

import "strings"

// LikeEscape aman untuk input pengguna agar wildcard LIKE (% _ \) tidak
// diperlakukan sebagai pola.
func LikeEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// FullTextClause membangun fragment WHERE pencarian-teks portabel untuk
// MySQL/MariaDB (driver satu-satunya aplikasi ini): substring match per kolom.
// Menggantikan sintaks PostgreSQL to_tsvector/plainto_tsquery yang selama ini
// selalu error 1064 di MariaDB dan membuat halaman hasil-pencarian kosong.
//
// Contoh output: (COALESCE(nama_arsip,'') LIKE ? OR COALESCE(nomor_arsip,'') LIKE ?)
// dengan satu argumen "%q%" per kolom.
func FullTextClause(query string, columns ...string) (string, []interface{}) {
	like := "%" + LikeEscape(query) + "%"
	parts := make([]string, 0, len(columns))
	args := make([]interface{}, 0, len(columns))
	for _, col := range columns {
		parts = append(parts, "COALESCE("+col+",'') LIKE ?")
		args = append(args, like)
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}