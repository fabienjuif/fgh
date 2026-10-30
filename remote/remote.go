// Package remote resolves Forgejo repositories from git remotes.
package remote

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

type Repository struct{ Host, Owner, Repo string }

func (r Repository) APIBase() string { return "https://" + r.Host }

func Parse(raw string) (Repository, error) {
	var r Repository
	var path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return r, errors.New("invalid remote URL")
		}
		switch u.Scheme {
		case "ssh":
			r.Host = u.Hostname()
		case "https":
			r.Host = u.Host
		default:
			return r, errors.New("unsupported remote scheme")
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return r, errors.New("invalid remote URL")
		}
		path = strings.TrimPrefix(u.Path, "/")
	} else {
		host, p, ok := strings.Cut(raw, ":")
		if !ok {
			return r, errors.New("invalid remote")
		}
		if _, h, found := strings.Cut(host, "@"); found {
			host = h
		}
		r.Host, path = host, p
	}
	parts := strings.Split(strings.TrimSuffix(path, ".git"), "/")
	if r.Host == "" || len(parts) != 2 || !validName(parts[0]) || !validName(parts[1]) {
		return Repository{}, errors.New("remote must identify host/owner/repo")
	}
	r.Owner, r.Repo = parts[0], parts[1]
	return r, nil
}

func Origin(ctx context.Context, dir string) (Repository, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = dir
	data, err := cmd.Output()
	if err != nil {
		return Repository{}, errors.New("cannot read origin remote")
	}
	return Parse(strings.TrimSpace(string(data)))
}

func Resolve(ctx context.Context, dir, override string) (Repository, error) {
	r, err := Origin(ctx, dir)
	if host := os.Getenv("FGH_HOST"); err != nil && host != "" {
		r.Host = host
		err = nil
	}
	if override != "" {
		parts := strings.Split(override, "/")
		if len(parts) != 2 || !validName(parts[0]) || !validName(parts[1]) {
			return Repository{}, errors.New("repository must be owner/repo")
		}
		r.Owner, r.Repo = parts[0], parts[1]
	}
	if err != nil {
		return Repository{}, err
	}
	if r.Host == "" {
		return Repository{}, errors.New("no host: set FGH_HOST or configure origin")
	}
	if r.Owner == "" || r.Repo == "" {
		return Repository{}, errors.New("no repository: use -R owner/repo or configure origin")
	}
	return r, nil
}

// Host allows authentication outside a repository; origin takes precedence.
func Host(ctx context.Context, dir string) (string, error) {
	r, err := Origin(ctx, dir)
	if err == nil {
		return r.Host, nil
	}
	if host := os.Getenv("FGH_HOST"); host != "" {
		return host, nil
	}
	return "", err
}

func validName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, " \\:\n\r\t?#")
}
