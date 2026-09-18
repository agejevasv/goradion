package radio

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type config struct {
	Theme string `yaml:"theme"`

	path     string
	doc      yaml.Node // the file as read, so that saving keeps comments and unknown keys
	readOnly bool      // the file is broken and left for the user to fix
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
`, commentLines(strings.Join(themeNames(), ", "), 76), defaultTheme, defaultTheme)
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
		log.Printf("config: %v", err)
		c.readOnly = true
		c.parse([]byte(defaultConfigText()))
	}
	return c
}

func (c *config) parse(data []byte) error {
	c.Theme, c.doc = defaultTheme, yaml.Node{}
	if err := yaml.Unmarshal(data, &c.doc); err != nil {
		return err
	}
	if c.doc.Kind == 0 { // empty file
		return nil
	}
	return c.doc.Decode(c)
}

// save updates the known keys in the file as read.
func (c *config) save() error {
	if c.readOnly {
		return fmt.Errorf("%s could not be read, not overwriting it", c.path)
	}
	var values yaml.Node
	if err := values.Encode(c); err != nil {
		return err
	}
	if c.doc.Kind == 0 {
		c.doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := c.doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: top level is not a mapping", c.path)
	}
	for i := 0; i+1 < len(values.Content); i += 2 {
		setMapValue(root, values.Content[i], values.Content[i+1])
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&c.doc); err != nil {
		return err
	}
	return writeFile(c.path, buf.Bytes())
}

func setMapValue(m, key, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key.Value {
			old := m.Content[i+1]
			value.HeadComment, value.LineComment, value.FootComment = old.HeadComment, old.LineComment, old.FootComment
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, key, value)
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
