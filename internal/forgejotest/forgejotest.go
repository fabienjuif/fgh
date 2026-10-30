// Package forgejotest starts a throwaway Forgejo in podman and seeds it, for the tests behind the
// integration build tag. It never talks to anything but that container.
package forgejotest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// AGitTitle is the title passed with -o title= on the AGit push; it identifies that pull request.
const AGitTitle = "agit pull request"

// Image is pinned to the major version the plan targets.
const Image = "codeberg.org/forgejo/forgejo:16"

const (
	adminUser = "fghadmin"
	adminPass = "fgh-integration-pass-1"
	repoName  = "seed"
)

// Instance is a seeded Forgejo: one repository, a branch pull request and an AGit pull request.
type Instance struct {
	URL      string // http://127.0.0.1:<port>
	Token    string // admin API token
	User     string
	Repo     string // "owner/name"
	CloneURL string // without credentials
	// BranchPR and AGitPR are pull request numbers.
	BranchPR, AGitPR int
	// BranchHead is the head branch of BranchPR.
	BranchHead string
	// AGitTopic is the topic pushed for AGitPR.
	AGitTopic string
}

// Start launches the container, seeds it and registers its teardown with t.Cleanup. Without podman
// (or when it cannot run) the test is skipped with the reason, never passed silently.
func Start(t *testing.T) *Instance {
	t.Helper()
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("integration: podman not found on PATH; run `make test-integration` on a host with podman")
	}
	if out, err := exec.Command("podman", "info").CombinedOutput(); err != nil {
		t.Skipf("integration: podman cannot run here (%v: %s); run `make test-integration` on the host", err, strings.TrimSpace(string(out)))
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("integration: git not found on PATH")
	}

	port := freePort(t)
	name := fmt.Sprintf("fgh-it-%d-%d", os.Getpid(), time.Now().UnixNano()%1_000_000)
	run(t, "podman", "run", "-d", "--rm", "--name", name,
		"-p", fmt.Sprintf("127.0.0.1:%d:3000", port),
		"-e", "FORGEJO__security__INSTALL_LOCK=true",
		"-e", "FORGEJO__database__DB_TYPE=sqlite3",
		"-e", fmt.Sprintf("FORGEJO__server__ROOT_URL=http://127.0.0.1:%d/", port),
		"-e", "FORGEJO__server__DISABLE_SSH=true",
		"-e", "FORGEJO__service__DISABLE_REGISTRATION=true",
		"-e", "FORGEJO__repository__DEFAULT_BRANCH=main",
		Image)
	t.Cleanup(func() { _ = exec.Command("podman", "rm", "-f", "-t", "0", name).Run() })

	in := &Instance{URL: fmt.Sprintf("http://127.0.0.1:%d", port), User: adminUser}
	in.waitReady(t)

	// -u git: Forgejo exits when run as root, and exec defaults to root in this image.
	run(t, "podman", "exec", "-u", "git", name, "forgejo", "admin", "user", "create", "--admin",
		"--username", adminUser, "--password", adminPass,
		"--email", adminUser+"@example.invalid", "--must-change-password=false")

	var tok struct {
		Sha1 string `json:"sha1"`
	}
	in.api(t, http.MethodPost, "/api/v1/users/"+adminUser+"/tokens", true,
		map[string]any{"name": "it", "scopes": []string{"all"}}, &tok)
	in.Token = tok.Sha1

	in.api(t, http.MethodPost, "/api/v1/user/repos", false,
		map[string]any{"name": repoName, "auto_init": true, "default_branch": "main"}, nil)
	in.Repo = adminUser + "/" + repoName
	in.CloneURL = in.URL + "/" + in.Repo + ".git"
	in.seedPulls(t)
	return in
}

func (in *Instance) seedPulls(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	u, _ := url.Parse(in.CloneURL)
	u.User = url.UserPassword(in.User, in.Token)
	g := func(args ...string) string {
		t.Helper()
		return run(t, "git", append([]string{"-C", dir}, args...)...)
	}
	run(t, "git", "clone", "-q", u.String(), dir)
	g("config", "user.name", "fgh it")
	g("config", "user.email", "it@example.invalid")

	commit := func(file string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		g("add", file)
		g("commit", "-q", "-m", "add "+file)
	}

	in.BranchHead = "feature-branch"
	g("checkout", "-q", "-b", in.BranchHead, "origin/main")
	commit("branch.txt")
	g("push", "-q", "origin", in.BranchHead)
	var pr struct {
		Number int `json:"number"`
	}
	in.api(t, http.MethodPost, "/api/v1/repos/"+in.Repo+"/pulls", false,
		map[string]any{"title": "branch pull request", "head": in.BranchHead, "base": "main"}, &pr)
	in.BranchPR = pr.Number

	// AGit: the server opens the pull request itself on a push to refs/for.
	in.AGitTopic = "agit-topic"
	g("checkout", "-q", "-b", "agit-local", "origin/main")
	commit("agit.txt")
	g("push", "-q", "origin", "HEAD:refs/for/main", "-o", "topic="+in.AGitTopic, "-o", "title="+AGitTitle)
	var pulls []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Head   struct {
			Ref string `json:"ref"`
		} `json:"head"`
	}
	in.api(t, http.MethodGet, "/api/v1/repos/"+in.Repo+"/pulls?state=open", false, nil, &pulls)
	for _, p := range pulls {
		if p.Title == AGitTitle && p.Number != in.BranchPR {
			in.AGitPR = p.Number
		}
	}
	if in.AGitPR == 0 {
		t.Fatalf("forgejotest: no open pull request titled %q after the AGit push: %+v", AGitTitle, pulls)
	}
}

func (in *Instance) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(in.URL + "/api/v1/version")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("forgejotest: Forgejo not ready after 120s")
}

// api calls the instance; basic selects admin password auth (needed once, to mint the token).
func (in *Instance) api(t *testing.T, method, path string, basic bool, body, out any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, in.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if basic {
		req.SetBasicAuth(adminUser, adminPass)
	} else {
		req.Header.Set("Authorization", "token "+in.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("forgejotest: %s %s: %d %s", method, path, resp.StatusCode, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("forgejotest: %s %s: %v", method, path, err)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("forgejotest: %s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
