package devshell

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

const (
	FlakeDir  = ".tmt-flake"
	flakeFile = "flake.nix"
	lockFile  = "flake.lock"
	envrcFile = ".envrc"
)

var systems = []string{"aarch64-darwin", "aarch64-linux", "x86_64-darwin", "x86_64-linux"}

type Writer struct{}

var _ tmt.DevShell = Writer{}

func New() Writer {
	return Writer{}
}

type repo struct {
	Name     string
	URL      string
	Follows  string
	modified int64
	hasPkgs  bool
}

func (Writer) Write(ctx context.Context, t tmt.Task) error {
	repos, envs, err := scan(t.Dir)
	if err != nil {
		return err
	}
	primary := -1
	for i, r := range repos {
		if r.hasPkgs && (primary < 0 || r.modified > repos[primary].modified) {
			primary = i
		}
	}
	flakeDir := filepath.Join(t.Dir, FlakeDir)
	if primary < 0 {
		repos = nil
		if err := os.RemoveAll(flakeDir); err != nil {
			return err
		}
	} else {
		for i := range repos {
			if i != primary && repos[i].hasPkgs {
				repos[i].Follows = repos[primary].Name + "/nixpkgs"
			}
		}
		flake, err := renderFlake(repos, repos[primary].Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(flakeDir, 0o755); err != nil {
			return err
		}
		if err := writeIfChanged(filepath.Join(flakeDir, flakeFile), flake); err != nil {
			return err
		}
	}
	envrc := filepath.Join(t.Dir, envrcFile)
	if len(repos) == 0 && len(envs) == 0 {
		if err := os.Remove(envrc); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return writeIfChanged(envrc, renderEnvrc(repos, envs))
}

func scan(dir string) ([]repo, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	var repos []repo
	var envs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		envs = append(envs, dotenvs(e.Name(), filepath.Join(path, envrcFile))...)
		if !definesShell(filepath.Join(path, flakeFile)) {
			continue
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, nil, err
		}
		r := repo{
			Name: e.Name(),
			URL:  (&url.URL{Scheme: "git+file", Path: real}).String(),
		}
		r.modified, r.hasPkgs = nixpkgsAge(filepath.Join(path, lockFile))
		repos = append(repos, r)
	}
	return repos, envs, nil
}

func nixpkgsAge(path string) (int64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var lock struct {
		Root  string `json:"root"`
		Nodes map[string]struct {
			Inputs map[string]json.RawMessage `json:"inputs"`
			Locked struct {
				LastModified int64 `json:"lastModified"`
			} `json:"locked"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return 0, false
	}
	root, ok := lock.Nodes[lock.Root]
	if !ok {
		return 0, false
	}
	var node string
	if err := json.Unmarshal(root.Inputs["nixpkgs"], &node); err != nil {
		return 0, false
	}
	n, ok := lock.Nodes[node]
	if !ok {
		return 0, false
	}
	return n.Locked.LastModified, true
}

var dotenvLine = regexp.MustCompile(`^\s*(dotenv_if_exists|dotenv)(?:\s+(\S+))?\s*$`)

func dotenvs(name, path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := dotenvLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		file := m[2]
		if file == "" {
			file = ".env"
		}
		if filepath.IsAbs(file) {
			lines = append(lines, "dotenv_if_exists "+shellQuote(file))
			continue
		}
		lines = append(lines, "dotenv_if_exists "+shellQuote(filepath.Join(name, file)))
	}
	return lines
}

var flakeTmpl = template.Must(template.New("flake").Funcs(template.FuncMap{"nix": nixString}).Parse(`{
  inputs = {
{{- range .Repos}}
    {{nix .Name}} = {
      url = {{nix .URL}};
{{- if .Follows}}
      inputs.nixpkgs.follows = {{nix .Follows}};
{{- end}}
    };
{{- end}}
  };

  outputs = { self, ... }@inputs:
    let
      nixpkgs = inputs.{{nix .Primary}}.inputs.nixpkgs;
      lib = nixpkgs.lib;
      repos = lib.attrValues (removeAttrs inputs [ "self" ]);
      shells = system: lib.concatMap (r: lib.optional (r ? devShells.${system}.default) r.devShells.${system}.default) repos;
    in
    {
      devShells = lib.genAttrs [ {{range .Systems}}{{nix .}} {{end}}] (system: {
        default = nixpkgs.legacyPackages.${system}.mkShell { inputsFrom = shells system; };
      });
    };
}
`))

func renderFlake(repos []repo, primary string) ([]byte, error) {
	var buf bytes.Buffer
	err := flakeTmpl.Execute(&buf, struct {
		Repos   []repo
		Primary string
		Systems []string
	}{repos, primary, systems})
	return buf.Bytes(), err
}

func renderEnvrc(repos []repo, envs []string) []byte {
	var b strings.Builder
	for _, r := range repos {
		q := shellQuote(r.Name)
		b.WriteString("watch_file " + q + "/" + flakeFile + " " + q + "/" + lockFile + "\n")
	}
	if len(repos) > 0 {
		b.WriteString("use flake ./" + FlakeDir + " --recreate-lock-file\n")
	}
	for _, l := range envs {
		b.WriteString(l + "\n")
	}
	return []byte(b.String())
}

func nixString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `${`, `\${`)
	return `"` + r.Replace(s) + `"`
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func definesShell(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && bytes.Contains(data, []byte("devShell"))
}

func writeIfChanged(path string, data []byte) error {
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}
