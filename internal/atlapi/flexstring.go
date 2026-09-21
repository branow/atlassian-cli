package atlapi

import "strings"

// FlexString decodes a JSON value Atlassian emits as a string in one place
// and a number in another — ids are the usual culprits. Confluence v2 quotes
// content and space ids while some v1 shapes do not, and Jira returns an
// attachment id as a string inside an issue's fields.attachment but as a
// number from the attachment endpoint. FlexString normalizes both to a Go
// string so callers never have to care which the server chose.
type FlexString string

// UnmarshalJSON accepts a JSON string or number (or null) and stores its
// textual form.
func (f *FlexString) UnmarshalJSON(b []byte) error {
	text := strings.TrimSpace(string(b))
	if text == "null" {
		*f = ""
		return nil
	}
	*f = FlexString(strings.Trim(text, `"`))
	return nil
}

// String returns the value as a plain Go string.
func (f FlexString) String() string { return string(f) }
