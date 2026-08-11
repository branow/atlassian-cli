package output

import (
	"encoding/json"
	"io"
)

// WriteJSON pretty-prints v as indented JSON. Struct field order (or, for
// maps, encoding/json's sorted key order) gives stable, deterministic
// output.
func WriteJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}
