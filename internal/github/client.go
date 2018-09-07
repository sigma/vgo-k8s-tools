package github

import (
	"context"
	"net/http"
	"os/user"
	"path/filepath"

	"github.com/bgentry/go-netrc/netrc"
	"github.com/birkelund/boltdbcache"
	ghclient "github.com/google/go-github/github"
	"github.com/gregjones/httpcache"
	"golang.org/x/oauth2"
)

var (
	githubUser  string
	githubToken string
)

func init() {
	net, err := getNetrc()
	if err == nil {
		m := net.FindMachine("api.github.com")
		githubUser = m.Login
		githubToken = m.Password
	}
}

func getNetrc() (*netrc.Netrc, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	return netrc.ParseFile(filepath.Join(usr.HomeDir, ".netrc"))
}

func getCache() (*boltdbcache.Cache, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	return boltdbcache.New(filepath.Join(usr.HomeDir, ".ghcache"))
}

// GetClient returns a personalized Github client
func GetClient() *ghclient.Client {

	var tc *http.Client

	if githubToken != "" {
		ctx := context.Background()
		tk := &oauth2.Token{AccessToken: githubToken}
		ts := oauth2.StaticTokenSource(tk)
		tc = oauth2.NewClient(ctx, ts)

		cache, err := getCache()
		if err == nil {
			tr := httpcache.NewTransport(cache)
			tc.Transport = tr
		}
	}

	c := ghclient.NewClient(tc)
	return c
}
