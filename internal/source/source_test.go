package source

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/source/sourcetest"
)

// kindOf returns err's Kind, or 0 when err is not an *Error.
func kindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}

func TestRead_StdinAndFiles(t *testing.T) {
	ctx := context.Background()
	for _, arg := range []string{"", "-"} {
		s, err := Read(ctx, arg, strings.NewReader("{}"))
		require.NoError(t, err)
		assert.Equal(t, Spec{Data: []byte("{}"), Label: "stdin"}, s, arg)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "arch.json")
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644))
	s, err := Read(ctx, path, nil)
	require.NoError(t, err)
	assert.Equal(t, Spec{Data: []byte("{}"), Name: path, Label: "arch.json"}, s)

	// A name with a colon that exists on disk is a file, not REV:PATH.
	colon := filepath.Join(dir, "v1:arch.json")
	require.NoError(t, os.WriteFile(colon, []byte("colon"), 0o644))
	s, err = Read(ctx, colon, nil)
	require.NoError(t, err)
	assert.Equal(t, "colon", string(s.Data))
	assert.Equal(t, "v1:arch.json", s.Label)

	// A missing file without a colon fails as os.ReadFile does.
	_, err = Read(ctx, filepath.Join(dir, "missing.json"), nil)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestRead_Revision(t *testing.T) {
	ctx := context.Background()
	r := sourcetest.New(t)
	c1 := r.Commit("v1", map[string]string{"docs/arch.json": "one"})
	c2 := r.Commit("v2", map[string]string{"docs/arch.json": "two"})
	r.Git("tag", "v1", c1)
	abs := r.Path("docs/arch.json")

	t.Run("relative to the current directory", func(t *testing.T) {
		t.Chdir(r.Path("docs"))
		s, err := Read(ctx, "HEAD~1:arch.json", nil)
		require.NoError(t, err)
		assert.Equal(t, Spec{Data: []byte("one"), Name: "arch.json", Label: "arch.json @ HEAD~1 (" + c1[:7] + ")"}, s)
	})
	t.Run("absolute, at HEAD", func(t *testing.T) {
		s, err := Read(ctx, "HEAD:"+abs, nil)
		require.NoError(t, err)
		assert.Equal(t, "two", string(s.Data))
		assert.Equal(t, "arch.json @ HEAD ("+c2[:7]+")", s.Label)
	})
	t.Run("a tag", func(t *testing.T) {
		s, err := Read(ctx, "v1:"+abs, nil)
		require.NoError(t, err)
		assert.Equal(t, "one", string(s.Data))
		assert.Equal(t, "arch.json @ v1 ("+c1[:7]+")", s.Label)
	})
	t.Run("a hash prefix", func(t *testing.T) {
		s, err := Read(ctx, c1[:10]+":"+abs, nil)
		require.NoError(t, err)
		assert.Equal(t, "arch.json @ "+c1[:7], s.Label)
	})
	t.Run("a directory deleted since", func(t *testing.T) {
		r := sourcetest.New(t)
		r.Commit("v1", map[string]string{"old/arch.json": "one"})
		r.Git("rm", "-q", "-r", "old")
		r.Commit("move", map[string]string{"new/arch.json": "two"})
		_, err := os.Stat(r.Path("old"))
		require.ErrorIs(t, err, fs.ErrNotExist)
		s, err := Read(ctx, "HEAD~1:"+r.Path("old/arch.json"), nil)
		require.NoError(t, err)
		assert.Equal(t, "one", string(s.Data))
	})
}

func TestRead_RevisionErrors(t *testing.T) {
	ctx := context.Background()
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"arch.json": "one", "docs/a.json": "a"})
	abs := r.Path("arch.json")
	for name, tc := range map[string]struct {
		arg  string
		kind Kind
		msg  string
	}{
		"empty revision":              {":" + abs, BadArgument, "a revision is needed before the colon"},
		"empty path":                  {"HEAD:", BadArgument, "a path is needed after the colon"},
		"unknown revision":            {"nope:" + abs, UnknownRevision, `unknown revision "nope"`},
		"a revision that is a flag":   {"-x:" + abs, UnknownRevision, `unknown revision "-x"`},
		"missing at the revision":     {"HEAD:" + r.Path("missing.json"), MissingAtRevision, "missing.json does not exist at HEAD ("},
		"a directory at the revision": {"HEAD:" + r.Path("docs"), MissingAtRevision, "does not exist at HEAD ("},
		"not in a repository":         {"HEAD:" + filepath.Join(t.TempDir(), "x.json"), NotARepo, "x.json is not in a git repository"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Read(ctx, tc.arg, nil)
			require.Error(t, err)
			assert.Equal(t, tc.kind, kindOf(err))
			assert.Contains(t, err.Error(), tc.msg)
		})
	}
}

func TestRead_GitMissing(t *testing.T) {
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"arch.json": "one"})
	t.Setenv("PATH", t.TempDir())
	_, err := Read(context.Background(), "HEAD:"+r.Path("arch.json"), nil)
	assert.Equal(t, GitMissing, kindOf(err))
	assert.Contains(t, err.Error(), "git not found on PATH (needed to read HEAD:")
	_, _, err = Latest(context.Background(), r.Path("arch.json"))
	assert.Equal(t, GitMissing, kindOf(err))
}

func TestRead_UnusualPaths(t *testing.T) {
	ctx := context.Background()
	r := sourcetest.New(t)
	name := "my docs/über arch.json"
	c1 := r.Commit("v1", map[string]string{name: "one"})
	r.Commit("v2", map[string]string{name: "two"})
	s, err := Read(ctx, "HEAD~1:"+r.Path(name), nil)
	require.NoError(t, err)
	assert.Equal(t, "one", string(s.Data))
	assert.Equal(t, "über arch.json @ HEAD~1 ("+c1[:7]+")", s.Label)
	old, _, err := Latest(ctx, r.Path(name))
	require.NoError(t, err)
	assert.Equal(t, "über arch.json @ "+c1[:7], old.Label)
}

// TestRead_LinkedWorktree: in a linked worktree, HEAD is that worktree's
// HEAD, which here has moved past the main worktree's.
func TestRead_LinkedWorktree(t *testing.T) {
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"arch.json": "one"})
	wt := filepath.Join(t.TempDir(), "wt")
	r.Git("worktree", "add", "-q", "-b", "wt", wt)
	require.NoError(t, os.WriteFile(filepath.Join(wt, "arch.json"), []byte("two"), 0o644))
	r.Git("-C", wt, "commit", "-q", "-am", "v2 in the worktree")
	c2 := r.Git("-C", wt, "rev-parse", "HEAD")
	require.NotEqual(t, c2, r.Git("rev-parse", "HEAD"), "the two worktrees' HEADs differ")

	s, err := Read(context.Background(), "HEAD:"+filepath.Join(wt, "arch.json"), nil)
	require.NoError(t, err)
	assert.Equal(t, "two", string(s.Data))
	assert.Equal(t, "arch.json @ HEAD ("+c2[:7]+")", s.Label)
}

func TestLatest(t *testing.T) {
	ctx := context.Background()
	t.Run("uncommitted edits against HEAD", func(t *testing.T) {
		r := sourcetest.New(t)
		r.Commit("v1", map[string]string{"arch.json": "one"})
		c2 := r.Commit("v2", map[string]string{"arch.json": "two"})
		r.Write(map[string]string{"arch.json": "three"})
		old, cur, err := Latest(ctx, r.Path("arch.json"))
		require.NoError(t, err)
		assert.Equal(t, Spec{Data: []byte("two"), Name: "arch.json", Label: "arch.json @ " + c2[:7]}, old)
		assert.Equal(t, Spec{Data: []byte("three"), Name: r.Path("arch.json"), Label: "arch.json"}, cur)
	})
	t.Run("a clean file against the version before its last change", func(t *testing.T) {
		r := sourcetest.New(t)
		c1 := r.Commit("v1", map[string]string{"arch.json": "one"})
		r.Commit("v2", map[string]string{"arch.json": "two"})
		old, cur, err := Latest(ctx, r.Path("arch.json"))
		require.NoError(t, err)
		assert.Equal(t, "one", string(old.Data))
		assert.Equal(t, "arch.json @ "+c1[:7], old.Label)
		assert.Equal(t, "two", string(cur.Data))
	})
	t.Run("a rename-only commit is skipped", func(t *testing.T) {
		r := sourcetest.New(t)
		c1 := r.Commit("v1", map[string]string{"a.json": "one"})
		r.Commit("v2", map[string]string{"a.json": "two"})
		r.Git("mv", "a.json", "b.json")
		r.Commit("rename", nil)
		old, _, err := Latest(ctx, r.Path("b.json"))
		require.NoError(t, err)
		assert.Equal(t, Spec{Data: []byte("one"), Name: "a.json", Label: "a.json @ " + c1[:7]}, old)
	})
	t.Run("a revert shows the version it reverted", func(t *testing.T) {
		r := sourcetest.New(t)
		r.Commit("v1", map[string]string{"arch.json": "one"})
		c2 := r.Commit("v2", map[string]string{"arch.json": "two"})
		r.Commit("revert", map[string]string{"arch.json": "one"})
		old, _, err := Latest(ctx, r.Path("arch.json"))
		require.NoError(t, err)
		assert.Equal(t, "two", string(old.Data))
		assert.Equal(t, "arch.json @ "+c2[:7], old.Label)
	})
	t.Run("a merge counts along first parents", func(t *testing.T) {
		r := sourcetest.New(t)
		c1 := r.Commit("v1", map[string]string{"arch.json": "one"})
		r.Git("checkout", "-q", "-b", "side")
		r.Commit("side", map[string]string{"arch.json": "two"})
		r.Git("checkout", "-q", "main")
		r.Git("merge", "-q", "--no-ff", "side", "-m", "merge")
		old, cur, err := Latest(ctx, r.Path("arch.json"))
		require.NoError(t, err)
		assert.Equal(t, "two", string(cur.Data))
		assert.Equal(t, "one", string(old.Data))
		assert.Equal(t, "arch.json @ "+c1[:7], old.Label)
	})
	t.Run("a single version", func(t *testing.T) {
		r := sourcetest.New(t)
		r.Commit("v1", map[string]string{"arch.json": "one"})
		_, _, err := Latest(ctx, r.Path("arch.json"))
		assert.Equal(t, NoEarlierVersion, kindOf(err))
		assert.EqualError(t, err, r.Path("arch.json")+" has no earlier version in git to compare with")
	})
	t.Run("an untracked file", func(t *testing.T) {
		r := sourcetest.New(t)
		r.Commit("v1", map[string]string{"other.json": "x"})
		r.Write(map[string]string{"arch.json": "one"})
		_, _, err := Latest(ctx, r.Path("arch.json"))
		assert.Equal(t, NotTracked, kindOf(err))
		assert.Contains(t, err.Error(), "arch.json is not tracked by git; pass two specs")
	})
	t.Run("a repository with no commit yet", func(t *testing.T) {
		r := sourcetest.New(t)
		r.Write(map[string]string{"arch.json": "one"})
		_, _, err := Latest(ctx, r.Path("arch.json"))
		assert.Equal(t, NotTracked, kindOf(err))
	})
	t.Run("a shallow clone", func(t *testing.T) {
		r := sourcetest.New(t)
		r.Commit("v1", map[string]string{"arch.json": "one"})
		r.Commit("v2", map[string]string{"arch.json": "two"})
		clone := filepath.Join(t.TempDir(), "clone")
		r.Git("clone", "-q", "--depth", "1", "file://"+r.Dir, clone)
		_, _, err := Latest(ctx, filepath.Join(clone, "arch.json"))
		assert.Equal(t, NoEarlierVersion, kindOf(err))
		assert.Contains(t, err.Error(), "the clone is shallow")
	})
	t.Run("missing on disk", func(t *testing.T) {
		r := sourcetest.New(t)
		r.Commit("v1", map[string]string{"arch.json": "one"})
		_, _, err := Latest(ctx, r.Path("gone.json"))
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
	t.Run("not in a repository", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "arch.json")
		require.NoError(t, os.WriteFile(path, []byte("one"), 0o644))
		_, _, err := Latest(ctx, path)
		assert.Equal(t, NotARepo, kindOf(err))
	})
}

func TestParseVersions(t *testing.T) {
	zero := strings.Repeat("0", 40)
	log := "c3\n\n:100644 100644 b2 b2 R100\told/a.json\tnew/a.json\n" +
		"c2\n\n:100644 100644 b1 b2 M\told/a.json\n" +
		"c1\n\n:000000 100644 " + zero + " b1 A\told/a.json\n" +
		"c0\n\n:100644 000000 b0 " + zero + " D\told/a.json\n"
	assert.Equal(t, []version{
		{commit: "c3", blob: "b2", path: "new/a.json"},
		{commit: "c2", blob: "b2", path: "old/a.json"},
		{commit: "c1", blob: "b1", path: "old/a.json"},
	}, parseVersions([]byte(log)))
}

func TestRevisionLabel(t *testing.T) {
	hash := "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"
	for rev, want := range map[string]string{
		"HEAD~3":   "arch.json @ HEAD~3 (a1b2c3d)",
		"v1.2":     "arch.json @ v1.2 (a1b2c3d)",
		"a1b2c3d4": "arch.json @ a1b2c3d",
		"A1B2C3D":  "arch.json @ a1b2c3d",
		"cafe":     "arch.json @ cafe (a1b2c3d)",
	} {
		assert.Equal(t, want, revisionLabel("arch.json", rev, hash), rev)
	}
}

// TestRead_IgnoresInheritedGitEnv: a git hook or a `git rebase -x` step
// exports GIT_DIR and GIT_INDEX_FILE (in a linked worktree with no
// GIT_WORK_TREE); diago still reads the repository that contains PATH.
func TestRead_IgnoresInheritedGitEnv(t *testing.T) {
	ctx := context.Background()
	other := sourcetest.New(t)
	other.Commit("other", map[string]string{"arch.json": "other"})
	r := sourcetest.New(t)
	c1 := r.Commit("v1", map[string]string{"docs/arch.json": "one"})
	r.Commit("v2", map[string]string{"docs/arch.json": "two"})
	t.Setenv("GIT_DIR", filepath.Join(other.Dir, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other.Dir, ".git", "index"))

	s, err := Read(ctx, "HEAD~1:"+r.Path("docs/arch.json"), nil)
	require.NoError(t, err)
	assert.Equal(t, "one", string(s.Data))
	old, _, err := Latest(ctx, r.Path("docs/arch.json"))
	require.NoError(t, err)
	assert.Equal(t, "one", string(old.Data))
	assert.Equal(t, "arch.json @ "+c1[:7], old.Label)
}

// TestLatest_UserConfigDoesNotChangeTheWalk: settings that change what git
// log prints (log.showRoot hides the root commit's raw line, core.quotePath
// quotes non-ASCII paths) do not change which version Latest finds.
func TestLatest_UserConfigDoesNotChangeTheWalk(t *testing.T) {
	r := sourcetest.New(t)
	name := "über arch.json"
	c1 := r.Commit("v1", map[string]string{name: "one"})
	r.Commit("v2", map[string]string{name: "two"})
	r.Git("config", "log.showRoot", "false")
	r.Git("config", "core.quotePath", "true")
	old, _, err := Latest(context.Background(), r.Path(name))
	require.NoError(t, err)
	assert.Equal(t, "one", string(old.Data))
	assert.Equal(t, name+" @ "+c1[:7], old.Label)
}

// TestRead_ReportsWhyGitRefusesARepository: a repository git will not open
// (dubious ownership, common in CI containers) is reported with git's own
// reason, not as a missing repository.
func TestRead_ReportsWhyGitRefusesARepository(t *testing.T) {
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"arch.json": "one"})
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	_, err := Read(context.Background(), "HEAD:"+r.Path("arch.json"), nil)
	if err == nil {
		t.Skip("this git ignores GIT_TEST_ASSUME_DIFFERENT_OWNER")
	}
	assert.Equal(t, GitFailed, kindOf(err))
	assert.Contains(t, err.Error(), "git failed reading HEAD:")
	assert.NotContains(t, err.Error(), "is not in a git repository")
	_, _, err = Latest(context.Background(), r.Path("arch.json"))
	assert.Equal(t, GitFailed, kindOf(err))
}

// TestRead_ReportsATimeout: a git call that outlives gitTimeout says so, in
// place of an unknown revision or a missing repository.
func TestRead_ReportsATimeout(t *testing.T) {
	r := sourcetest.New(t)
	r.Commit("v1", map[string]string{"arch.json": "one"})
	saved := gitTimeout
	gitTimeout = time.Nanosecond
	t.Cleanup(func() { gitTimeout = saved })
	_, err := Read(context.Background(), "HEAD:"+r.Path("arch.json"), nil)
	assert.Equal(t, GitFailed, kindOf(err))
	assert.Contains(t, err.Error(), "timed out after 1ns")
}

// TestRead_StdinWithoutAReader: a caller that has no stdin to offer passes
// nil, and "" or "-" is then an error, not a nil-pointer read.
func TestRead_StdinWithoutAReader(t *testing.T) {
	for _, arg := range []string{"", "-"} {
		_, err := Read(context.Background(), arg, nil)
		assert.EqualError(t, err, "read stdin: no stdin to read", arg)
	}
}
