package handlers

import "strings"

// arsipSearchLike builds a portable substring-search predicate over the given
// arsip columns.
//
// It replaces the PostgreSQL-only to_tsvector()/plainto_tsquery() full-text
// predicate, which MySQL/MariaDB do not support (Error 1305: FUNCTION
// to_tsvector does not exist). The behaviour matches the LIKE-based search used
// elsewhere in the app (see ArsipHandler.Index), so results stay consistent
// across the whole application.
func arsipSearchLike(q string, columns ...string) (string, []interface{}) {
	like := "%" + strings.TrimSpace(q) + "%"
	conds := make([]string, 0, len(columns))
	args := make([]interface{}, 0, len(columns))
	for _, c := range columns {
		conds = append(conds, "COALESCE("+c+",'') LIKE ?")
		args = append(args, like)
	}
	return "(" + strings.Join(conds, " OR ") + ")", args
}
