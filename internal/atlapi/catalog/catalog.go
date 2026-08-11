// Package catalog maps Atlassian operationIds to their REST endpoints. The
// embedded catalog is distilled from the published OpenAPI specs by
// scripts/build-catalog; regenerate it there (make catalog) rather than
// editing atlas-catalog.json by hand.
//
// One operationId can appear in more than one product (e.g. getIssue in
// both jira and jira-software, getUser in both jira and confluence-v1), so
// lookups distinguish a top-level view (all products) from a per-namespace
// view. A top-level Lookup of an id present in several products is
// reported as not-found so callers can steer the user to the namespaced
// form (atl jira api / atl confluence api).
package catalog

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed atlas-catalog.json
var catalogJSON []byte

// Field is one path, query, body, or response field of an operation.
type Field struct {
	// Name is the field name in its documented (wire) casing.
	Name string `json:"name"`
	// In is where the field travels: "path", "query", or "body". Body
	// fields omit it.
	In string `json:"in,omitempty"`
	// Type is the OpenAPI type/format, an "X[]" array notation, or a
	// referenced component-schema name, empty when not documented.
	Type string `json:"type,omitempty"`
	// Required marks required request fields; meaningful for request
	// fields only.
	Required bool `json:"required,omitempty"`
}

// Operation describes one operationId's REST binding within a product.
type Operation struct {
	// ID is the operationId in its canonical (documented) casing.
	ID string `json:"id"`
	// Product is one of "jira", "jira-software", "confluence-v1",
	// "confluence-v2".
	Product string `json:"product"`
	// Method is the upper-case HTTP method.
	Method string `json:"method"`
	// Path is the full post-host path (the client prepends
	// https://{site}); path params appear as {name}.
	Path string `json:"path"`
	// Summary is a short human description, when the spec provides one.
	Summary string `json:"summary,omitempty"`
	// Deprecated marks operations the spec flags as deprecated.
	Deprecated bool `json:"deprecated,omitempty"`
	// PathParams lists the {name} parameters embedded in Path.
	PathParams []Field `json:"pathParams,omitempty"`
	// Query lists the documented query parameters.
	Query []Field `json:"query,omitempty"`
	// Body lists the flattened top-level request-body schema properties.
	Body []Field `json:"body,omitempty"`
	// Response lists the flattened top-level success-response properties.
	Response []Field `json:"response,omitempty"`
}

// catalogFile is the on-disk shape of atlas-catalog.json: the operations as
// a flat list (one entry per product binding) plus a registry of component
// schemas used to expand nested field shapes.
type catalogFile struct {
	Operations []Operation        `json:"operations"`
	Types      map[string][]Field `json:"types,omitempty"`
}

// productGroups maps a namespace (as used by "atl jira api" / "atl
// confluence api") to the concrete products it spans. A namespace that is
// itself a concrete product name resolves to just that product.
var productGroups = map[string][]string{
	"jira":       {"jira", "jira-software"},
	"confluence": {"confluence-v1", "confluence-v2"},
}

var (
	byLowerID   map[string][]Operation
	canonicalID map[string]string
	allIDs      []string
	typeDefs    map[string][]Field
)

func init() {
	var file catalogFile
	if err := json.Unmarshal(catalogJSON, &file); err != nil {
		panic("atlapi/catalog: embedded atlas-catalog.json is invalid: " + err.Error())
	}
	byLowerID = make(map[string][]Operation, len(file.Operations))
	canonicalID = make(map[string]string, len(file.Operations))
	idSet := make(map[string]struct{})
	for _, op := range file.Operations {
		lower := strings.ToLower(op.ID)
		byLowerID[lower] = append(byLowerID[lower], op)
		canonicalID[lower] = op.ID
		idSet[op.ID] = struct{}{}
	}
	allIDs = make([]string, 0, len(idSet))
	for id := range idSet {
		allIDs = append(allIDs, id)
	}
	sort.Strings(allIDs)
	typeDefs = make(map[string][]Field, len(file.Types))
	for name, fields := range file.Types {
		typeDefs[strings.ToLower(name)] = fields
	}
}

// productsFor returns the concrete products a namespace spans, or the
// namespace itself when it names a single product.
func productsFor(namespace string) []string {
	if group, ok := productGroups[strings.ToLower(namespace)]; ok {
		return group
	}
	return []string{namespace}
}

// Lookup returns the operation for id in the top-level (all-products) view,
// matched case-insensitively. It reports not-found both for an unknown id
// and for one that is ambiguous across products; use Products to tell the
// two apart and LookupIn for a namespaced resolution.
func Lookup(id string) (Operation, bool) {
	ops := byLowerID[strings.ToLower(id)]
	if len(ops) == 1 {
		return ops[0], true
	}
	return Operation{}, false
}

// LookupIn returns the operation for id restricted to a namespace's
// products, matched case-insensitively. It reports not-found for an
// unknown id or one still ambiguous within the namespace (a handful of ids
// collide across a namespace's product versions); use ProductsIn to
// distinguish.
func LookupIn(namespace, id string) (Operation, bool) {
	matches := matchesIn(namespace, id)
	if len(matches) == 1 {
		return matches[0], true
	}
	return Operation{}, false
}

// LookupProduct returns the operation for id defined by an exact product
// (e.g. "jira-software"), matched case-insensitively on both product and id.
// It pins one side of an operationId that several products share, the escape
// hatch for ids that collide even within a single namespace group.
func LookupProduct(product, id string) (Operation, bool) {
	for _, op := range byLowerID[strings.ToLower(id)] {
		if strings.EqualFold(op.Product, product) {
			return op, true
		}
	}
	return Operation{}, false
}

// matchesIn returns every operation for id within a namespace's products.
func matchesIn(namespace, id string) []Operation {
	want := map[string]bool{}
	for _, p := range productsFor(namespace) {
		want[p] = true
	}
	var out []Operation
	for _, op := range byLowerID[strings.ToLower(id)] {
		if want[op.Product] {
			out = append(out, op)
		}
	}
	return out
}

// Products returns the distinct products defining id (top-level view),
// sorted. An empty result means the id is unknown; more than one means it
// is ambiguous and should be reached via a namespace.
func Products(id string) []string {
	return distinctProducts(byLowerID[strings.ToLower(id)])
}

// ProductsIn returns the distinct products defining id within a namespace,
// sorted.
func ProductsIn(namespace, id string) []string {
	return distinctProducts(matchesIn(namespace, id))
}

func distinctProducts(ops []Operation) []string {
	seen := map[string]bool{}
	var out []string
	for _, op := range ops {
		if !seen[op.Product] {
			seen[op.Product] = true
			out = append(out, op.Product)
		}
	}
	sort.Strings(out)
	return out
}

// Operations returns every operationId in canonical casing, sorted and
// de-duplicated across products, for listing and shell completion.
func Operations() []string {
	return allIDs
}

// OperationsIn returns the operationIds available within a namespace, in
// canonical casing, sorted and de-duplicated.
func OperationsIn(namespace string) []string {
	want := map[string]bool{}
	for _, p := range productsFor(namespace) {
		want[p] = true
	}
	idSet := map[string]struct{}{}
	for _, ops := range byLowerID {
		for _, op := range ops {
			if want[op.Product] {
				idSet[op.ID] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(idSet))
	for id := range idSet {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// LookupType returns the field list defining a component schema (e.g.
// "IssueUpdateDetails"), matched case-insensitively, so a field typed as a
// component can be expanded into its own fields. Opaque or unreferenced
// types are absent.
func LookupType(name string) ([]Field, bool) {
	fields, ok := typeDefs[strings.ToLower(name)]
	return fields, ok
}

// CanonicalID returns the documented casing for id, matched
// case-insensitively; unknown ids pass through unchanged.
func CanonicalID(id string) string {
	if name, ok := canonicalID[strings.ToLower(id)]; ok {
		return name
	}
	return id
}
