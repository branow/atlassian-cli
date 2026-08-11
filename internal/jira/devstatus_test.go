package jira

import "testing"

const devStatusBody = `{
  "detail": [
    {
      "pullRequests": [
        {
          "id": "320",
          "name": "Merged in LAN-4594",
          "url": "https://bitbucket.org/x/y/pull-requests/320",
          "status": "DECLINED",
          "author": {"name": "Orest Bodnar"},
          "source": {"branch": "feature-x"},
          "destination": {"branch": "master"},
          "repositoryName": "team/order-service",
          "lastUpdate": "2026-01-14T08:38:35.566+0000",
          "commentCount": 2
        }
      ]
    }
  ]
}`

func TestParsePullRequests(t *testing.T) {
	prs, err := ParsePullRequests([]byte(devStatusBody))
	if err != nil {
		t.Fatalf("ParsePullRequests: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("want 1 pull request, got %d", len(prs))
	}
	got := prs[0]
	want := PullRequest{
		ID:                "320",
		Name:              "Merged in LAN-4594",
		URL:               "https://bitbucket.org/x/y/pull-requests/320",
		Status:            "DECLINED",
		Author:            "Orest Bodnar",
		SourceBranch:      "feature-x",
		DestinationBranch: "master",
		Repository:        "team/order-service",
		LastUpdate:        "2026-01-14T08:38:35.566+0000",
		CommentCount:      2,
	}
	if got != want {
		t.Fatalf("flattened pull request mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestParsePullRequestsEmpty(t *testing.T) {
	// The endpoint's "nothing linked" shape must yield no PRs, not an error.
	prs, err := ParsePullRequests([]byte(`{"detail": []}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 0 {
		t.Fatalf("want no pull requests, got %d", len(prs))
	}
}

func TestHasDetail(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"with PRs", devStatusBody, true},
		{"empty detail", `{"detail": []}`, false},
		{"detail without PRs", `{"detail": [{"pullRequests": []}]}`, false},
		{"invalid json", `not json`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := HasDetail([]byte(c.body)); got != c.want {
				t.Fatalf("HasDetail = %v, want %v", got, c.want)
			}
		})
	}
}

func TestBranchArrow(t *testing.T) {
	cases := []struct {
		pr   PullRequest
		want string
	}{
		{PullRequest{SourceBranch: "feat", DestinationBranch: "main"}, "feat -> main"},
		{PullRequest{SourceBranch: "feat"}, "feat"},
		{PullRequest{DestinationBranch: "main"}, "main"},
		{PullRequest{}, ""},
	}
	for _, c := range cases {
		if got := branchArrow(c.pr); got != c.want {
			t.Fatalf("branchArrow(%+v) = %q, want %q", c.pr, got, c.want)
		}
	}
}

func TestIssueIDOf(t *testing.T) {
	id, err := IssueIDOf([]byte(`{"id": "197467", "key": "CP-48382"}`))
	if err != nil {
		t.Fatalf("IssueIDOf: %v", err)
	}
	if id != "197467" {
		t.Fatalf("id = %q, want 197467", id)
	}
}
