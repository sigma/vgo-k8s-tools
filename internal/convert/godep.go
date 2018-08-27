package convert

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	GodepCompat = "v80"
)

type Godeps struct {
	Comment      string `json:"_Comment"`
	ImportPath   string
	GoVersion    string
	GodepVersion string
	Packages     []string
	Deps         []Dependency
}

type Dependency struct {
	ImportPath string
	Comment    string `json:",omitempty"`
	Rev        string
}

func (gd *Godeps) DumpToFile(fname string) error {
	gd.Comment = "GENERATED FROM VGO, DO NOT EDIT"
	gd.Packages = append(gd.Packages, "./...")
	gd.GoVersion = getVersion()
	gd.GodepVersion = GodepCompat

	sort.Slice(gd.Deps, func(i, j int) bool {
		return strings.Compare(gd.Deps[i].ImportPath, gd.Deps[j].ImportPath) < 0
	})

	js, err := json.MarshalIndent(gd, "", "\t")
	if err != nil {
		return err
	}

	parent := filepath.Dir(fname)
	err = os.MkdirAll(parent, 0755)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(fname, js, 0644)
	return err
}

func getVersion() string {
	v := runtime.Version()
	p := strings.Split(v, ".")
	if len(p) > 1 {
		return p[0] + "." + p[1]
	}
	return v
}
