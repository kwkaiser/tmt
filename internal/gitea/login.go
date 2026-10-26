package gitea

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	envServerURL = "GITEA_SERVER_URL"
	envToken     = "GITEA_TOKEN"
)

type login struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Token    string `yaml:"token"`
	SSHHost  string `yaml:"ssh_host"`
	Insecure bool   `yaml:"insecure"`
}

type teaConfig struct {
	Logins []login `yaml:"logins"`
}

func envLogin(getenv func(string) string, host string) (login, bool) {
	token := getenv(envToken)
	if token == "" {
		return login{}, false
	}
	server := getenv(envServerURL)
	if server == "" {
		return login{URL: "https://" + host, Token: token}, true
	}
	if urlHost(server) != host {
		return login{}, false
	}
	return login{URL: server, Token: token}, true
}

func teaConfigPaths(getenv func(string) string) []string {
	var paths []string
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "tea", "config.yml"))
	}
	if home := getenv("HOME"); home != "" {
		paths = append(paths,
			filepath.Join(home, ".config", "tea", "config.yml"),
			filepath.Join(home, "Library", "Application Support", "tea", "config.yml"),
		)
	}
	return paths
}

func readTeaConfig(paths []string) (teaConfig, string, error) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return teaConfig{}, p, err
		}
		var cfg teaConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return teaConfig{}, p, fmt.Errorf("parse %s: %w", p, err)
		}
		return cfg, p, nil
	}
	return teaConfig{}, "", fmt.Errorf("no tea config found in %s", strings.Join(paths, ", "))
}

func (c teaConfig) loginFor(host string) (login, bool) {
	for _, l := range c.Logins {
		if urlHost(l.URL) == host {
			return l, true
		}
	}
	for _, l := range c.Logins {
		if l.SSHHost != "" && hostOnly(l.SSHHost) == host {
			return l, true
		}
	}
	return login{}, false
}

func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func hostOnly(h string) string {
	if u, err := url.Parse("ssh://" + h); err == nil && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	return strings.ToLower(h)
}
