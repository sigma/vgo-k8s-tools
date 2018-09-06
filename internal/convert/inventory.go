package convert

import (
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/sigma/vgo-k8s-tools/internal/dependencies"
)

type Inventory struct {
	inv     map[string]*Module
	g       dependencies.Graph
	RootDir string
}

func (i *Inventory) GetModule(mod string) *Module {
	return i.inv[mod]
}

func (i *Inventory) GetExternalDependencies() []*Module {
	res := make([]*Module, 0)
	for _, m := range i.inv {
		if m.Main {
			continue
		}
		if !strings.HasPrefix(m.Replace.Path, ".") {
			res = append(res, m)
		}
	}
	return res
}

func (i *Inventory) GetSubmodules() []*Module {
	res := make([]*Module, 0)
	for _, m := range i.inv {
		if strings.HasPrefix(m.Replace.Path, ".") {
			res = append(res, m)
		}
	}
	return res
}

func (i *Inventory) GetSubmodulesFor(mod string) []string {
	res := make([]string, 0)

	g, err := i.getFullDependencyGraph()
	if err != nil {
		return res
	}

	mods := i.GetSubmodules()
	clos := g.RecursiveTransitiveClosure(mod)
	for _, m := range mods {
		if m.Path == mod {
			continue
		}
		for _, c := range clos {
			if c == m.Path || strings.HasPrefix(c, m.Path+"/") {
				res = append(res, m.Path)
				break
			}
		}
	}
	return res
}

func (i *Inventory) GetTools() ([]string, error) {
	tools := make([]string, 0)
	fname := filepath.Join(i.RootDir, "tools.go")

	if _, err := os.Stat(fname); !os.IsNotExist(err) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, fname, nil, parser.ImportsOnly)
		if err != nil {
			return tools, err
		}

		for _, s := range f.Imports {
			tools = append(tools, strings.Trim(s.Path.Value, `"`))
		}
	}

	return tools, nil
}

func (i *Inventory) GetMainModule() *Module {
	for _, m := range i.inv {
		if m.Main {
			return m
		}
	}
	return nil
}

func (i *Inventory) AsGodeps() (*Godeps, error) {
	tools, err := i.GetTools()
	if err != nil {
		return nil, err
	}

	subs, err := i.getSubPackages()
	if err != nil {
		return nil, err
	}

	deps := make([]Dependency, 0)
	for _, m := range i.inv {
		if m.Main {
			continue
		}

		h, err := m.GetHash()
		if err != nil {
			return nil, err
		}

		for _, sub := range subs[m.Path] {
			d := Dependency{
				ImportPath: filepath.Join(m.Path, sub),
				Rev:        h,
			}

			if !strings.HasSuffix(m.Version, h[:12]) {
				d.Comment = m.Version
			}
			deps = append(deps, d)
		}
	}

	gd := &Godeps{
		Packages:   tools,
		ImportPath: i.GetMainModule().Path,
		Deps:       deps,
	}
	return gd, nil
}

func (i *Inventory) getFullDependencyGraph() (dependencies.Graph, error) {
	if i.g != nil {
		return i.g, nil
	}

	b := dependencies.NewMultiDepsBuilder()

	mods := i.GetSubmodules()
	localPackages := make([]string, 0)
	skipSubdirs := make([]string, 0)
	for _, m := range mods {
		localPackages = append(localPackages, m.Path)
		if strings.HasPrefix(m.Replace.Dir, i.RootDir) {
			relpath, _ := filepath.Rel(i.RootDir, m.Replace.Dir)
			skipSubdirs = append(skipSubdirs, relpath)
		}
		b.Ingest(&dependencies.DepsBuilder{
			Root:        m.Replace.Dir,
			Package:     m.Path,
			SkipSubdirs: []string{"vendor"},
		})
	}
	skipSubdirs = append(skipSubdirs, "vendor")

	mainBuilder := &dependencies.DepsBuilder{
		Root:          i.RootDir,
		Package:       i.GetMainModule().Path,
		LocalPackages: localPackages,
		SkipSubdirs:   skipSubdirs,
	}

	b.Ingest(mainBuilder)

	for _, m := range i.GetExternalDependencies() {
		b.Ingest(&dependencies.DepsBuilder{
			Root:    m.Dir,
			Package: m.Path,
		})
	}

	g, err := b.GetFullDependencyGraph()

	// TODO(yhodique) don't hardcode this... Actually shouldn't be needed ?
	g = g.Normalize(func(path string) string {
		if strings.HasPrefix(path, "k8s.io/kubernetes/staging/src/") {
			return strings.TrimPrefix(path, "k8s.io/kubernetes/staging/src/")
		}
		return path
	})
	i.g = g
	return g, err
}

func (i *Inventory) getDependencies() ([]string, error) {
	pkg := i.GetMainModule().Path
	log.Println("computing dependencies for", pkg)
	g, err := i.getFullDependencyGraph()
	if err != nil {
		return nil, err
	}

	return g.RecursiveTransitiveClosure(pkg), nil
}

func (i *Inventory) getSubPackages() (map[string][]string, error) {
	subPkgs := make(map[string][]string)
	deps, err := i.getDependencies()
	if err != nil {
		return nil, err
	}
	for _, m := range i.inv {
		if m.Main {
			continue
		}
		for _, d := range deps {
			if strings.HasPrefix(d, m.Path) {
				sub := strings.TrimPrefix(d, m.Path)
				sub = strings.TrimPrefix(sub, "/")
				subPkgs[m.Path] = append(subPkgs[m.Path], sub)
			}
		}
	}
	return subPkgs, nil
}
