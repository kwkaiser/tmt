package cli

import "os"

var (
	bundledPath string
	bundledEnv  map[string]string
)

func applyBundled(path string, env map[string]string, getenv func(string) string, lookup func(string) (string, bool), setenv func(string, string) error) error {
	if path != "" {
		if cur := getenv("PATH"); cur != "" {
			path += string(os.PathListSeparator) + cur
		}
		if err := setenv("PATH", path); err != nil {
			return err
		}
	}
	for k, v := range env {
		if _, ok := lookup(k); ok {
			continue
		}
		if err := setenv(k, v); err != nil {
			return err
		}
	}
	return nil
}
