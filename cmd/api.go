package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/atlapi/catalog"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/output"
)

// apiScope is one namespace's command surface for the generic invoker: the
// name it is invoked under, how its product reads in help text, the closing
// paragraph that applies only to that scope, and its examples.
type apiScope struct {
	use     string
	product string
	tail    string
	example string
}

// apiScopes describes every namespace newAPICmd can be built for. The
// namespace also scopes which products are searched: "" is the top-level
// view over every default product (and errors on cross-product ambiguity,
// pointing at a namespaced form), "jira" spans jira + jira-software,
// "confluence" spans the v1 + v2 REST APIs, and "jira-v2" is the opt-in
// Jira REST v2 surface reached as "atl jira api v2".
var apiScopes = map[string]apiScope{
	"": {
		use:     "api",
		product: "Atlassian",
		tail:    namespaceTail,
		example: `  atl api getIssue -f issueIdOrKey=TEST-1
  atl api createIssue --describe`,
	},
	"jira": {
		use:     "api",
		product: "Jira",
		tail:    namespaceTail,
		example: `  atl jira api createIssue --input issue.json
  atl jira api getIssue -f issueIdOrKey=TEST-1
  atl jira api --list`,
	},
	"confluence": {
		use:     "api",
		product: "Confluence",
		tail:    namespaceTail,
		example: `  atl confluence api getPageById -f id=12345
  atl confluence api --list`,
	},
	"jira-v2": {
		use:     "v2",
		product: "Jira REST v2",
		tail:    wikiMarkupTail,
		example: `  atl jira api v2 addComment --input comment.json
  atl jira api v2 addComment --describe
  atl jira api v2 --list`,
	},
}

// namespaceTail closes the help for the default scopes, where one
// operationId can resolve to several products.
const namespaceTail = `One operationId can exist in more than one product (e.g. getIssue in both
Jira and Jira Software). The top-level "atl api" reports such ids as
ambiguous and points to the namespaced form; "atl jira api" / "atl
confluence api" scope the search.`

// wikiMarkupTail closes the help for the v2 scope. v2 mirrors v3's
// operations at /rest/api/2 and takes wiki markup where v3 takes ADF, which
// is the reason to reach for it: Jira renders an attachment inline from its
// filename, images and video alike, with no ADF media node to hand-build.
const wikiMarkupTail = `Jira REST v2 mirrors v3's operations under /rest/api/2 and takes wiki
markup where v3 takes ADF, so rich text is a plain string:

  h2. Heading   *bold*   {code}...{code}
  !diagram.png|thumbnail!          attachment rendered inline
  !walkthrough.mp4!                attachment played inline

Jira resolves those by filename against the issue's own attachments, so
attach the file first (atl jira issue attach) and name it in the comment.
Because v2 shares almost every operationId with v3, it is not part of "atl
jira api" or "atl api"; reach it here or with --product jira-v2.`

// newAPICmd builds the generic invoker for a namespace: any catalogued
// Atlassian operation, without a bespoke subcommand per operation.
func newAPICmd(f *cmdutil.Factory, namespace string) *cobra.Command {
	var fields []string
	var inputFile string
	var listOperations bool
	var describe bool
	var productFlag string

	scope, ok := apiScopes[namespace]
	if !ok {
		panic("cmd: no api scope for namespace " + namespace)
	}

	cmd := &cobra.Command{
		Use:   scope.use + " <operation>",
		Short: fmt.Sprintf("Invoke a named %s API operation", scope.product),
		Long: fmt.Sprintf(`Invoke a named %s API operation against the active profile's site
(e.g. getIssue, createIssue, getPageById). The operationId routes to its
REST endpoint via the embedded catalog: path parameters are filled from
matching fields, and the remaining fields become query parameters for
GET/DELETE operations or a JSON request body otherwise. --list prints every
catalogued operationId in scope.

-f/--field values parse as JSON when they look like it (numbers, booleans,
null, arrays, objects) and as plain strings otherwise. --input reads a full
JSON object from a file (or "-" for stdin) — use it for nested request
bodies — and cannot be combined with -f/--field.

--describe fully documents an operation: its product, HTTP method, path, and
path/query/body/response fields — each with its type and, for request
fields, whether it is required, with nested component types expanded inline
(no credentials or network needed). Add -o json for a machine-readable shape.

%s`, scope.product, scope.tail),
		Args:    cobra.MaximumNArgs(1),
		Example: scope.example,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return operationsFor(namespace), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if listOperations {
				if len(args) != 0 {
					return &cmdutil.ValidationError{Message: "--list cannot be combined with an operation name"}
				}
				return runAPIList(f, namespace)
			}
			if len(args) != 1 {
				return &cmdutil.ValidationError{Message: "an operation name is required (run api --list to see all operations)"}
			}
			if describe {
				if len(fields) > 0 || inputFile != "" {
					return &cmdutil.ValidationError{Message: "--describe cannot be combined with --field or --input"}
				}
				return runAPIDescribe(f, namespace, productFlag, args[0])
			}
			return runAPI(cmd.Context(), f, namespace, productFlag, args[0], fields, inputFile)
		},
	}

	cmd.Flags().StringArrayVarP(&fields, "field", "f", nil, "request field as key=value (repeatable)")
	cmd.Flags().StringVar(&inputFile, "input", "", `read request fields as a JSON object from a file ("-" for stdin)`)
	cmd.Flags().BoolVar(&listOperations, "list", false, "list all catalogued operationIds in scope")
	cmd.Flags().BoolVar(&describe, "describe", false, "print the operation's method, path, and documented fields")
	cmd.Flags().StringVar(&productFlag, "product", "", "pin the product for an operationId defined in several (jira|jira-v2|jira-software|confluence-v1|confluence-v2)")
	return cmd
}

// newJiraAPICmd builds "atl jira api" with the opt-in v2 surface attached
// as a subcommand. v2 is a sibling scope rather than another product in the
// jira namespace because it mirrors nearly every v3 operationId, which
// would make the whole Jira catalog ambiguous.
func newJiraAPICmd(f *cmdutil.Factory) *cobra.Command {
	cmd := newAPICmd(f, "jira")
	cmd.AddCommand(newAPICmd(f, "jira-v2"))
	return cmd
}

// operationsFor returns the operationIds in scope for a namespace, sorted.
func operationsFor(namespace string) []string {
	if namespace == "" {
		return catalog.Operations()
	}
	return catalog.OperationsIn(namespace)
}

// resolveOperation looks an operationId up within a namespace's scope,
// mapping the failure modes to distinct errors: an id in no product is an
// UnknownOperationError (a typo, exit code validation); an id in several
// products is a ValidationError steering the user to a namespaced form or
// the --product flag. A non-empty product pins one product exactly, which is
// the escape hatch for ids that collide even within a single namespace (e.g.
// getIssue in jira and jira-software both under "atl jira api").
func resolveOperation(namespace, product, id string) (catalog.Operation, error) {
	if product != "" {
		if op, ok := catalog.LookupProduct(product, id); ok {
			return op, nil
		}
		defined := catalog.Products(id)
		if len(defined) == 0 {
			return catalog.Operation{}, &atlapi.UnknownOperationError{Operation: id}
		}
		return catalog.Operation{}, &cmdutil.ValidationError{Message: fmt.Sprintf(
			"operation %q is not defined in product %q; it is defined in: %s",
			catalog.CanonicalID(id), product, strings.Join(defined, ", "))}
	}

	var op catalog.Operation
	var ok bool
	var products []string
	if namespace == "" {
		op, ok = catalog.Lookup(id)
		products = catalog.Products(id)
	} else {
		op, ok = catalog.LookupIn(namespace, id)
		products = catalog.ProductsIn(namespace, id)
	}
	if ok {
		return op, nil
	}
	if len(products) == 0 {
		return catalog.Operation{}, &atlapi.UnknownOperationError{Operation: id}
	}
	return catalog.Operation{}, ambiguousOperationError(id, products)
}

// ambiguousOperationError builds the guidance shown when an operationId
// resolves to more than one product. It offers the namespaced form and, for
// ids that collide within one namespace, the --product flag that pins an
// exact product.
func ambiguousOperationError(id string, products []string) error {
	canonical := catalog.CanonicalID(id)
	var suggestions []string
	for _, ns := range namespacesFor(products) {
		suggestions = append(suggestions, fmt.Sprintf("atl %s api %s", ns, canonical))
	}
	suggestions = append(suggestions, fmt.Sprintf("--product %s", products[0]))
	return &cmdutil.ValidationError{Message: fmt.Sprintf(
		"operation %q is defined in multiple products (%s); disambiguate with: %s",
		canonical, strings.Join(products, ", "), strings.Join(suggestions, " | "))}
}

// namespacesFor maps a set of products to the api namespaces that reach
// them, sorted and de-duplicated.
func namespacesFor(products []string) []string {
	set := map[string]bool{}
	for _, p := range products {
		switch {
		case strings.HasPrefix(p, "jira"):
			set["jira"] = true
		case strings.HasPrefix(p, "confluence"):
			set["confluence"] = true
		}
	}
	out := make([]string, 0, len(set))
	for ns := range set {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}

// runAPIList prints every catalogued operationId in scope, one per line (or
// as a JSON array with -o json), needing no credentials or network access.
func runAPIList(f *cmdutil.Factory, namespace string) error {
	ids := operationsFor(namespace)
	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, ids)
	}
	for _, id := range ids {
		fmt.Fprintln(f.IOStreams.Out, id)
	}
	return nil
}

func runAPI(ctx context.Context, f *cmdutil.Factory, namespace, product, operation string, fields []string, inputFile string) error {
	if len(fields) > 0 && inputFile != "" {
		return &cmdutil.ValidationError{Message: "--field and --input cannot be combined"}
	}
	// Resolve the operation before touching credentials, so a typo or an
	// ambiguous id is reported as such even when the user is not logged in.
	op, err := resolveOperation(namespace, product, operation)
	if err != nil {
		return err
	}

	requestFields, err := buildRequestFields(fields, inputFile, f.IOStreams.In)
	if err != nil {
		return err
	}

	method, path, query, body, err := atlapi.Route(op, requestFields)
	if err != nil {
		return &cmdutil.ValidationError{Message: err.Error()}
	}

	client, err := f.ClientFn()
	if err != nil {
		return err
	}
	resp, callErr := client.Do(ctx, method, path, query, body)
	if resp != nil {
		if err := writeAPIResponse(f, resp); err != nil {
			return err
		}
	}
	return callErr
}

// writeAPIResponse emits one operation's response. A JSON body is
// pretty-printed as before; a body the endpoint served as something else
// (an attachment's bytes, an export, a thumbnail) is written verbatim, so
// "atl api getAttachmentContent -f id=... > file" produces the file instead
// of an error and an empty one.
func writeAPIResponse(f *cmdutil.Factory, resp *atlapi.Response) error {
	if resp.Binary {
		_, err := f.IOStreams.Out.Write(resp.Raw)
		return err
	}
	payload := resp.Body
	if payload == nil {
		payload = resp.Fields
	}
	return output.WriteJSON(f.IOStreams.Out, payload)
}

// buildRequestFields assembles the operation's request fields: -f/--field
// values as key=value pairs, or --input's JSON object.
func buildRequestFields(fields []string, inputFile string, stdin io.Reader) (map[string]any, error) {
	if inputFile != "" {
		return readInputFields(inputFile, stdin)
	}
	return parseFields(fields)
}

func parseFields(fields []string) (map[string]any, error) {
	pairs := make(map[string]any, len(fields))
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return nil, &cmdutil.ValidationError{Message: fmt.Sprintf("invalid --field %q, expected key=value", field)}
		}
		pairs[key] = typedValue(value)
	}
	return pairs, nil
}

// typedValue converts a --field value to the JSON type it spells: numbers,
// booleans, null, arrays, and objects parse as themselves so maxResults=42
// is sent as a JSON number; anything else stays a string. Numbers decode as
// json.Number rather than float64 so a large id (e.g. a 19-digit content id
// past 2^53) round-trips exactly into the path or body instead of being
// mangled into scientific notation or losing its low bits.
func typedValue(value string) any {
	dec := json.NewDecoder(strings.NewReader(value))
	dec.UseNumber()
	var typed any
	if err := dec.Decode(&typed); err == nil && !dec.More() {
		return typed
	}
	return value
}

func readInputFields(inputFile string, stdin io.Reader) (map[string]any, error) {
	data, err := readInput(inputFile, stdin)
	if err != nil {
		return nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, &cmdutil.ValidationError{Message: fmt.Sprintf("--input must be a JSON object: %v", err)}
	}
	return fields, nil
}

func readInput(inputFile string, stdin io.Reader) ([]byte, error) {
	if inputFile == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(inputFile)
}
