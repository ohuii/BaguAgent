package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvFile(t *testing.T) {
	t.Setenv("BAGU_EXISTING", "from-env")

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := `
# comment
BAGU_AI_PROVIDER=openai-compatible
BAGU_QUOTED="hello world"
export BAGU_EXPORTED='exported value'
BAGU_EXISTING=from-file
`
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := loadDotEnvFile(envPath); err != nil {
		t.Fatalf("loadDotEnvFile() error = %v", err)
	}

	tests := map[string]string{
		"BAGU_AI_PROVIDER": "openai-compatible",
		"BAGU_QUOTED":      "hello world",
		"BAGU_EXPORTED":    "exported value",
		"BAGU_EXISTING":    "from-env",
	}
	for key, want := range tests {
		if got := os.Getenv(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}
