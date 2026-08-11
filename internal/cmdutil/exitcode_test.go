package cmdutil_test

import (
	"fmt"
	"testing"

	"github.com/branow/atlassian-cli/internal/atlapi"
	"github.com/branow/atlassian-cli/internal/cmdutil"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, cmdutil.ExitSuccess},
		{"generic", fmt.Errorf("boom"), cmdutil.ExitError},
		{"cancelled", cmdutil.ErrCancelled, cmdutil.ExitCancelled},
		{"not logged in", cmdutil.ErrNotLoggedIn, cmdutil.ExitAuth},
		{"validation", &cmdutil.ValidationError{Message: "bad flag"}, cmdutil.ExitValidation},
		{"api 401", &atlapi.APIError{Status: 401, Message: "unauthorized"}, cmdutil.ExitAuth},
		{"api 403", &atlapi.APIError{Status: 403, Message: "forbidden"}, cmdutil.ExitAuth},
		{"api 404", &atlapi.APIError{Status: 404, Message: "not found"}, cmdutil.ExitNotFound},
		{"api 429", &atlapi.APIError{Status: 429, Message: "too many requests"}, cmdutil.ExitRateLimited},
		{"api 500", &atlapi.APIError{Status: 500, Message: "server error"}, cmdutil.ExitRateLimited},
		{"api 400", &atlapi.APIError{Status: 400, Message: "bad request"}, cmdutil.ExitError},
		{"unknown operation", &atlapi.UnknownOperationError{Operation: "getIssuez"}, cmdutil.ExitValidation},
		{"unsupported operation", &atlapi.UnsupportedOperationError{Operation: "doThing"}, cmdutil.ExitValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cmdutil.ExitCode(c.err); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
