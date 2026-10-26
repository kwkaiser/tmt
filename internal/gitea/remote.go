package gitea

import (
	"fmt"
	"net/url"
	"strings"
)

type remote struct {
	Host  string
	Owner string
	Name  string
}

func parseRemote(raw string) (remote, error) {
	raw = strings.TrimSpace(raw)
	path, host, err := splitRemote(raw)
	if err != nil {
		return remote{}, err
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	i := strings.LastIndex(path, "/")
	if host == "" || i <= 0 || i == len(path)-1 {
		return remote{}, fmt.Errorf("parse gitea remote %q: expected host/owner/repo", raw)
	}
	owner := path[:i]
	if j := strings.LastIndex(owner, "/"); j >= 0 {
		owner = owner[j+1:]
	}
	return remote{Host: strings.ToLower(host), Owner: owner, Name: path[i+1:]}, nil
}

func splitRemote(raw string) (string, string, error) {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", fmt.Errorf("parse gitea remote %q: %w", raw, err)
		}
		return u.Path, u.Hostname(), nil
	}
	hostPart, path, ok := strings.Cut(raw, ":")
	if !ok {
		return "", "", fmt.Errorf("parse gitea remote %q: unsupported format", raw)
	}
	if at := strings.LastIndex(hostPart, "@"); at >= 0 {
		hostPart = hostPart[at+1:]
	}
	return path, hostPart, nil
}
