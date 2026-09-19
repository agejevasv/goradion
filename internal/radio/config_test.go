package radio

import (
	"os"
	"strings"
	"testing"
)

func TestConfigDefaultsAndSave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
	c.Theme, c.Volume = "dracula", 35
	if err := c.save("theme"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(configFile())
	if got := string(data); got != "# mine\ntheme: dracula # dark\nfuture: 1\n" {
		t.Fatalf("saved config = %q", got)
	}

	// A broken file is left alone.
	os.WriteFile(configFile(), []byte("theme: [\n"), 0644)
	c = loadConfig()
	if c.Theme != defaultTheme || c.save("theme") == nil {
		t.Fatalf("broken config: theme %q, saved anyway", c.Theme)
	}
}

func TestConfigRemote(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := loadConfig()
	if !c.Autoplay || c.Remote.Autostart || c.Remote.Port != DefaultRemotePort || c.Remote.Key != nil {
		t.Fatalf("defaults %+v", c)
	}

	for text, want := range map[string]string{
		"remote:\n  key: 123456\n": "123456",
		"remote:\n  key: \"\"\n":   "",
	} {
		os.WriteFile(configFile(), []byte(text), 0644)
		if c := loadConfig(); c.Remote.Key == nil || *c.Remote.Key != want {
			t.Fatalf("%q: key %v", text, c.Remote.Key)
		}
	}

	// Saving leaves the hand-edited keys alone.
	text := "autoplay: false # quiet\nremote:\n  # mine\n  autostart: true\n  port: 8123\n"
	os.WriteFile(configFile(), []byte(text), 0644)
	c = loadConfig()
	if c.Autoplay || !c.Remote.Autostart || c.Remote.Port != 8123 {
		t.Fatalf("parsed %+v", c)
	}
	c.Theme = "nord"
	if err := c.save("theme"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(configFile())
	if !strings.HasPrefix(string(data), text) {
		t.Fatalf("saved config:\n%s", data)
	}
}

func TestConfigSaveKeepsLayout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	os.MkdirAll(configDir(), 0755)
	in := "# top\n\ntheme: nord   # dark\n\nvolume: 10\r\ntag:\nstation: 'x' # s\n\nremote:\n  port: 1\n"
	os.WriteFile(configFile(), []byte(in), 0644)
	c := loadConfig()
	c.Theme, c.Volume, c.Tag, c.Station = "dracula", 35, "Jazz", "https://a/b?c=d#e"
	if err := c.save("theme", "volume", "tag", "station"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(configFile())
	want := "# top\n\ntheme: dracula   # dark\n\nvolume: 35\r\ntag: Jazz\nstation: https://a/b?c=d#e # s\n\nremote:\n  port: 1\n"
	if string(data) != want {
		t.Fatalf("saved %q", data)
	}
	if got := loadConfig().savedConfig; got != c.savedConfig {
		t.Fatalf("reloaded %+v", got)
	}
}

// The user may edit the file while goradion runs.
func TestConfigSaveKeepsEditsMadeMeanwhile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c := loadConfig()
	edited := "theme: nord\nremote:\n  autostart: true\n  key: mine\n"
	os.WriteFile(configFile(), []byte(edited), 0644)

	c.Volume = 35
	if err := c.save("volume"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(configFile())
	if got := string(data); got != edited+"volume: 35\n" {
		t.Fatalf("saved:\n%s", got)
	}

	// Broken mid-edit: left alone. Fixed later: saved.
	os.WriteFile(configFile(), []byte("theme: [\n"), 0644)
	if c.save("volume") == nil {
		t.Fatal("saved over a broken file")
	}
	os.WriteFile(configFile(), []byte("theme: nord\n"), 0644)
	if err := c.save("volume"); err != nil {
		t.Fatal(err)
	}
}
