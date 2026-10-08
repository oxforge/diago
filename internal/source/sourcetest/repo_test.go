package sourcetest

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepo_SameCommitsGiveSameHashes(t *testing.T) {
	a, b := New(t), New(t)
	files := map[string]string{"docs/arch.json": "{}"}
	assert.Equal(t, a.Commit("one", files), b.Commit("one", files))
	assert.Equal(t, "{}", a.Git("show", "HEAD:docs/arch.json"))
}

// TestRepo_IgnoresInheritedGitEnv: run from a git hook, where GIT_DIR and
// GIT_INDEX_FILE point at the developer's repository, the helper still
// builds its own repository and leaves the developer's untouched.
func TestRepo_IgnoresInheritedGitEnv(t *testing.T) {
	dev := New(t)
	head := dev.Commit("dev", map[string]string{"a.json": "a"})
	t.Setenv("GIT_DIR", filepath.Join(dev.Dir, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(dev.Dir, ".git", "index"))

	r := New(t)
	r.Commit("one", map[string]string{"b.json": "b"})
	assert.Equal(t, "b", r.Git("show", "HEAD:b.json"))
	assert.Equal(t, head, dev.Git("rev-parse", "HEAD"), "the developer's repository is untouched")
}
