package init

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blang/semver"
	"github.com/sigma/vgo-k8s-tools/internal/convert"
	"github.com/sigma/vgo-k8s-tools/internal/github"
)

var (
	helper *github.RefHelper
	cache  map[string]string
)

func init() {
	helper = github.NewRefHelper()
	cache = make(map[string]string)
}

type Replacement struct {
	ModuleName string
	Path       string
}

type Requirement struct {
	ModuleName string
	Version    string
}

type GoModWriter struct {
	RootDir      string
	ModuleName   string
	Replacements []*Replacement
	Requirements []*Requirement
}

type Converter struct {
	RootDir       string
	ModuleName    string
	StagingSubdir string
}

func NewConverter(root, mod string) *Converter {
	return &Converter{
		RootDir:    root,
		ModuleName: mod,
	}
}

func (c *Converter) GetReplacements() []*Replacement {
	res := make([]*Replacement, 0)
	root, _ := filepath.Abs(c.RootDir)
	vendorRoot := filepath.Join(root, "vendor")
	err := filepath.Walk(vendorRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if (info.Mode() & os.ModeSymlink) != 0 {
			resolved, err := os.Readlink(path)
			if err != nil {
				panic(err)
			}
			abs, _ := filepath.Abs(filepath.Join(filepath.Dir(path), resolved))
			rel, _ := filepath.Rel(root, abs)

			if !strings.HasPrefix(rel, ".") && !strings.HasPrefix(rel, "vendor/") {
				mod, _ := filepath.Rel(vendorRoot, path)
				res = append(res, &Replacement{
					ModuleName: mod,
					Path:       abs,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil
	}
	return res
}

func (c *Converter) GenGoMods() error {
	replacements := c.GetReplacements()

	writers := make([]*GoModWriter, 0)

	abs, _ := filepath.Abs(c.RootDir)
	writers = append(writers, &GoModWriter{
		RootDir:      abs,
		ModuleName:   c.ModuleName,
		Replacements: replacements,
		Requirements: getRequirements(abs),
	})

	for _, r := range replacements {
		writers = append(writers, &GoModWriter{
			RootDir:      r.Path,
			ModuleName:   r.ModuleName,
			Replacements: replacements,
			Requirements: getRequirements(r.Path),
		})
	}

	for _, w := range writers {
		err := w.Write()
		if err != nil {
			return err
		}
	}
	return nil
}

// ouch, that's horrible. But hey, that's just a transition tool. We'll stop
// needing it when k8s uses vgo for real.
func guessRepo(path string) (string, string, error) {
	pack := "."
	if strings.HasPrefix(path, "github.com") ||
		strings.HasPrefix(path, "golang.org") ||
		strings.HasPrefix(path, "bitbucket.org") ||
		strings.HasPrefix(path, "gonum.org") {
		cpts := strings.SplitN(path, "/", 4)
		if len(cpts) > 3 {
			pack = cpts[3]
		}
		return strings.Join(cpts[:3], "/"), pack, nil
	}
	if strings.HasPrefix(path, "cloud.google.com") ||
		strings.HasPrefix(path, "google.golang.org") ||
		strings.HasPrefix(path, "k8s.io") ||
		strings.HasPrefix(path, "vbom.ml") {
		cpts := strings.SplitN(path, "/", 3)
		if len(cpts) > 2 {
			pack = cpts[2]
		}
		return strings.Join(cpts[:2], "/"), pack, nil
	}
	if strings.HasPrefix(path, "go4.org") {
		cpts := strings.SplitN(path, "/", 2)
		if len(cpts) > 1 {
			pack = cpts[1]
		}
		return strings.Join(cpts[:1], "/"), pack, nil
	}
	if strings.HasPrefix(path, "gopkg.in") {
		r := regexp.MustCompile(`(?P<Repo>.*\.v[0-9]*)(?:/(?P<Path>.*))?`)
		cpts := r.FindStringSubmatch(path)

		if len(cpts) < 3 {
			return path, ".", fmt.Errorf("not a valid gopkg.in repo")
		}
		if cpts[2] != "" {
			pack = cpts[2]
		}
		return cpts[1], pack, nil
	}

	return path, ".", fmt.Errorf("don't know how to guess %s", path)
}

func getRequirements(path string) []*Requirement {
	fname := filepath.Join(path, "Godeps", "Godeps.json")
	content, err := ioutil.ReadFile(fname)

	if err != nil {
		return nil
	}

	var doc convert.Godeps
	json.Unmarshal(content, &doc)

	repos := make(map[string]string)
	for _, d := range doc.Deps {
		if strings.HasPrefix(d.Rev, "xxxxxxxxxx") {
			continue
		}
		r, _, err := guessRepo(d.ImportPath)
		if err != nil {
			panic(d.ImportPath)
		}

		if e, ok := cache[r]; ok {
			repos[r] = e
			continue
		}

		ts := helper.GithubCommit(r, d.Rev).VgoTimestamp()
		maj := "v0"
		if strings.HasPrefix(r, "gopkg.in") {
			maj = r[strings.LastIndex(r, ".")+1:]
		}
		repos[r] = fmt.Sprintf("%s.0.0-%s-%s", maj, ts, d.Rev[:12])

		if d.Comment != "" && isSupportedVgoVersion(d.Comment) {
			repos[r] = d.Comment
		}

		cache[r] = repos[r]
	}

	res := make([]*Requirement, 0)
	for k, v := range repos {
		res = append(res, &Requirement{
			ModuleName: k,
			Version:    v,
		})
	}
	return res
}

func isSupportedVgoVersion(version string) bool {
	numVersion := version
	// TODO(yhodique) vgo seems a bit overzealous right there
	if !strings.HasPrefix(version, "v") {
		return false
	}
	numVersion = version[1:]

	if strings.Contains(numVersion, "-g") { // git pseudo-version
		return false
	}

	v, err := semver.Make(numVersion)
	if err != nil {
		return false
	}

	if v.Major >= 2 {
		return false
	}

	return true
}

func (g *GoModWriter) Write() error {
	var b bytes.Buffer
	fmt.Fprintln(&b, "module", g.ModuleName)
	fmt.Fprintln(&b, "require (")
	for _, r := range g.Replacements {
		if r.ModuleName == g.ModuleName {
			continue
		}
		fmt.Fprintln(&b, r.ModuleName, "v0.0.0-dev")
	}
	fmt.Fprintln(&b, ")")
	fmt.Fprintln(&b, "replace (")
	for _, r := range g.Replacements {
		if r.ModuleName == g.ModuleName {
			fmt.Fprintln(&b, r.ModuleName, "v0.0.0-dev => ./")
			continue
		}
		rel, _ := filepath.Rel(g.RootDir, r.Path)
		if !strings.HasPrefix(rel, ".") {
			rel = "./" + rel
		}
		fmt.Fprintln(&b, r.ModuleName, "v0.0.0-dev =>", rel)
	}
	fmt.Fprintln(&b, ")")
	fmt.Fprintln(&b, "require (")
	for _, r := range g.Requirements {
		fmt.Fprintln(&b, r.ModuleName, r.Version)
	}
	fmt.Fprintln(&b, ")")

	return ioutil.WriteFile(filepath.Join(g.RootDir, "go.mod"), b.Bytes(), 0644)
}
