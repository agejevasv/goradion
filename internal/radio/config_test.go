package radio

import (
	"os"
	"strings"
	"testing"
)

func TestConfigDefaultsAndSave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	InitLog(false)

	c := loadConfig()
	if c.Theme != defaultTheme {
		t.Fatalf("default theme = %q", c.Theme)
	}
	data, err := os.ReadFile(configFile())
	if err != nil || !strings.Contains(string(data), "theme: "+defaultTheme) {
		t.Fatalf("default config not written: %q, %v", data, err)
	}

	// Saving keeps comments and keys goradion does not know.
	os.WriteFile(configFile(), []byte("# mine\ntheme: nord # dark\nfuture: 1\n"), 0644)
	c = loadConfig()
	if c.Theme != "nord" {
		t.Fatalf("theme = %q", c.Theme)
	}
	c.Theme = "dracula"
	if err := c.save(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(configFile())
	if got := string(data); got != "# mine\ntheme: dracula # dark\nfuture: 1\n" {
		t.Fatalf("saved config = %q", got)
	}

	// A broken file is left alone.
	os.WriteFile(configFile(), []byte("theme: [\n"), 0644)
	c = loadConfig()
	if c.Theme != defaultTheme || c.save() == nil {
		t.Fatalf("broken config: theme %q, saved anyway", c.Theme)
	}
}
