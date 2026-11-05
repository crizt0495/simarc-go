package config

import (
	"os"
	"path/filepath"
	"strings"
)

// envPath adalah lokasi file .env yang dipakai (default: file .env di
// direktori kerja). Aplikasi desktop memanggil SetEnvPath agar selalu
// membaca .env di samping executable (mandiri & offline).
var envPath = ".env"

// SetEnvPath mengubah lokasi file .env (mis. di samping executable).
func SetEnvPath(p string) { envPath = p }

// EnvFilePath mengembalikan path file .env aktif.
func EnvFilePath() string { return envPath }

// LoadFrom memuat kunci-kunci .env pada `p` ke process environment lalu
// menjalankan Load(). Nilai environment yang SUDAH ada di proses tidak
// ditimpa (env runtime > file), sama seperti konvensi godotenv.
func LoadFrom(p string) (*Config, error) {
	SetEnvPath(p)
	b, err := os.ReadFile(p)
	if err == nil {
		parseEnv(string(b), func(k, v string) {
			// Prioritas: env runtime proses > isi file .env.
			if _, exists := os.LookupEnv(k); !exists {
				_ = os.Setenv(k, v)
			}
		})
	}
	return Load()
}

// LoadFromDir mencari .env di `dir` lalu memuatnya.
func LoadFromDir(dir string) (*Config, error) { return LoadFrom(filepath.Join(dir, ".env")) }

// parseEnv membaca format .env baris-per-baris:
//   - baris kosong / komentar (#, ;) dilewati
//   - `KEY=value` atau `KEY="value"` atau `KEY='value'`
//   - kata kunci per baris; `export KEY=value` juga diterima
func parseEnv(content string, set func(k, v string)) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = strings.ReplaceAll(val[1:len(val)-1], `\"`, `"`)
		}
		if key == "" {
			continue
		}
		set(key, val)
	}
}

// UpdateEnv menulis nilai ke file .env tanpa mengurangi kepemilikan pengguna:
// komentar dipertahankan, kunci yang ada diperbarui DENGAN posisinya,
// kunci baru ditambahkan di akhir. Nilai juga direfleksikan ke process
// environment (dipakai proses turunan seperti mysqldump backup).
func UpdateEnv(values map[string]string) error {
	content := ""
	if b, err := os.ReadFile(envPath); err == nil {
		content = string(b)
	}

	updated := make(map[string]bool, len(values))
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, ";") {
			trimmed = strings.TrimPrefix(trimmed, "export ")
			if eq := strings.Index(trimmed, "="); eq > 0 {
				key := strings.TrimSpace(trimmed[:eq])
				if v, ok := values[key]; ok {
					line = key + "=" + quoteEnvValue(v)
					updated[key] = true
				}
			}
		}
		lines = append(lines, line)
	}
	for k, v := range values {
		if !updated[k] {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			lines = append(lines, k+"="+quoteEnvValue(v))
		}
		os.Setenv(k, v)
	}
	out := strings.Join(lines, "\n")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(envPath, []byte(out), 0o644)
}

// quoteEnvValue mengutip nilai .env bila mengandung karakter berbahaya
// (spasi, komentar, pemisah, atau tanda kutip).
func quoteEnvValue(v string) string {
	if v == "" {
		return ""
	}
	if strings.ContainsAny(v, " #;'\"") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}
