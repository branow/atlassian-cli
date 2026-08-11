package jira

import (
	"encoding/json"
	"net/url"
	"strings"
)

// The Development panel in Jira's web UI (branches, commits, pull requests)
// is backed by /rest/dev-status, an internal endpoint that no documented
// Jira REST API covers — Atlassian's own JSWCLOUD-16901 says so outright.
// The applicationType has been seen as both "bitbucket" (Cloud) and the
// older "stash" (Bitbucket Server's former name) depending on the instance
// vintage, so callers try both and keep the first that returns real data.

// DevStatusApplicationTypes are the applicationType values to try, in order,
// when querying the dev-status endpoint; the first with a non-empty detail
// wins. Bitbucket Cloud answers to "bitbucket"; older/linked Server
// instances answer to "stash".
var DevStatusApplicationTypes = []string{"bitbucket", "stash"}

// PullRequest is one Bitbucket pull request linked to an issue, flattened
// from the dev-status detail payload into the fields worth showing.
type PullRequest struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	URL               string `json:"url"`
	Status            string `json:"status"`
	Author            string `json:"author"`
	SourceBranch      string `json:"sourceBranch"`
	DestinationBranch string `json:"destinationBranch"`
	Repository        string `json:"repository"`
	LastUpdate        string `json:"lastUpdate"`
	CommentCount      int    `json:"commentCount"`
}

// devStatusResponse is the subset of the /rest/dev-status detail payload we
// read: each detail entry carries the pull requests for one linked instance.
type devStatusResponse struct {
	Detail []struct {
		PullRequests []devStatusPullRequest `json:"pullRequests"`
	} `json:"detail"`
}

// devStatusPullRequest mirrors the endpoint's per-PR shape. author, source,
// and destination are nested objects; the flat PullRequest pulls the useful
// leaf out of each.
type devStatusPullRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status"`
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
	Source struct {
		Branch string `json:"branch"`
	} `json:"source"`
	Destination struct {
		Branch string `json:"branch"`
	} `json:"destination"`
	LastUpdate     string `json:"lastUpdate"`
	RepositoryName string `json:"repositoryName"`
	CommentCount   int    `json:"commentCount"`
}

// ParsePullRequests flattens a dev-status detail payload into the pull
// requests it lists. A body with no detail (the endpoint's "nothing linked"
// shape) yields an empty slice, not an error.
func ParsePullRequests(raw []byte) ([]PullRequest, error) {
	var resp devStatusResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	var out []PullRequest
	for _, d := range resp.Detail {
		for _, pr := range d.PullRequests {
			out = append(out, PullRequest{
				ID:                pr.ID,
				Name:              pr.Name,
				URL:               pr.URL,
				Status:            pr.Status,
				Author:            pr.Author.Name,
				SourceBranch:      pr.Source.Branch,
				DestinationBranch: pr.Destination.Branch,
				Repository:        pr.RepositoryName,
				LastUpdate:        pr.LastUpdate,
				CommentCount:      pr.CommentCount,
			})
		}
	}
	return out, nil
}

// HasDetail reports whether a dev-status body carries at least one linked
// instance, so a caller can tell an application type that has data from one
// that returned an empty envelope and move on to the next.
func HasDetail(raw []byte) bool {
	var resp devStatusResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return false
	}
	for _, d := range resp.Detail {
		if len(d.PullRequests) > 0 {
			return true
		}
	}
	return false
}

// DevStatusPath builds the dev-status detail request path for an issue's
// numeric id, application type, and data type (e.g. "pullrequest").
func DevStatusPath(issueID, applicationType, dataType string) (string, url.Values) {
	query := url.Values{}
	query.Set("issueId", issueID)
	query.Set("applicationType", applicationType)
	query.Set("dataType", dataType)
	return "/rest/dev-status/latest/issue/detail", query
}

// PullRequestColumns is the table view for linked pull requests.
var PullRequestColumns = []Column[PullRequest]{
	{Header: "ID", Value: func(p PullRequest) string { return p.ID }},
	{Header: "STATUS", Value: func(p PullRequest) string { return p.Status }},
	{Header: "TITLE", Value: func(p PullRequest) string { return p.Name }},
	{Header: "BRANCH", Value: func(p PullRequest) string { return branchArrow(p) }},
	{Header: "REPOSITORY", Value: func(p PullRequest) string { return p.Repository }},
	{Header: "URL", Value: func(p PullRequest) string { return p.URL }},
}

// branchArrow renders "source -> destination" when both are known, or
// whichever single branch is present.
func branchArrow(p PullRequest) string {
	switch {
	case p.SourceBranch != "" && p.DestinationBranch != "":
		return p.SourceBranch + " -> " + p.DestinationBranch
	case p.SourceBranch != "":
		return p.SourceBranch
	default:
		return p.DestinationBranch
	}
}

// IssueIDOf extracts the numeric id from a GET issue response (fields=id is
// enough); it is the id the dev-status endpoint keys on, distinct from the
// issue key.
func IssueIDOf(raw []byte) (string, error) {
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.ID), nil
}
