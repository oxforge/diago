// Package source resolves a spec argument to its bytes: stdin, a file on
// disk, or REV:PATH, a file as it was at a git revision. It knows nothing of
// Mermaid, schemas or rendering.
package source

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Spec is one resolved spec argument.
type Spec struct {
	Data  []byte // the raw bytes, JSON or Mermaid, untranslated
	Name  string // the path part, for Mermaid sniffing by extension ("" for stdin)
	Label string // this side's half of a diff caption
}

// Kind classifies an Error.
type Kind int

const (
	BadArgument       Kind = iota + 1 // REV:PATH with an empty REV or PATH
	GitMissing                        // git is not on PATH
	NotARepo                          // PATH is not inside a git repository
	UnknownRevision                   // REV does not resolve to a commit, or starts with "-"
	MissingAtRevision                 // PATH does not exist at REV
	NotTracked                        // Latest: the file has no committed version
	NoEarlierVersion                  // Latest: every committed version equals the file on disk
	GitFailed                         // git failed for a reason none of the above names: a refused repository, a timeout
	Symlink                           // PATH is a symbolic link, whose git history is its link text
)

// Error is a spec argument git cannot resolve. Msg names the argument, the
// path or the revision it is about.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// Read resolves one spec argument. "" and "-" are stdin; a name that exists
// on disk is that file, even when it contains a colon; any other name with a
// colon is REV:PATH, split at the first colon, PATH relative to the current
// directory; anything else is read as a file, so a missing one fails as
// os.ReadFile does.
func Read(ctx context.Context, arg string, stdin io.Reader) (Spec, error) {
	if arg == "" || arg == "-" {
		if stdin == nil {
			return Spec{}, errors.New("read stdin: no stdin to read")
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			return Spec{}, fmt.Errorf("read stdin: %w", err)
		}
		return Spec{Data: b, Label: "stdin"}, nil
	}
	if _, err := os.Stat(arg); err == nil {
		return readFile(arg)
	}
	if rev, file, ok := strings.Cut(arg, ":"); ok {
		return readRevision(ctx, arg, rev, file)
	}
	return readFile(arg)
}

// readFile reads a file on disk.
func readFile(file string) (Spec, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return Spec{}, err
	}
	return Spec{Data: b, Name: file, Label: filepath.Base(file)}, nil
}

// readRevision reads file as it was at rev; arg is the argument as given.
func readRevision(ctx context.Context, arg, rev, file string) (Spec, error) {
	switch {
	case rev == "":
		return Spec{}, &Error{Kind: BadArgument, Msg: fmt.Sprintf("%s: a revision is needed before the colon", arg)}
	case file == "":
		return Spec{}, &Error{Kind: BadArgument, Msg: fmt.Sprintf("%s: a path is needed after the colon", arg)}
	case strings.HasPrefix(rev, "-"):
		// git would read it as an option.
		return Spec{}, &Error{Kind: UnknownRevision, Msg: fmt.Sprintf("unknown revision %q", rev)}
	}
	r, err := locate(ctx, arg, file)
	if err != nil {
		return Spec{}, err
	}
	hash, err := r.commit(ctx, rev)
	if err != nil {
		return Spec{}, err
	}
	entry, err := r.git(ctx, "ls-tree", "-z", hash, "--", r.rel)
	if err != nil {
		return Spec{}, gitFailed(err, arg)
	}
	mode, kind, id, ok := treeEntry(entry)
	if !ok || kind != "blob" { // nothing there, or a directory
		return Spec{}, &Error{Kind: MissingAtRevision, Msg: fmt.Sprintf("%s does not exist at %s (%s)", file, rev, hash[:7])}
	}
	data, err := r.git(ctx, "cat-file", "blob", id)
	if err != nil {
		return Spec{}, gitFailed(err, arg)
	}
	if mode == symlinkMode {
		return Spec{}, &Error{Kind: Symlink, Msg: fmt.Sprintf("%s is a symbolic link at %s (%s), to %s; pass the file it points to", file, rev, hash[:7], data)}
	}
	return Spec{Data: data, Name: file, Label: revisionLabel(filepath.Base(file), rev, hash)}, nil
}

// Latest resolves `diago diff PATH`: cur is the file on disk, and old the
// newest version of it committed along first parents from HEAD, following
// renames, whose content differs from cur.
func Latest(ctx context.Context, file string) (old, cur Spec, err error) {
	if fi, err := os.Lstat(file); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		// Its history is its link text, not the spec it points to.
		if target, err := filepath.EvalSymlinks(file); err == nil {
			return Spec{}, Spec{}, &Error{Kind: Symlink, Msg: fmt.Sprintf("%s is a symbolic link; pass the file it points to: %s", file, target)}
		}
		target, _ := os.Readlink(file)
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(file), target)
		}
		return Spec{}, Spec{}, &Error{Kind: Symlink, Msg: fmt.Sprintf("%s is a symbolic link to %s, which does not exist", file, target)}
	}
	if cur, err = readFile(file); err != nil {
		return Spec{}, Spec{}, err
	}
	r, err := locate(ctx, file, file)
	if err != nil {
		return Spec{}, Spec{}, err
	}
	notTracked := &Error{Kind: NotTracked, Msg: fmt.Sprintf("%s is not tracked by git; pass two specs", file)}
	if _, err := r.commit(ctx, "HEAD"); err != nil {
		var e *Error
		if errors.As(err, &e) && e.Kind == UnknownRevision {
			return Spec{}, Spec{}, notTracked // a repository with no commit yet
		}
		return Spec{}, Spec{}, err
	}
	disk, err := r.git(ctx, "hash-object", "--", r.rel)
	if err != nil {
		return Spec{}, Spec{}, gitFailed(err, file)
	}
	blob := strings.TrimSpace(string(disk))
	tracked := false
	var found *version
	err = r.walk(ctx, func(v version) bool {
		tracked = true
		if v.blob == blob {
			return true
		}
		found = &v
		return false
	})
	if err != nil {
		return Spec{}, Spec{}, gitFailed(err, file)
	}
	if !tracked {
		return Spec{}, Spec{}, notTracked
	}
	if found != nil {
		if found.mode == symlinkMode {
			return Spec{}, Spec{}, &Error{Kind: Symlink, Msg: fmt.Sprintf("%s was a symbolic link at %s, so that version is a link's text, not a spec; pass two specs", file, found.commit[:7])}
		}
		data, err := r.git(ctx, "cat-file", "blob", found.blob)
		if err != nil {
			return Spec{}, Spec{}, gitFailed(err, file)
		}
		return Spec{Data: data, Name: found.path, Label: path.Base(found.path) + " @ " + found.commit[:7]}, cur, nil
	}
	msg := fmt.Sprintf("%s has no earlier version in git to compare with", file)
	if out, err := r.git(ctx, "rev-parse", "--is-shallow-repository"); err == nil && strings.TrimSpace(string(out)) == "true" {
		msg += "; the clone is shallow, so older versions may be missing (git fetch --unshallow)"
	}
	return Spec{}, Spec{}, &Error{Kind: NoEarlierVersion, Msg: msg}
}

// version is one committed version of a file.
type version struct {
	commit string // the full hash of the commit that wrote it
	mode   string // the file's mode at that commit; symlinkMode for a symbolic link
	blob   string // the blob id of the file's content at that commit
	path   string // the file's repo-relative path at that commit, slash-separated
}

// rawLog reads `git log -z --format=%H --raw --no-abbrev` one version at a
// time. Its fields end in NUL: a commit hash, then for each file the commit
// changed a ":<modes> <old> <new> <status>" field (after a newline) and its
// path, or two paths for a rename or copy, the new one last. -z leaves every
// path unquoted, whatever it holds.
type rawLog struct {
	r      *bufio.Reader
	commit string
}

func newRawLog(r io.Reader) *rawLog { return &rawLog{r: bufio.NewReader(r)} }

// next returns the next version, newest first, skipping deletions (an
// all-zero new blob); io.EOF ends the log.
func (l *rawLog) next() (version, error) {
	for {
		field, err := l.field()
		if err != nil {
			return version{}, err
		}
		field = strings.TrimLeft(field, "\n")
		if !strings.HasPrefix(field, ":") {
			if field != "" {
				l.commit = field
			}
			continue
		}
		meta := strings.Fields(field)
		if len(meta) < 5 {
			return version{}, fmt.Errorf("git log: unexpected raw field %q", field)
		}
		file, err := l.path()
		if err != nil {
			return version{}, err
		}
		if status := meta[4]; status[0] == 'R' || status[0] == 'C' {
			if file, err = l.path(); err != nil {
				return version{}, err
			}
		}
		if l.commit == "" || strings.Trim(meta[3], "0") == "" {
			continue
		}
		return version{commit: l.commit, mode: meta[1], blob: meta[3], path: file}, nil
	}
}

// field reads one NUL-terminated field; a field cut short by the end of the
// output is dropped, and io.EOF returned.
func (l *rawLog) field() (string, error) {
	f, err := l.r.ReadString(0)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(f, "\x00"), nil
}

// path reads the path field a raw field announced: the log cannot end there.
func (l *rawLog) path() (string, error) {
	f, err := l.field()
	if err == io.EOF {
		return "", io.ErrUnexpectedEOF
	}
	return f, err
}

// symlinkMode is the mode git stores a symbolic link under.
const symlinkMode = "120000"

// treeEntry reads the first entry `git ls-tree -z` printed:
// "<mode> <type> <object>\t<path>"; ok is false when it printed none.
func treeEntry(out []byte) (mode, kind, id string, ok bool) {
	entry, _, _ := strings.Cut(string(out), "\x00")
	meta, _, found := strings.Cut(entry, "\t")
	fields := strings.Fields(meta)
	if !found || len(fields) != 3 {
		return "", "", "", false
	}
	return fields[0], fields[1], fields[2], true
}
