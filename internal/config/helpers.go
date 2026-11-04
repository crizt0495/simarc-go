package config

import "os"

func getEnv(key string, fb ...string) string {
	v := os.Getenv(key)
	if v != "" {
		return v
	}
	if len(fb) > 0 && fb[0] != "" {
		return fb[0]
	}
	return ""
}
