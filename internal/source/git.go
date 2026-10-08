package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// gitTimeout bounds every git call, as the CLI bounds a render. A variable
// only so a test can shorten it.
var gitTimeout = 10 * time.Second

// gitConfig pins the settings whose user values would change the output
// parsed here: quoted paths, signature lines, color codes, and what -m
// prints for a merge. (The root commit's raw line, which log.showRoot can
// hide, is asked for with --root.)
var gitConfig = []string{"-c", "core.quotePath=false", "-c", "log.showSignature=false", "-c", "color.ui=false", "-c", "log.diffMerges=first-parent"}

// localEnv is what `git rev-parse --local-env-vars` prints: the variables
// git exports to hooks and to the commands it runs (`git rebase -x`).
// Inherited, they point git at that repository instead of the one that
// contains PATH, so no git call here sees them.
var localEnv = []string{
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
	"GIT_OBJECT_DIRECTORY", "GIT_DIR", "GIT_WORK_TREE", "GIT_IMPLICIT_WORK_TREE", "GIT_GRAFT_FILE",
	"GIT_INDEX_FILE", "GIT_NO_REPLACE_OBJECTS", "GIT_REPLACE_REF_BASE", "GIT_PREFIX", "GIT_SHALLOW_FILE",
	"GIT_COMMON_DIR",
}

// gitEnv is the environment every git call runs in: the caller's, less
// localEnv.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(localEnv, name) {
			env = append(env, kv)
		}
	}
	return env
}

// errNoGit is git missing from PATH.
var errNoGit = errors.New("git not found on PATH")

// repo is where git runs for one PATH: dir is the nearest existing ancestor
// directory of PATH, and rel is PATH relative to it with a "./" prefix, so
// git resolves it there and not at the repository root.
type repo struct {
	dir, rel string
}

// locate finds file's repository; arg is the argument as given, for the
// git-missing message.
func locate(ctx context.Context, arg, file string) (repo, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return repo{}, err
	}
	dir := filepath.Dir(abs)
	for {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	rel, err := filepath.Rel(dir, abs)
	if err != nil {
		return repo{}, err
	}
	r := repo{dir: dir, rel: "./" + filepath.ToSlash(rel)}
	if _, err := r.git(ctx, "rev-parse", "--git-dir"); err != nil {
		var ge *gitError
		if errors.As(err, &ge) && !ge.timeout && !hasGitEntry(dir) {
			return repo{}, &Error{Kind: NotARepo, Msg: fmt.Sprintf("%s is not in a git repository", file)}
		}
		return repo{}, gitFailed(err, arg)
	}
	return r, nil
}

// hasGitEntry reports whether dir or one of its ancestors holds a .git
// entry, the mark git's own discovery looks for. When one does, git's
// failure to open the repository has a reason of its own (an ownership
// check, a damaged repository), which gitFailed reports.
func hasGitEntry(dir string) bool {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// commit resolves rev to a full commit hash. --quiet keeps git silent on a
// name that does not resolve, so a failure with something on stderr has
// another cause.
func (r repo) commit(ctx context.Context, rev string) (string, error) {
	out, err := r.git(ctx, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", classify(err, rev, UnknownRevision, fmt.Sprintf("unknown revision %q", rev))
	}
	return strings.TrimSpace(string(out)), nil
}

// gitError is a failed git call.
type gitError struct {
	args    []string
	stderr  string // git's first non-empty stderr line; "" when git said nothing
	timeout bool   // the call outlived gitTimeout
	err     error
}

func (e *gitError) Error() string {
	return fmt.Sprintf("git %s: %s", strings.Join(e.args, " "), e.reason())
}

func (e *gitError) Unwrap() error { return e.err }

// reason is what a GitFailed message quotes: the timeout, git's own words,
// or the exit status when git said nothing.
func (e *gitError) reason() string {
	switch {
	case e.timeout:
		return fmt.Sprintf("timed out after %s", gitTimeout)
	case e.stderr != "":
		return e.stderr
	default:
		return e.err.Error()
	}
}

// classify reports a git call whose expected failure is silent (a --quiet
// query on a name that does not resolve) as kind with msg, and any other
// failure through gitFailed.
func classify(err error, arg string, kind Kind, msg string) error {
	var ge *gitError
	if errors.As(err, &ge) && !ge.timeout && ge.stderr == "" {
		return &Error{Kind: kind, Msg: msg}
	}
	return gitFailed(err, arg)
}

// gitFailed reports a git call that failed for a reason no other Kind names,
// quoting that reason; a missing git is GitMissing.
func gitFailed(err error, arg string) error {
	if errors.Is(err, errNoGit) {
		return &Error{Kind: GitMissing, Msg: fmt.Sprintf("git not found on PATH (needed to read %s)", arg)}
	}
	reason := err.Error()
	var ge *gitError
	if errors.As(err, &ge) {
		reason = ge.reason()
	}
	return &Error{Kind: GitFailed, Msg: fmt.Sprintf("git failed reading %s: %s", arg, reason)}
}

// git runs git in r.dir and returns its stdout; a failure is a *gitError,
// and a missing git is errNoGit.
func (r repo) git(ctx context.Context, args ...string) ([]byte, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, errNoGit
	}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd, stderr := r.command(ctx, bin, args)
	out, err := cmd.Output()
	if err != nil {
		return nil, failure(ctx, args, stderr, err)
	}
	return out, nil
}

// walk calls each with the versions of r.rel, newest first, along first
// parents from HEAD and following renames, until each returns false. git
// streams them and is stopped there, so the walk reads history only back to
// the version the caller needs, not to the root commit.
func (r repo) walk(ctx context.Context, each func(version) bool) error {
	bin, err := exec.LookPath("git")
	if err != nil {
		return errNoGit
	}
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	args := []string{"log", "-z", "-m", "--first-parent", "--follow", "--root", "--format=%H", "--raw", "--no-abbrev", "--", r.rel}
	cmd, stderr := r.command(ctx, bin, args)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("git log: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return failure(ctx, args, stderr, err)
	}
	log := newRawLog(out)
	for {
		v, err := log.next()
		if err == io.EOF {
			break
		}
		if err != nil { // git's output cannot be read: stop git, report why
			cancel()
			_ = cmd.Wait()
			return err
		}
		if !each(v) {
			cancel() // the rest of the history is not needed
			_ = cmd.Wait()
			return nil
		}
	}
	if err := cmd.Wait(); err != nil {
		return failure(ctx, args, stderr, err)
	}
	return nil
}

// command is git with args in r.dir, under gitConfig and gitEnv; its stderr
// goes to the buffer returned beside it.
func (r repo) command(ctx context.Context, bin string, args []string) (*exec.Cmd, *bytes.Buffer) {
	full := append(append([]string{"-C", r.dir}, gitConfig...), args...)
	cmd := exec.CommandContext(ctx, bin, full...)
	cmd.Env = gitEnv()
	// Once git is stopped, a child it started (a lazy fetch in a partial
	// clone) may still hold its pipes; Wait gives up on them after a second
	// instead of waiting for that child.
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return cmd, &stderr
}

// failure is the *gitError of a git call that failed with err: a timeout
// when ctx expired, else git's first non-empty stderr line.
func failure(ctx context.Context, args []string, stderr *bytes.Buffer, err error) *gitError {
	ge := &gitError{args: args, timeout: ctx.Err() != nil, err: err}
	for _, line := range strings.Split(stderr.String(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			ge.stderr = line
			break
		}
	}
	return ge
}

// revisionLabel names a REV:PATH side of a diff caption: the base name, then
// the revision as typed and the commit's first seven hash characters, or
// those seven alone when the revision is already a prefix of the hash.
func revisionLabel(base, rev, hash string) string {
	short := hash[:7]
	if isHashPrefix(rev, hash) {
		return base + " @ " + short
	}
	return fmt.Sprintf("%s @ %s (%s)", base, rev, short)
}

// isHashPrefix reports whether rev is hex digits that begin hash.
func isHashPrefix(rev, hash string) bool {
	if rev == "" {
		return false
	}
	for _, c := range rev {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return strings.HasPrefix(hash, strings.ToLower(rev))
}
