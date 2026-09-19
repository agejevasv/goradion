package radio

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/agejevasv/goradion/internal/logging"
	"github.com/agejevasv/goradion/internal/mpv"
	"gopkg.in/yaml.v3"
)

const DefaultRemotePort = 7373

// savedConfig is what save writes back. The rest is only read, so that
// saving never rewrites what the user edits by hand.
type savedConfig struct {
	Theme   string `yaml:"theme"`
	Volume  int    `yaml:"volume"`
	Tag     string `yaml:"tag"`
	Station string `yaml:"station"` // URL
}

type remoteConfig struct {
	Autostart bool    `yaml:"autostart"`
	Port      int     `yaml:"port"`
	Key       *string `yaml:"key"` // nil: a random code per run
}

type config struct {
	savedConfig `yaml:",inline"`
	Autoplay    bool         `yaml:"autoplay"`
	Remote      remoteConfig `yaml:"remote"`

	path string
}

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "goradion")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "goradion")
	default:
		return filepath.Join(home, ".config", "goradion")
	}
}

func configFile() string {
	return filepath.Join(configDir(), "config.yaml")
}

func defaultConfigText() string {
	return fmt.Sprintf(`# goradion settings. goradion updates this file itself, e.g. when you pick a
# theme with Ctrl+T, and keeps your comments.

# Colour theme, one of:
%s# "%s" uses the terminal's own colours and background.
theme: %s

# Play the last station at launch.
autoplay: true

# Remembered from the last run.
volume: %d
tag: ""
station: ""

# Phone remote, Ctrl+P. The key is a fixed access code instead of a random one
# per run; "" for none.
# remote:
#   autostart: true
#   port: %d
#   key: abc123
`, commentLines(strings.Join(themeNames(), ", "), 76), defaultTheme, defaultTheme, mpv.DefaultVolume, DefaultRemotePort)
}

// commentLines wraps text at spaces into indented YAML comment lines.
func commentLines(text string, width int) string {
	var sb strings.Builder
	line := ""
	for _, word := range strings.Fields(text) {
		if line != "" && len(line)+1+len(word) > width {
			sb.WriteString("#   " + line + "\n")
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	sb.WriteString("#   " + line + "\n")
	return sb.String()
}

// loadConfig reads config.yaml, creating it when it is missing. A file that
// can't be read or parsed is logged and the defaults are used.
func loadConfig() *config {
	c := &config{path: configFile()}
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		data = []byte(defaultConfigText())
		err = writeFile(c.path, data)
	}
	if err == nil {
		err = c.parse(data)
	}
	if err != nil {
		logging.Printf("config: %v", err)
		c.parse([]byte(defaultConfigText()))
	}
	return c
}

func (c *config) parse(data []byte) error {
	c.savedConfig = savedConfig{Theme: defaultTheme, Volume: mpv.DefaultVolume}
	c.Autoplay = true
	c.Remote = remoteConfig{Port: DefaultRemotePort}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.Kind == 0 { // empty file
		return nil
	}
	return doc.Decode(c)
}

// save rewrites only the named keys in the file as it is now, which the user
// may have edited since goradion started: re-encoding the document would also
// drop their blank lines. Missing keys are appended.
func (c *config) save(names ...string) error {
	data, err := os.ReadFile(c.path)
	if os.IsNotExist(err) {
		data, err = []byte(defaultConfigText()), nil
	}
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %w, not overwriting it", c.path, err)
	}
	var root *yaml.Node
	if doc.Kind != 0 {
		if root = doc.Content[0]; root.Kind != yaml.MappingNode {
			return fmt.Errorf("%s: top level is not a mapping", c.path)
		}
	}
	var values yaml.Node
	if err := values.Encode(c.savedConfig); err != nil {
		return err
	}

	lines := strings.SplitAfter(string(data), "\n")
	var appended strings.Builder
	for i := 0; i+1 < len(values.Content); i += 2 {
		name := values.Content[i].Value
		if !slices.Contains(names, name) {
			continue
		}
		value, err := yaml.Marshal(values.Content[i+1])
		if err != nil {
			return err
		}
		text := strings.TrimSuffix(string(value), "\n")
		key, old := mapEntry(root, name)
		if key == nil {
			appended.WriteString(name + ": " + text + "\n")
			continue
		}
		if old.Kind != yaml.ScalarNode || (old.Line != key.Line && old.Tag != "!!null") {
			return fmt.Errorf("%s: %s is not a one-line value", c.path, name)
		}
		lines[key.Line-1] = replaceValue(lines[key.Line-1], key.Column-1+len(name), text, old.LineComment+key.LineComment)
	}

	out := strings.Join(lines, "")
	if appended.Len() > 0 {
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += appended.String()
	}
	return writeFile(c.path, []byte(out))
}

func mapEntry(m *yaml.Node, name string) (key, value *yaml.Node) {
	if m == nil {
		return nil, nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == name {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// replaceValue puts text after the colon that follows the key ending at
// keyEnd, keeping a trailing comment and the line ending.
func replaceValue(line string, keyEnd int, text, comment string) string {
	colon := keyEnd + strings.Index(line[keyEnd:], ":")
	body := strings.TrimRight(line, "\r\n")
	ending := line[len(body):]
	tail := ""
	if comment != "" {
		if i := strings.LastIndex(body, comment); i > colon {
			tail = body[len(strings.TrimRight(body[:i], " \t")):]
			body = body[:i]
		}
	}
	return line[:colon+1] + " " + text + tail + ending
}

// writeFile replaces the file through a rename, so that a crash never leaves
// it half written.
func writeFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
