package jira

// Jira's edit/create API rejects a bare string for its object-shaped system
// fields: `priority=High` must be sent as {"priority":{"name":"High"}}, not
// the JSON string "High". CoerceFields wraps a scalar -f value into the object
// shape the API expects for the well-known fields below, so callers can write
// `-f priority=High` instead of `-f 'priority={"name":"High"}'`. A value the
// user already spelled as an object or array is left untouched, so the explicit
// JSON form keeps working and unknown/custom fields pass through verbatim.

// objectFieldKey maps a system field whose value is a single object to the
// property a bare value fills. These wrap by name/key/accountId; to target one
// by id instead, pass the explicit JSON form, e.g. -f 'priority={"id":"3"}'.
var objectFieldKey = map[string]string{
	"priority":  "name",
	"assignee":  "accountId",
	"reporter":  "accountId",
	"parent":    "key",
	"project":   "key",
	"issuetype": "name",
}

// arrayObjectFieldKey maps a system field whose value is an array of objects to
// the property identifying each element. A single scalar value becomes a
// one-element array.
var arrayObjectFieldKey = map[string]string{
	"components":  "name",
	"versions":    "name",
	"fixVersions": "name",
}

// CoerceFields rewrites bare scalar values for well-known Jira object fields
// into the object/array shape the REST API requires, mutating fields in place.
// Values that are already objects or arrays (e.g. a user-supplied JSON literal)
// and fields not listed above are left unchanged.
func CoerceFields(fields map[string]any) {
	for name, value := range fields {
		s, isScalar := scalarValue(value)
		if !isScalar {
			continue
		}
		switch {
		case name == "labels":
			fields[name] = []any{s}
		case objectFieldKey[name] != "":
			fields[name] = map[string]any{objectFieldKey[name]: s}
		case arrayObjectFieldKey[name] != "":
			fields[name] = []any{map[string]any{arrayObjectFieldKey[name]: s}}
		}
	}
}

// scalarValue reports whether v is a plain string suitable for wrapping and, if
// so, returns it. Numbers, booleans, objects, and arrays are left to the caller
// untouched: only a bare string is the ambiguous case CoerceFields resolves.
func scalarValue(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}
