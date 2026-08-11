package catalog_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/branow/atlassian-cli/internal/atlapi/catalog"
)

func TestLookupMatchesCaseInsensitively(t *testing.T) {
	for _, id := range []string{"getProject", "getproject", "GETPROJECT", "GetProject"} {
		op, ok := catalog.Lookup(id)
		if !ok {
			t.Errorf("Lookup(%q) not found, want the getProject operation", id)
			continue
		}
		if op.Product != "jira" || op.Method != http.MethodGet || op.Path != "/rest/api/3/project/{projectIdOrKey}" {
			t.Errorf("Lookup(%q) = %s %s %s, want jira GET /rest/api/3/project/{projectIdOrKey}", id, op.Product, op.Method, op.Path)
		}
	}
}

func TestLookupUnknownOperation(t *testing.T) {
	if _, ok := catalog.Lookup("noSuchOperation"); ok {
		t.Error("Lookup(noSuchOperation) found an operation, want not found")
	}
	if prods := catalog.Products("noSuchOperation"); len(prods) != 0 {
		t.Errorf("Products(noSuchOperation) = %v, want empty", prods)
	}
}

// getIssue exists in both jira and jira-software; the top-level Lookup must
// report it as not-found so callers steer the user to the namespaced form.
func TestLookupAmbiguousOperationAcrossProducts(t *testing.T) {
	if _, ok := catalog.Lookup("getIssue"); ok {
		t.Error("Lookup(getIssue) resolved, want not found (ambiguous across products)")
	}
	prods := catalog.Products("getIssue")
	if len(prods) != 2 || prods[0] != "jira" || prods[1] != "jira-software" {
		t.Errorf("Products(getIssue) = %v, want [jira jira-software]", prods)
	}
}

func TestLookupInResolvesWithinNamespace(t *testing.T) {
	op, ok := catalog.LookupIn("confluence", "createPage")
	if !ok {
		t.Fatal("LookupIn(confluence, createPage) not found")
	}
	if op.Product != "confluence-v2" || op.Method != http.MethodPost {
		t.Errorf("got %s %s, want confluence-v2 POST", op.Product, op.Method)
	}

	// getProject is a jira operation; it must not resolve in the confluence
	// namespace.
	if _, ok := catalog.LookupIn("confluence", "getProject"); ok {
		t.Error("LookupIn(confluence, getProject) resolved, want not found")
	}
}

// A handful of ids collide within a single namespace (getIssue is in both
// jira and jira-software); LookupIn must report those as not-found too.
func TestLookupInAmbiguousWithinNamespace(t *testing.T) {
	if _, ok := catalog.LookupIn("jira", "getIssue"); ok {
		t.Error("LookupIn(jira, getIssue) resolved, want not found (collides within namespace)")
	}
	if prods := catalog.ProductsIn("jira", "getIssue"); len(prods) != 2 {
		t.Errorf("ProductsIn(jira, getIssue) = %v, want two products", prods)
	}
	// A concrete single product still resolves that same id unambiguously.
	if _, ok := catalog.LookupIn("jira-software", "getIssue"); !ok {
		t.Error("LookupIn(jira-software, getIssue) not found, want the jira-software binding")
	}
}

func TestOperationsCountAndConsistency(t *testing.T) {
	ids := catalog.Operations()
	if got, want := len(ids), 1054; got != want {
		t.Errorf("got %d unique operationIds, want %d", got, want)
	}
	// Sorted and de-duplicated.
	for i := 1; i < len(ids); i++ {
		if ids[i-1] >= ids[i] {
			t.Fatalf("Operations() not strictly sorted at %q, %q", ids[i-1], ids[i])
		}
	}
}

func TestOperationsInNamespaceCounts(t *testing.T) {
	if got, want := len(catalog.OperationsIn("jira")), 719; got != want {
		t.Errorf("OperationsIn(jira) = %d, want %d", got, want)
	}
	if got, want := len(catalog.OperationsIn("confluence")), 346; got != want {
		t.Errorf("OperationsIn(confluence) = %d, want %d", got, want)
	}
}

// Every unambiguous operation must have a known method and a well-formed
// path (leading slash, {param} placeholders allowed). Guards against a
// distiller bug leaking malformed paths into the embedded catalog.
func TestCataloguedOperationsAreWellFormed(t *testing.T) {
	pathShape := regexp.MustCompile(`^(/[A-Za-z0-9._{}-]+)+$`)
	methods := map[string]bool{
		http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
		http.MethodPatch: true, http.MethodDelete: true,
	}
	for _, id := range catalog.Operations() {
		op, ok := catalog.Lookup(id)
		if !ok {
			// Ambiguous across products; validated via its products instead.
			for _, p := range catalog.Products(id) {
				if len(p) == 0 {
					t.Errorf("operation %s reports an empty product", id)
				}
			}
			continue
		}
		if !methods[op.Method] {
			t.Errorf("operation %s has unknown method %q", id, op.Method)
		}
		if !pathShape.MatchString(op.Path) {
			t.Errorf("operation %s has a malformed path %q", id, op.Path)
		}
	}
}

func TestLookupTypeResolvesComponentSchemas(t *testing.T) {
	fields, ok := catalog.LookupType("IssueUpdateDetails")
	if !ok || len(fields) == 0 {
		t.Fatalf("LookupType(IssueUpdateDetails) = %v, %v; want a non-empty definition", fields, ok)
	}
	if _, ok := catalog.LookupType("issueupdatedetails"); !ok {
		t.Error("LookupType should match type names case-insensitively")
	}
	if _, ok := catalog.LookupType("NotARealType"); ok {
		t.Error("LookupType returned a definition for an unknown type")
	}
}

func TestCanonicalID(t *testing.T) {
	if got := catalog.CanonicalID("getproject"); got != "getProject" {
		t.Errorf("CanonicalID(getproject) = %q, want getProject", got)
	}
	if got := catalog.CanonicalID("stillUnknown"); got != "stillUnknown" {
		t.Errorf("CanonicalID(stillUnknown) = %q, want it unchanged", got)
	}
}

// findField is a small helper mirroring how field types are compared.
func findField(fields []catalog.Field, name string) (catalog.Field, bool) {
	for _, f := range fields {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return catalog.Field{}, false
}

func TestOperationFieldsCarryMetadata(t *testing.T) {
	op, ok := catalog.Lookup("getProject")
	if !ok {
		t.Fatal("Lookup(getProject) not found")
	}
	pp, ok := findField(op.PathParams, "projectIdOrKey")
	if !ok || pp.In != "path" || !pp.Required {
		t.Errorf("getProject path param projectIdOrKey = %+v, want in=path required=true", pp)
	}
}
