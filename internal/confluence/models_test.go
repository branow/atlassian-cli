package confluence

import "testing"

func TestParseSpaceList(t *testing.T) {
	raw := []byte(`{"results":[{"id":"123","key":"DS","name":"Demo","type":"global"},{"id":456,"key":"ENG","name":"Eng","type":"personal"}]}`)
	list, err := ParseSpaceList(raw)
	if err != nil {
		t.Fatalf("ParseSpaceList: %v", err)
	}
	if len(list.Results) != 2 {
		t.Fatalf("got %d spaces, want 2", len(list.Results))
	}
	// The second id arrives as a JSON number; FlexString normalizes it.
	if list.Results[1].ID.String() != "456" {
		t.Errorf("got id %q, want 456", list.Results[1].ID)
	}
}

func TestParsePageStorage(t *testing.T) {
	raw := []byte(`{"id":"1","title":"T","spaceId":"9","status":"current","version":{"number":7},"body":{"storage":{"value":"<p>x</p>","representation":"storage"}}}`)
	page, err := ParsePage(raw)
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	if page.Version.Number != 7 {
		t.Errorf("got version %d, want 7", page.Version.Number)
	}
	if page.StorageValue() != "<p>x</p>" {
		t.Errorf("got storage %q", page.StorageValue())
	}
}

func TestSearchResultRow(t *testing.T) {
	raw := []byte(`{"results":[
		{"content":{"id":"10","type":"page","title":"Hello","space":{"key":"DS"}}},
		{"title":"Fallback","resultGlobalContainer":{"title":"Some Space"}}
	]}`)
	results, err := ParseSearch(raw)
	if err != nil {
		t.Fatalf("ParseSearch: %v", err)
	}
	id, typ, title, space := results.Results[0].Row()
	if id != "10" || typ != "page" || title != "Hello" || space != "DS" {
		t.Errorf("got %q/%q/%q/%q", id, typ, title, space)
	}
	// The second hit has no content: title and space fall back.
	_, _, title2, space2 := results.Results[1].Row()
	if title2 != "Fallback" || space2 != "Some Space" {
		t.Errorf("got fallback title %q space %q", title2, space2)
	}
}
