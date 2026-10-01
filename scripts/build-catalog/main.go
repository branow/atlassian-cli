// Command build-catalog distills the embedded operation catalog from the
// Atlassian OpenAPI specs in specs/ into
// internal/atlapi/catalog/atlas-catalog.json. Run it via
// `go run ./scripts/build-catalog` (or `make catalog`).
//
// It is deliberately dumb: it reads each spec generically as JSON, walks
// paths/operations, and records each operation's method, full post-host
// path, path/query parameters, flattened request-body and success-response
// properties, plus a per-product registry of the component schemas those
// fields reference (one level of $ref following, transitively closed with a
// cycle guard). The registry is keyed by product because two products can
// define the same schema name with different shapes -- Jira v2's Comment
// body is a wiki-markup string where v3's is an ADF document -- and a flat
// registry would let whichever spec is read first describe both. The client
// stays schema-agnostic; all product knowledge lives in the generated JSON.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// source pairs a spec file with the product name its operations belong to.
type source struct {
	file    string
	product string
}

var sources = []source{
	{"jira-cloud.v3.json", "jira"},
	{"jira-cloud.v2.json", "jira-v2"},
	{"jira-software.v3.json", "jira-software"},
	{"confluence-cloud.v1.json", "confluence-v1"},
	{"confluence-cloud.v2.json", "confluence-v2"},
}

// httpMethods is the set of path-item keys that denote an operation.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true,
	"delete": true, "patch": true, "options": true, "head": true,
}

// Field mirrors catalog.Field. Body/type fields omit In.
type Field struct {
	Name     string `json:"name"`
	In       string `json:"in,omitempty"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// Operation mirrors catalog.Operation.
type Operation struct {
	ID         string  `json:"id"`
	Product    string  `json:"product"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Summary    string  `json:"summary,omitempty"`
	Deprecated bool    `json:"deprecated,omitempty"`
	PathParams []Field `json:"pathParams,omitempty"`
	Query      []Field `json:"query,omitempty"`
	Body       []Field `json:"body,omitempty"`
	Response   []Field `json:"response,omitempty"`
}

// catalogFile mirrors catalog.catalogFile.
type catalogFile struct {
	Operations []Operation                   `json:"operations"`
	Types      map[string]map[string][]Field `json:"types,omitempty"`
}

func main() {
	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	specsDir := filepath.Join(root, "specs")
	outPath := filepath.Join(root, "internal", "atlapi", "catalog", "atlas-catalog.json")

	var operations []Operation
	types := map[string]map[string][]Field{}

	for _, src := range sources {
		spec, err := readSpec(filepath.Join(specsDir, src.file))
		if err != nil {
			fatal(fmt.Errorf("%s: %w", src.file, err))
		}
		comps := components(spec)
		paramComps := namedComponents(spec, "parameters")
		bodyComps := namedComponents(spec, "requestBodies")
		ops := distill(spec, src.product, comps, paramComps, bodyComps)
		operations = append(operations, ops...)
		productTypes := map[string][]Field{}
		collectTypes(ops, comps, productTypes)
		types[src.product] = productTypes
		fmt.Printf("%-16s %4d operations  %4d types\n", src.product, len(ops), len(productTypes))
	}

	// Stable, deterministic order: by product then operationId.
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Product != operations[j].Product {
			return operations[i].Product < operations[j].Product
		}
		return operations[i].ID < operations[j].ID
	})

	out := catalogFile{Operations: operations, Types: types}
	if err := writeJSON(outPath, out); err != nil {
		fatal(err)
	}
	total := 0
	for _, productTypes := range types {
		total += len(productTypes)
	}
	fmt.Printf("wrote %d operations, %d types across %d products -> %s\n", len(operations), total, len(types), outPath)
}

// distill turns one parsed spec into its product's operations, deduping
// operationIds within the spec (last wins).
func distill(spec map[string]any, product string, comps, paramComps, bodyComps map[string]any) []Operation {
	base := basePath(spec)
	paths, _ := spec["paths"].(map[string]any)

	byID := map[string]Operation{}
	var order []string
	// Iterate paths (and each item's methods) in a stable order so a spec
	// with duplicate operationIds resolves the "last wins" dedup the same
	// way on every run — map iteration order in Go is randomized.
	for _, rawPath := range sortedKeys(paths) {
		item, ok := paths[rawPath].(map[string]any)
		if !ok {
			continue
		}
		sharedParams, _ := item["parameters"].([]any)
		for _, method := range sortedKeys(item) {
			if !httpMethods[strings.ToLower(method)] {
				continue
			}
			op, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			id, _ := op["operationId"].(string)
			if id == "" {
				continue
			}
			built := buildOperation(id, product, method, base+rawPath, op, sharedParams, comps, paramComps, bodyComps)
			if _, seen := byID[id]; !seen {
				order = append(order, id)
			}
			byID[id] = built
		}
	}

	out := make([]Operation, 0, len(byID))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

// buildOperation assembles one Operation from its raw spec node, merging
// path-item-level parameters with the operation's own.
func buildOperation(id, product, method, fullPath string, op map[string]any, sharedParams []any, comps, paramComps, bodyComps map[string]any) Operation {
	result := Operation{
		ID:         id,
		Product:    product,
		Method:     strings.ToUpper(method),
		Path:       fullPath,
		Summary:    str(op["summary"]),
		Deprecated: boolVal(op["deprecated"]),
	}

	seen := map[string]bool{}
	addParam := func(p map[string]any) {
		// A parameter may be a $ref into components.parameters (common in
		// confluence-v1); resolve it before reading name/in, else the whole
		// parameter is silently dropped.
		if ref := str(p["$ref"]); ref != "" {
			if resolved := mapOf(paramComps[refName(ref)]); resolved != nil {
				p = resolved
			}
		}
		name := str(p["name"])
		in := str(p["in"])
		if name == "" || in == "" {
			return // e.g. confluence v1's bogus undefined:undefined param
		}
		key := in + ":" + name
		if seen[key] {
			return
		}
		seen[key] = true
		f := Field{Name: name, In: in, Required: boolVal(p["required"]), Type: schemaType(mapOf(p["schema"]), comps)}
		switch in {
		case "path":
			result.PathParams = append(result.PathParams, f)
		case "query":
			result.Query = append(result.Query, f)
		}
	}
	// Operation params take precedence over shared ones on name/in clash.
	for _, rp := range params(op["parameters"]) {
		addParam(rp)
	}
	for _, rp := range params(sharedParams) {
		addParam(rp)
	}
	sortFields(result.PathParams)
	sortFields(result.Query)

	result.Body = bodyFields(op["requestBody"], comps, bodyComps)
	result.Response = responseFields(op["responses"], comps)
	return result
}

// bodyFields flattens the JSON request-body schema's top-level properties.
// A requestBody may itself be a $ref into components.requestBodies (common
// in confluence-v2's write operations); resolve it first, else the body
// fields come back empty and --describe and canonicalization are broken.
func bodyFields(rawBody any, comps, bodyComps map[string]any) []Field {
	body, ok := rawBody.(map[string]any)
	if !ok {
		return nil
	}
	if ref := str(body["$ref"]); ref != "" {
		if resolved := mapOf(bodyComps[refName(ref)]); resolved != nil {
			body = resolved
		}
	}
	schema := jsonSchema(body["content"])
	if schema == nil {
		return nil
	}
	return flatten(schema, comps)
}

// responseFields flattens the first available success response's JSON
// schema (200 then 201 then any 2xx).
func responseFields(rawResponses any, comps map[string]any) []Field {
	responses, ok := rawResponses.(map[string]any)
	if !ok {
		return nil
	}
	for _, code := range successCodes(responses) {
		resp, ok := responses[code].(map[string]any)
		if !ok {
			continue
		}
		if schema := jsonSchema(resp["content"]); schema != nil {
			return flatten(schema, comps)
		}
	}
	return nil
}

// successCodes returns the 2xx response codes present, 200/201 first.
func successCodes(responses map[string]any) []string {
	var out []string
	for _, preferred := range []string{"200", "201"} {
		if _, ok := responses[preferred]; ok {
			out = append(out, preferred)
		}
	}
	var rest []string
	for code := range responses {
		if strings.HasPrefix(code, "2") && code != "200" && code != "201" {
			rest = append(rest, code)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// jsonSchema returns the application/json schema node from a content map.
func jsonSchema(rawContent any) map[string]any {
	content, ok := rawContent.(map[string]any)
	if !ok {
		return nil
	}
	media, ok := content["application/json"].(map[string]any)
	if !ok {
		return nil
	}
	return mapOf(media["schema"])
}

// flatten resolves a schema (following one $ref and merging allOf) and
// returns its top-level properties as sorted Fields.
func flatten(schema map[string]any, comps map[string]any) []Field {
	props, required := properties(schema, comps, map[string]bool{})
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Field, 0, len(names))
	for _, name := range names {
		out = append(out, Field{
			Name:     name,
			Type:     schemaType(props[name], comps),
			Required: required[name],
		})
	}
	return out
}

// properties collects a schema's top-level properties and required set,
// dereferencing a single $ref and merging allOf members. seen guards
// against $ref cycles.
func properties(schema map[string]any, comps map[string]any, seen map[string]bool) (map[string]map[string]any, map[string]bool) {
	props := map[string]map[string]any{}
	required := map[string]bool{}
	if schema == nil {
		return props, required
	}
	if ref := str(schema["$ref"]); ref != "" {
		name := refName(ref)
		if seen[name] {
			return props, required
		}
		seen[name] = true
		return properties(mapOf(comps[name]), comps, seen)
	}
	for _, r := range asStrings(schema["required"]) {
		required[r] = true
	}
	if p, ok := schema["properties"].(map[string]any); ok {
		for name, v := range p {
			props[name] = mapOf(v)
		}
	}
	for _, member := range asMaps(schema["allOf"]) {
		mp, mr := properties(member, comps, seen)
		for name, v := range mp {
			props[name] = v
		}
		for name := range mr {
			required[name] = true
		}
	}
	return props, required
}

// schemaType renders a field schema's type: a referenced component name, an
// "X[]" array notation, or the scalar type/format.
func schemaType(schema map[string]any, comps map[string]any) string {
	if schema == nil {
		return ""
	}
	if ref := str(schema["$ref"]); ref != "" {
		return refName(ref)
	}
	if str(schema["type"]) == "array" {
		inner := schemaType(mapOf(schema["items"]), comps)
		if inner == "" {
			return "array"
		}
		return inner + "[]"
	}
	if len(asMaps(schema["allOf"])) == 1 {
		// A lone allOf member is a common way to reference a component.
		if ref := str(asMaps(schema["allOf"])[0]["$ref"]); ref != "" {
			return refName(ref)
		}
	}
	if format := str(schema["format"]); format != "" {
		return format
	}
	return str(schema["type"])
}

// collectTypes records the definition of every component schema referenced
// by the operations' fields, transitively, guarding against cycles.
func collectTypes(ops []Operation, comps map[string]any, types map[string][]Field) {
	var queue []string
	enqueue := func(fields []Field) {
		for _, f := range fields {
			base := strings.TrimSuffix(f.Type, "[]")
			if _, ok := comps[base]; ok {
				queue = append(queue, base)
			}
		}
	}
	for _, op := range ops {
		enqueue(op.PathParams)
		enqueue(op.Query)
		enqueue(op.Body)
		enqueue(op.Response)
	}
	for len(queue) > 0 {
		name := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if _, done := types[name]; done {
			continue
		}
		comp, ok := comps[name].(map[string]any)
		if !ok {
			continue
		}
		fields := flatten(comp, comps)
		types[name] = fields
		enqueue(fields)
	}
}

// basePath returns the server URL's path portion (scheme+host stripped),
// which is prepended to every operation path so the client can treat Path
// as the full post-host path. Handles the templated confluence-v2 host.
func basePath(spec map[string]any) string {
	servers, ok := spec["servers"].([]any)
	if !ok || len(servers) == 0 {
		return ""
	}
	server, ok := servers[0].(map[string]any)
	if !ok {
		return ""
	}
	url := str(server["url"])
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "//")
	if i := strings.Index(url, "/"); i >= 0 {
		return strings.TrimRight(url[i:], "/")
	}
	return ""
}

func components(spec map[string]any) map[string]any {
	return namedComponents(spec, "schemas")
}

// namedComponents returns one components sub-map (schemas, parameters, or
// requestBodies), keyed by component name, or an empty map when absent.
func namedComponents(spec map[string]any, kind string) map[string]any {
	c, _ := spec["components"].(map[string]any)
	if c == nil {
		return map[string]any{}
	}
	m, _ := c[kind].(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

// sortedKeys returns a map's keys in sorted order, for deterministic
// iteration over spec paths and path-item methods.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func refName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

func params(raw any) []map[string]any { return asMaps(raw) }

func asMaps(raw any) []map[string]any {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, v := range list {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func asStrings(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mapOf(raw any) map[string]any {
	m, _ := raw.(map[string]any)
	return m
}

func str(raw any) string {
	s, _ := raw.(string)
	return s
}

func boolVal(raw any) bool {
	b, _ := raw.(bool)
	return b
}

func sortFields(fields []Field) {
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
}

func readSpec(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// writeJSON writes v as indented, newline-terminated JSON. HTML escaping is
// disabled so paths and descriptions stay readable.
func writeJSON(path string, v any) error {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// repoRoot walks up from the working directory to the module root (the
// directory containing go.mod), so the tool works regardless of where
// `go run` is invoked from.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod above %s", dir)
		}
		dir = parent
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "build-catalog:", err)
	os.Exit(1)
}
