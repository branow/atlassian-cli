package atlapi_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/branow/atlassian-cli/internal/atlapi"
)

// TestDownloadStreamsBytesAndFilename covers the plain case the JSON client
// could not serve: a 2xx response whose body is not JSON comes back as bytes
// plus the name the server gave it.
func TestDownloadStreamsBytesAndFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "*/*" {
			t.Errorf("Accept = %q, want */*", got)
		}
		w.Header().Set("Content-Type", "application/vnd.ms-excel")
		w.Header().Set("Content-Disposition", `attachment; filename="report.xlsx"`)
		w.Write([]byte("binary-bytes"))
	}))
	defer server.Close()

	dl, err := atlapi.New(server.URL, testCreds()).Download(context.Background(), "/rest/api/3/attachment/content/1")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer dl.Body.Close()

	body, err := io.ReadAll(dl.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "binary-bytes" {
		t.Errorf("body = %q", body)
	}
	if dl.Filename != "report.xlsx" {
		t.Errorf("Filename = %q, want report.xlsx", dl.Filename)
	}
	if dl.ContentType != "application/vnd.ms-excel" {
		t.Errorf("ContentType = %q", dl.ContentType)
	}
}

// TestDownloadFollowsRedirectAndDropsAuth is the security-relevant case: the
// attachment endpoint 303s to a presigned URL on another host, which
// authenticates by query string and must never receive the site's Basic
// credentials.
func TestDownloadFollowsRedirectAndDropsAuth(t *testing.T) {
	var presignedAuth string
	var presignedCalls int
	presigned := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presignedCalls++
		presignedAuth = r.Header.Get("Authorization")
		w.Write([]byte("redirected-bytes"))
	}))
	defer presigned.Close()

	var siteAuth string
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		siteAuth = r.Header.Get("Authorization")
		http.Redirect(w, r, presigned.URL+"/media?token=presigned", http.StatusSeeOther)
	}))
	defer site.Close()

	dl, err := atlapi.New(site.URL, testCreds()).Download(context.Background(), "/rest/api/3/attachment/content/1")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer dl.Body.Close()

	body, _ := io.ReadAll(dl.Body)
	if string(body) != "redirected-bytes" {
		t.Errorf("body = %q, want the redirect target's bytes", body)
	}
	if presignedCalls != 1 {
		t.Errorf("presigned host called %d times, want 1", presignedCalls)
	}
	if siteAuth == "" {
		t.Error("the Atlassian host must receive the Basic credentials")
	}
	if presignedAuth != "" {
		t.Errorf("credentials leaked to the redirect target: %q", presignedAuth)
	}
}

// TestDownloadKeepsAuthOnSameHostRedirect guards the other half of the
// redirect rule: a hop that stays on the site still needs to authenticate.
func TestDownloadKeepsAuthOnSameHostRedirect(t *testing.T) {
	var secondAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/first", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/second", http.StatusSeeOther)
	})
	mux.HandleFunc("/second", func(w http.ResponseWriter, r *http.Request) {
		secondAuth = r.Header.Get("Authorization")
		w.Write([]byte("ok"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	dl, err := atlapi.New(server.URL, testCreds()).Download(context.Background(), "/first")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	dl.Body.Close()
	if secondAuth == "" {
		t.Error("a same-host redirect must keep the credentials")
	}
}

// TestDownloadErrorBecomesAPIError keeps a failed download on the same exit
// code path as every other call instead of writing an error body to the
// destination file.
func TestDownloadErrorBecomesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["attachment not found"]}`))
	}))
	defer server.Close()

	_, err := atlapi.New(server.URL, testCreds()).Download(context.Background(), "/rest/api/3/attachment/content/404")
	var apiErr *atlapi.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %T (%v), want *atlapi.APIError", err, err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Message != "attachment not found" {
		t.Errorf("got %d %q", apiErr.Status, apiErr.Message)
	}
}

func TestFilenameFromDisposition(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"quoted filename", `attachment; filename="report.xlsx"`, "report.xlsx"},
		{"bare filename", `attachment; filename=notes.txt`, "notes.txt"},
		{"rfc 5987 encoding", `attachment; filename*=UTF-8''r%C3%A9sum%C3%A9.pdf`, "résumé.pdf"},
		{"path is reduced to its base", `attachment; filename="/etc/passwd"`, "passwd"},
		{"parent traversal is rejected", `attachment; filename="../../etc/passwd"`, "passwd"},
		{"windows separators are stripped", `attachment; filename="..\\..\\secret.txt"`, "secret.txt"},
		{"dot-dot alone is rejected", `attachment; filename=".."`, ""},
		{"no filename parameter", `attachment`, ""},
		{"absent header", "", ""},
		{"unparseable header", `attachment; filename=`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := atlapi.FilenameFromDisposition(tc.header); got != tc.want {
				t.Errorf("FilenameFromDisposition(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}
