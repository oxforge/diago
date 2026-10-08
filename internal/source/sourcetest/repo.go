// Package sourcetest builds git repositories for tests: fixed identities and
// commit dates and none of the developer's git config, so the same commits
// give the same hashes on every machine.
package sourcetest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Repo is a git repository in a test's temporary directory.
type Repo struct {
	Dir string
	t   testing.TB
	n   int // commits so far: the next one is dated n minutes past 2026-01-01
}

// New initializes a repository on branch main, or skips the test when git
// is not on PATH.
func New(t testing.TB) *Repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	r := &Repo{Dir: t.TempDir(), t: t}
	r.Git("init", "-q", "-b", "main")
	return r
}

// Git runs git in the repository and returns its trimmed output; a failure
// fails the test.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	date := fmt.Sprintf("2026-01-01T00:%02d:00Z", r.n)
	cmd := exec.Command("git", append([]string{"-C", r.Dir}, args...)...)
	cmd.Env = append(withoutGitEnv(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=diago", "GIT_AUTHOR_EMAIL=diago@example.com", "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME=diago", "GIT_COMMITTER_EMAIL=diago@example.com", "GIT_COMMITTER_DATE="+date)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// Path is rel's absolute path in the repository; rel is slash-separated.
func (r *Repo) Path(rel string) string { return filepath.Join(r.Dir, filepath.FromSlash(rel)) }

// Write writes files (slash-separated paths relative to the repository, to
// their contents), creating directories as needed.
func (r *Repo) Write(files map[string]string) {
	r.t.Helper()
	for rel, content := range files {
		p := r.Path(rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

// Commit writes files, stages every change in the work tree and commits; it
// returns the commit's full hash.
func (r *Repo) Commit(msg string, files map[string]string) string {
	r.t.Helper()
	r.Write(files)
	r.Git("add", "-A")
	r.n++
	r.Git("commit", "-q", "--allow-empty", "-m", msg)
	return r.Git("rev-parse", "HEAD")
}

// withoutGitEnv is the test's environment less every GIT_ variable: run from
// a git hook, GIT_DIR and GIT_INDEX_FILE would point every call here at the
// developer's repository.
func withoutGitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return env
}
