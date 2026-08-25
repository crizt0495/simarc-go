package services

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"time"

	"arsippro/internal/database"

	"gorm.io/gorm"
)

// ExportDatabaseAsSQL generates a mysqldump-compatible .sql file entirely
// through GORM, so it works on Vercel serverless where the mysqldump binary
// is not available. It handles CREATE TABLE + INSERT statements for every
// table in the current database.
func ExportDatabaseAsSQL() ([]byte, error) {
	db := database.DB
	var buf bytes.Buffer

	buf.WriteString("-- SIMARC Database Export\n")
	buf.WriteString(fmt.Sprintf("-- Generated: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	buf.WriteString("-- Method: GORM-based export (Vercel-compatible)\n")
	buf.WriteString("SET FOREIGN_KEY_CHECKS=0;\n\n")

	// Get all table names
	tables, err := db.Migrator().GetTables()
	if err != nil {
		return nil, fmt.Errorf("gagal mendapatkan daftar tabel: %w", err)
	}

	// Skip internal/system tables
	skipTables := map[string]bool{
		"sqlite_sequence":       true,
		"gorp_migrations":      true,
		"permission_role":      true,
		"goose_db_version":     true,
	}

	for _, tableName := range tables {
		if skipTables[tableName] {
			continue
		}

		// Export table structure
		if err := exportTableStructure(&buf, db, tableName); err != nil {
			log.Printf("[WARN] Gagal export struktur tabel %s: %v", tableName, err)
			continue
		}

		// Export table data
		if err := exportTableData(&buf, db, tableName); err != nil {
			log.Printf("[WARN] Gagal export data tabel %s: %v", tableName, err)
		}
	}

	buf.WriteString("\nSET FOREIGN_KEY_CHECKS=1;\n")

	return buf.Bytes(), nil
}

// exportTableStructure writes CREATE TABLE statement for the given table.
func exportTableStructure(buf *bytes.Buffer, db *gorm.DB, tableName string) error {
	// Use SHOW CREATE TABLE to get the exact structure
	type CreateTableResult struct {
		Table       string `gorm:"column:Table"`
		CreateTable string `gorm:"column:Create Table"`
	}
	var result CreateTableResult
	if err := db.Raw("SHOW CREATE TABLE `"+tableName+"`").Scan(&result).Error; err != nil {
		return err
	}

	createStmt := result.CreateTable
	if createStmt == "" {
		createStmt = result.Table // fallback
	}

	buf.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS `%s`;\n", tableName))
	buf.WriteString(createStmt)
	buf.WriteString(";\n\n")

	return nil
}

// exportTableData writes INSERT statements for all rows in the given table.
func exportTableData(buf *bytes.Buffer, db *gorm.DB, tableName string) error {
	// Get column names
	type ColumnInfo struct {
		FieldName string `gorm:"column:Field"`
		FieldType string `gorm:"column:Type"`
	}
	var columns []ColumnInfo
	if err := db.Raw("SHOW COLUMNS FROM `"+tableName+"`").Scan(&columns).Error; err != nil {
		return err
	}
	if len(columns) == 0 {
		return nil
	}

	fieldNames := make([]string, len(columns))
	for i, col := range columns {
		fieldNames[i] = "`" + col.FieldName + "`"
	}
	columnList := strings.Join(fieldNames, ", ")

	// Fetch all rows using raw SQL to avoid GORM type issues
	var rows []map[string]interface{}
	if err := db.Table(tableName).Find(&rows).Error; err != nil {
		return err
	}

	if len(rows) == 0 {
		return nil
	}

	// Batch INSERT (100 rows per statement for efficiency)
	batchSize := 100
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]

		buf.WriteString(fmt.Sprintf("INSERT INTO `%s` (%s) VALUES\n", tableName, columnList))

		for j, row := range batch {
			vals := make([]string, len(columns))
			for k, col := range columns {
				vals[k] = formatSQLValue(row[col.FieldName])
			}
			comma := ","
			if j == len(batch)-1 {
				comma = ";"
			}
			buf.WriteString(fmt.Sprintf("(%s)%s\n", strings.Join(vals, ", "), comma))
		}
		buf.WriteString("\n")
	}

	return nil
}

// formatSQLValue converts a Go value to a SQL literal.
func formatSQLValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}

	switch val := v.(type) {
	case string:
		escaped := strings.ReplaceAll(val, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "'", "\\'")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		escaped = strings.ReplaceAll(escaped, "\r", "\\r")
		escaped = strings.ReplaceAll(escaped, "\x00", "")
		return "'" + escaped + "'"
	case []byte:
		return formatSQLValue(string(val))
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32:
		return fmt.Sprintf("%f", val)
	case float64:
		return fmt.Sprintf("%f", val)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case time.Time:
		if val.IsZero() {
			return "NULL"
		}
		return "'" + val.Format("2006-01-02 15:04:05") + "'"
	default:
		s := fmt.Sprintf("%v", val)
		escaped := strings.ReplaceAll(s, "'", "\\'")
		return "'" + escaped + "'"
	}
}
