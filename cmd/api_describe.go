package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/branow/atlassian-cli/internal/atlapi/catalog"
	"github.com/branow/atlassian-cli/internal/cmdutil"
	"github.com/branow/atlassian-cli/internal/output"
)

// runAPIDescribe prints an operation's REST binding and documented fields
// from the embedded catalog, needing no credentials or network. It answers
// "which fields does this operation take?" so users compose calls instead
// of guessing.
func runAPIDescribe(f *cmdutil.Factory, namespace, product, operation string) error {
	op, err := resolveOperation(namespace, product, operation)
	if err != nil {
		return err
	}

	if f.OutputFormat() == "json" {
		return output.WriteJSON(f.IOStreams.Out, describeView(op))
	}

	out := f.IOStreams.Out
	fmt.Fprintf(out, "%s  (product %s)\n  %s %s\n\n", op.ID, op.Product, op.Method, op.Path)
	if len(op.PathParams) > 0 {
		printSection(out, op.Product, "Path parameters", op.PathParams, true)
		fmt.Fprintln(out)
	}
	if len(op.Query) > 0 {
		printSection(out, op.Product, "Query parameters", op.Query, true)
		fmt.Fprintln(out)
	}
	if len(op.Body) > 0 {
		printSection(out, op.Product, "Body", op.Body, true)
		fmt.Fprintln(out)
	}
	printSection(out, op.Product, "Response", op.Response, false)
	return nil
}

// printSection prints one labelled field section, expanding nested
// component types recursively as the operation's product documents them.
// showRequired adds the required-or-optional column, meaningful only for
// request fields.
func printSection(w io.Writer, product, label string, fields []catalog.Field, showRequired bool) {
	fmt.Fprintf(w, "  %s:\n", label)
	if len(fields) == 0 {
		fmt.Fprintln(w, "    (none)")
		return
	}
	printFields(w, product, fields, "    ", showRequired, map[string]bool{})
}

// printFields renders a block of sibling fields in aligned columns, then
// recurses into any field whose type is a catalogued component type,
// indented one level deeper. expanding tracks the types open on the current
// branch so a self-referential type does not recurse forever; sibling reuse
// of a type still expands because the entry is cleared after the branch.
func printFields(w io.Writer, product string, fields []catalog.Field, indent string, showRequired bool, expanding map[string]bool) {
	nameW, typeW := 0, 0
	for _, f := range fields {
		nameW = max(nameW, len(f.Name))
		typeW = max(typeW, len(f.Type))
	}
	for _, f := range fields {
		line := indent + fmt.Sprintf("%-*s", nameW, f.Name)
		if typeW > 0 {
			line += "  " + fmt.Sprintf("%-*s", typeW, f.Type)
		}
		if showRequired {
			line += "  " + requiredLabel(f.Required)
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))

		base := strings.TrimSuffix(f.Type, "[]")
		if nested, ok := catalog.LookupType(product, base); ok && !expanding[base] {
			expanding[base] = true
			printFields(w, product, nested, indent+"  ", showRequired, expanding)
			delete(expanding, base)
		}
	}
}

func requiredLabel(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

// describeView is the JSON shape of --describe -o json: the operation's
// binding plus a types map defining every component type its fields
// reference (transitively), so a machine can resolve nested shapes without
// re-deriving them.
func describeView(op catalog.Operation) map[string]any {
	view := map[string]any{
		"operation": op.ID,
		"product":   op.Product,
		"method":    op.Method,
		"path":      op.Path,
	}
	if op.Deprecated {
		view["deprecated"] = true
	}
	if len(op.PathParams) > 0 {
		view["pathParams"] = op.PathParams
	}
	if len(op.Query) > 0 {
		view["query"] = op.Query
	}
	if len(op.Body) > 0 {
		view["body"] = op.Body
	}
	if len(op.Response) > 0 {
		view["response"] = op.Response
	}
	types := map[string][]catalog.Field{}
	collectReferencedTypes(op.Product, op.PathParams, types)
	collectReferencedTypes(op.Product, op.Query, types)
	collectReferencedTypes(op.Product, op.Body, types)
	collectReferencedTypes(op.Product, op.Response, types)
	if len(types) > 0 {
		view["types"] = types
	}
	return view
}

// collectReferencedTypes walks fields and records the definition of every
// component type they reference as the operation's product documents it,
// recursing through nested types.
// The acc map both accumulates results and guards against cycles.
func collectReferencedTypes(product string, fields []catalog.Field, acc map[string][]catalog.Field) {
	for _, f := range fields {
		base := strings.TrimSuffix(f.Type, "[]")
		if _, seen := acc[base]; seen {
			continue
		}
		if def, ok := catalog.LookupType(product, base); ok {
			acc[base] = def
			collectReferencedTypes(product, def, acc)
		}
	}
}
