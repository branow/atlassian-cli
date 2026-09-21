package atlapi

import (
	"net/http"
	"testing"
)

func TestParseResponseFlagsRateLimited(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header http.Header
		want   bool
	}{
		{"429 is rate-limited", 429, nil, true},
		{"503 with Retry-After is rate-limited", 503, http.Header{"Retry-After": {"5"}}, true},
		{"503 without Retry-After is not", 503, nil, false},
		{"503 suspended-site body is not", 503, http.Header{}, false},
		{"500 is not", 500, nil, false},
		{"400 is not", 400, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseResponse(tc.status, tc.header, []byte(`{"message":"nope"}`))
			apiErr, ok := err.(*APIError)
			if !ok {
				t.Fatalf("got %T, want *APIError", err)
			}
			if apiErr.RateLimited != tc.want {
				t.Errorf("RateLimited = %v, want %v", apiErr.RateLimited, tc.want)
			}
		})
	}
}

// TestParseResponseKeepsNonJSONBodyAsBinary is the fix for attachment
// downloads through the generic api path: a 2xx body the server never
// claimed was JSON is data, not a decode failure.
func TestParseResponseKeepsNonJSONBodyAsBinary(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		wantBinary  bool
		wantErr     bool
	}{
		{"spreadsheet bytes", "application/vnd.ms-excel", "PK\x03\x04binary", true, false},
		{"png bytes", "image/png", "\x89PNG\r\n", true, false},
		{"plain text", "text/plain; charset=utf-8", "not json", true, false},
		{"json content type with broken body is an error", "application/json", "{not json", false, true},
		{"undeclared broken body is an error", "", "{not json", false, true},
		{"json is decoded as before", "application/json;charset=UTF-8", `{"id":"1"}`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header := http.Header{}
			if tc.contentType != "" {
				header.Set("Content-Type", tc.contentType)
			}
			resp, err := parseResponse(http.StatusOK, header, []byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error for a body that claims to be JSON and is not")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseResponse: %v", err)
			}
			if resp.Binary != tc.wantBinary {
				t.Errorf("Binary = %v, want %v", resp.Binary, tc.wantBinary)
			}
			if string(resp.Raw) != tc.body {
				t.Errorf("Raw = %q, want the untouched body", resp.Raw)
			}
		})
	}
}
