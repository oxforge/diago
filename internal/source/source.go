// Package source resolves a spec argument to its bytes: stdin, a file on
// disk, or REV:PATH, a file as it was at a git revision. It knows nothing of
// Mermaid, schemas or rendering.
package source

import (
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
	missing := fmt.Sprintf("%s does not exist at %s (%s)", file, rev, hash[:7])
	oid, err := r.git(ctx, "rev-parse", "--verify", "--quiet", hash+":"+r.rel)
	if err != nil {
		return Spec{}, classify(err, arg, MissingAtRevision, missing)
	}
	id := strings.TrimSpace(string(oid))
	kind, err := r.git(ctx, "cat-file", "-t", id)
	if err != nil {
		return Spec{}, gitFailed(err, arg)
	}
	if strings.TrimSpace(string(kind)) != "blob" {
		return Spec{}, &Error{Kind: MissingAtRevision, Msg: missing} // a directory at rev
	}
	data, err := r.git(ctx, "cat-file", "blob", id)
	if err != nil {
		return Spec{}, gitFailed(err, arg)
	}
	return Spec{Data: data, Name: file, Label: revisionLabel(filepath.Base(file), rev, hash)}, nil
}

// Latest resolves `diago diff PATH`: cur is the file on disk, and old the
// newest version of it committed along first parents from HEAD, following
// renames, whose content differs from cur.
func Latest(ctx context.Context, file string) (old, cur Spec, err error) {
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
	log, err := r.git(ctx, "log", "-m", "--first-parent", "--follow", "--root", "--format=%H", "--raw", "--no-abbrev", "--", r.rel)
	if err != nil {
		return Spec{}, Spec{}, gitFailed(err, file)
	}
	versions := parseVersions(log)
	if len(versions) == 0 {
		return Spec{}, Spec{}, notTracked
	}
	blob := strings.TrimSpace(string(disk))
	for _, v := range versions {
		if v.blob == blob {
			continue
		}
		data, err := r.git(ctx, "cat-file", "blob", v.blob)
		if err != nil {
			return Spec{}, Spec{}, gitFailed(err, file)
		}
		return Spec{Data: data, Name: v.path, Label: path.Base(v.path) + " @ " + v.commit[:7]}, cur, nil
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
	blob   string // the blob id of the file's content at that commit
	path   string // the file's repo-relative path at that commit, slash-separated
}

// parseVersions reads `git log --format=%H --raw --no-abbrev`, newest first:
// a commit hash line, then one ":<modes> <old> <new> <status>\t<path>" line
// (two paths for a rename or copy, the new one last) per file it changed. A
// deletion, whose new blob is all zeros, is not a version.
func parseVersions(log []byte) []version {
	var out []version
	commit := ""
	for _, line := range strings.Split(string(log), "\n") {
		switch {
		case line == "":
		case strings.HasPrefix(line, ":"):
			meta, paths, ok := strings.Cut(line, "\t")
			fields := strings.Fields(meta)
			if !ok || len(fields) < 5 || commit == "" || strings.Trim(fields[3], "0") == "" {
				continue
			}
			names := strings.Split(paths, "\t")
			out = append(out, version{commit: commit, blob: fields[3], path: names[len(names)-1]})
		default:
			commit = line
		}
	}
	return out
}
