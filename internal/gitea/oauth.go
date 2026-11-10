package gitea

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-signet/sdk-go/credstore"
)

const (
	authMethodOAuth  = "oauth"
	teaService       = "tea-cli"
	refreshThreshold = 5 * time.Minute
)

type tokenLoader func(name string) (credstore.Token, error)

type tokenRefresher func(ctx context.Context, name string) error

func teaTokenLoader(configDir string) tokenLoader {
	store := credstore.DefaultTokenSecureStore(teaService, filepath.Join(configDir, "credentials.json"))
	return store.Load
}

func teaRefresh(ctx context.Context, name string) error {
	out, err := exec.CommandContext(ctx, "tea", "api", "--login", name, "/version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tea refresh %s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func fresh(tok credstore.Token) bool {
	if tok.AccessToken == "" {
		return false
	}
	return tok.ExpiresAt.IsZero() || time.Now().Add(refreshThreshold).Before(tok.ExpiresAt)
}

func oauthToken(ctx context.Context, name string, load tokenLoader, refresh tokenRefresher) (string, error) {
	if tok, err := load(name); err == nil && fresh(tok) {
		return tok.AccessToken, nil
	}
	if err := refresh(ctx, name); err != nil {
		return "", err
	}
	tok, err := load(name)
	if err != nil {
		return "", fmt.Errorf("load tea oauth token for %s: %w", name, err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("tea oauth token for %s is empty; run `tea login`", name)
	}
	return tok.AccessToken, nil
}
