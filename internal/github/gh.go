package github

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	ghclient "github.com/google/go-github/github"
)

// RefHelper provides ways to generate vgo pseudo references
type RefHelper struct {
	client *ghclient.Client
}

// NewRefHelper builds a RefHelper
func NewRefHelper() *RefHelper {
	return &RefHelper{
		client: GetClient(),
	}
}

// Commit represents a commit in a repository
type Commit struct {
	ID   string
	Date *time.Time
}

// GithubCommit returns a Commit object
func (h *RefHelper) GithubCommit(repo, commit string) *Commit {
	r, err := repoToGithub(repo)
	if err != nil {
		fmt.Println("failed for", repo)
		return &Commit{}
	}

	cpts := strings.Split(r, "/")
	owner := cpts[1]
	repo = cpts[2]

	svc := h.client.Git
	ctx := context.Background()
	c, _, err := svc.GetCommit(ctx, owner, repo, commit)

	if err != nil {
		panic(err)
		return &Commit{
			ID: commit,
		}
	}

	fmt.Println("got date for", repo)
	return &Commit{
		ID:   commit,
		Date: c.Committer.Date,
	}
}

func (c *Commit) VgoTimestamp() string {
	if c.Date == nil {
		return "00000000000000"
	}
	return c.Date.Format("20060102150405")
}

// TODO(yhodique) make it more subtle...
func repoToGithub(repo string) (string, error) {
	known := map[string]string{
		"cloud.google.com/go":        "github.com/GoogleCloudPlatform/gcloud-golang",
		"google.golang.org/api":      "github.com/google/google-api-go-client",
		"google.golang.org/grpc":     "github.com/grpc/grpc-go",
		"google.golang.org/genproto": "github.com/google/go-genproto",
		"go4.org":                    "github.com/camlistore/go4",
		"vbom.ml/util":               "github.com/fvbommel/util",
		// TODO(yhodique) terrible, terrible hack
		"bitbucket.org/bertimus9/systemstat": "github.com/sigma/systemstat",
	}

	if val, ok := known[repo]; ok {
		return val, nil
	}

	if strings.HasPrefix(repo, "github.com") {
		return repo, nil
	} else if strings.HasPrefix(repo, "golang.org/x/") {
		return fmt.Sprintf("github.com/golang/%s", repo[13:]), nil
	} else if strings.HasPrefix(repo, "gonum.org/v1/") {
		return fmt.Sprintf("github.com/gonum/%s", repo[13:]), nil
	} else if strings.HasPrefix(repo, "k8s.io/") {
		return fmt.Sprintf("github.com/kubernetes/%s", repo[7:]), nil
	} else if strings.HasPrefix(repo, "gopkg.in/") {
		s := regexp.MustCompile("\\.v[0-9]+").Split(repo, 2)
		cpts := strings.Split(s[0], "/")
		if len(cpts) == 2 {
			return fmt.Sprintf("github.com/go-%s/%s", cpts[1], cpts[1]), nil
		}
		// len(cpts) == 3
		return fmt.Sprintf("github.com/%s/%s", cpts[1], cpts[2]), nil
	}

	return "", errors.New("don't know how to handle repo")
}
