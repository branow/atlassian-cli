package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
)

// stdoutDestination is the --out value that streams the bytes to stdout
// instead of a file, for piping into a converter.
const stdoutDestination = "-"

// downloadSpec describes one attachment download: where the bytes live and
// how to name them when the response does not say. Jira and Confluence
// differ only in these three values, so both products' download commands
// are the same flow over a different spec.
type downloadSpec struct {
	// path is the post-host path serving the bytes.
	path string
	// id identifies the attachment in messages and in the last-resort
	// filename.
	id string
	// name is the filename to use when the response carries no
	// Content-Disposition. Empty means fall back to "attachment-<id>".
	name string
}

// runAttachmentDownload streams an attachment to dest: a file path, a
// directory to drop the server-named file into, "-" for stdout, or "" for
// the server's own filename in the working directory. The bytes are copied
// straight from the response to the destination rather than buffered, so an
// attachment larger than memory still works.
func runAttachmentDownload(ctx context.Context, f *cmdutil.Factory, spec downloadSpec, dest string) error {
	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	download, err := client.Download(ctx, spec.path)
	if err != nil {
		return err
	}
	defer download.Body.Close()

	filename := firstNonEmpty(download.Filename, atlapi.SafeFilename(spec.name), "attachment-"+spec.id)
	if dest == stdoutDestination {
		_, err := io.Copy(f.IOStreams.Out, download.Body)
		return err
	}

	target, err := resolveDownloadTarget(dest, filename)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		if err := confirm(f, fmt.Sprintf("%s already exists; overwrite?", target)); err != nil {
			return err
		}
	}
	written, err := writeDownload(target, download.Body)
	if err != nil {
		return err
	}
	return reportDownload(f, spec, download, target, written)
}

// resolveDownloadTarget turns the --out value into the concrete path to
// write: empty means the server's filename in the working directory, an
// existing directory means the server's filename inside it, and anything
// else is taken as the file to write.
func resolveDownloadTarget(dest, filename string) (string, error) {
	if dest == "" {
		return filename, nil
	}
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return filepath.Join(dest, filename), nil
	}
	return dest, nil
}

// writeDownload streams body into target, removing a partial file if the
// transfer fails so a truncated download is never left looking complete.
func writeDownload(target string, body io.Reader) (int64, error) {
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if copyErr != nil {
		os.Remove(target)
		return 0, copyErr
	}
	if closeErr != nil {
		os.Remove(target)
		return 0, closeErr
	}
	return written, nil
}

// reportDownload confirms what landed where: a JSON record under -o json, a
// one-line summary otherwise, and nothing under --quiet.
func reportDownload(f *cmdutil.Factory, spec downloadSpec, download *atlapi.Download, target string, written int64) error {
	if f.OutputFormat() == "json" {
		raw, err := json.Marshal(map[string]any{
			"id":        spec.id,
			"filename":  filepath.Base(target),
			"path":      target,
			"size":      written,
			"mediaType": download.ContentType,
		})
		if err != nil {
			return err
		}
		return writeResultJSON(f, raw)
	}
	if f.Quiet {
		return nil
	}
	_, err := fmt.Fprintf(f.IOStreams.Out, "Downloaded attachment %s to %s (%d bytes)\n", spec.id, target, written)
	return err
}

// destinationNeedsName reports whether the caller must resolve the
// attachment's filename before writing: a --out naming the file (or stdout)
// does not need one, an empty --out or a directory does.
func destinationNeedsName(dest string) bool {
	if dest == stdoutDestination {
		return false
	}
	if dest == "" {
		return true
	}
	info, err := os.Stat(dest)
	return err == nil && info.IsDir()
}

// firstNonEmpty returns the first non-empty value, the naming precedence
// every download follows: what the server said, then what the caller knew,
// then a synthetic fallback.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
