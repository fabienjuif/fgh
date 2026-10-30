// Package auth resolves and stores host credentials without third-party dependencies.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrNoToken = errors.New("no authentication token; run fgh auth login --with-token")

type Credential struct {
	Token  string
	Source string
}
type hostEntry struct {
	Token string `json:"token"`
}

func Token(ctx context.Context, host string) (Credential, error) {
	for _, key := range []string{"FGH_TOKEN", "FORGEJO_TOKEN"} {
		if token := os.Getenv(key); token != "" {
			return Credential{token, key}, nil
		}
	}
	path, err := configPath()
	if err != nil {
		return Credential{}, err
	}
	hosts, err := readHosts(path)
	if err != nil {
		return Credential{}, err
	}
	if entry := hosts[host]; entry.Token != "" {
		return Credential{entry.Token, path}, nil
	}
	if strings.ContainsAny(host, "\n\r") {
		return Credential{}, errors.New("invalid host")
	}
	cmd := exec.CommandContext(ctx, "git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n\n")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	data, err := cmd.Output()
	if err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			if token, ok := strings.CutPrefix(line, "password="); ok && token != "" {
				return Credential{token, "git credential"}, nil
			}
		}
	}
	return Credential{}, ErrNoToken
}

func Save(host, token string) error {
	if host == "" || strings.ContainsAny(host, "\r\n/") || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return errors.New("invalid host or token")
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	hosts, err := readHosts(path)
	if err != nil {
		return err
	}
	hosts[host] = hostEntry{Token: token}
	data, err := json.MarshalIndent(hosts, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".hosts-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err = file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "fgh", "hosts.json"), nil
}

func readHosts(path string) (map[string]hostEntry, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]hostEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("%s must have mode 0600", path)
	}
	hosts := map[string]hostEntry{}
	if err = json.Unmarshal(data, &hosts); err != nil {
		return nil, errors.New("invalid hosts.json")
	}
	if hosts == nil {
		hosts = map[string]hostEntry{}
	}
	return hosts, nil
}
