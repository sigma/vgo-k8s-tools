package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Module represents a go module
type Module struct {
	Path    string
	Version string
	Main    bool
	Time    string
	Replace Replacement
	Dir     string
	GoMod   string
}

// Replacement represents a local version of a module
type Replacement struct {
	Path    string
	Version string
	Time    string
	Dir     string
	GoMod   string
}

type VgoRunner struct {
	RootDir string
	inv     *Inventory
}

func (r *VgoRunner) getCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.Dir = r.RootDir
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GO111MODULE=on")
	return cmd
}

func (r *VgoRunner) GetInventory() (*Inventory, error) {
	var err error
	if r.inv == nil {
		r.inv, err = r.getInventory()
	}
	return r.inv, err
}

func (r *VgoRunner) getInventory() (*Inventory, error) {
	cmd := r.getCommand("go", "list", "-json", "-m", "all")
	out, err := cmd.Output()
	if err != nil {
		log.Println("oops")
		log.Println(string(out))
		return nil, err
	}

	res := make(map[string]*Module)
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var m Module
		err := dec.Decode(&m)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		res[m.Path] = &m
	}
	return &Inventory{
		inv:     res,
		RootDir: r.RootDir,
	}, nil
}

func (r *VgoRunner) Tidy() error {
	cmd := r.getCommand("go", "mod", "tidy")
	return cmd.Run()
}

func (r *VgoRunner) GenVgoMod(mod string) error {
	inv, err := r.GetInventory()
	if err != nil {
		return err
	}

	thisMod := inv.GetModule(mod)

	subs := make([]*Module, 0)
	for _, m := range inv.GetSubmodulesFor(mod) {
		if m == thisMod.Path {
			continue
		}
		subs = append(subs, inv.GetModule(m))
	}
	sort.Slice(subs, func(i, j int) bool {
		return subs[i].Path < subs[j].Path
	})

	var b bytes.Buffer
	modPath, err := filepath.Rel(thisMod.Dir, inv.GetMainModule().GoMod)
	if err != nil {
		return err
	}

	fmt.Fprintln(&b, "// Generated from", modPath)
	fmt.Fprintf(&b, "module %s\n\n", mod)

	fmt.Fprintln(&b, "require (")
	for _, m := range subs {
		fmt.Fprintln(&b, "\t", m.Path, m.Version)
	}
	fmt.Fprintf(&b, ")\n\n")

	fmt.Fprintln(&b, "replace (")
	for _, m := range subs {
		path, err := filepath.Rel(thisMod.Dir, m.Dir)
		if err != nil {
			return err
		}
		fmt.Fprintln(&b, "\t", m.Path, m.Version, "=>", path)
	}
	fmt.Fprintf(&b, ")\n\n")

	fmt.Fprintln(&b, "require (")
	deps := inv.GetExternalDependencies()
	sort.Slice(deps, func(i, j int) bool {
		return deps[i].Path < deps[j].Path
	})

	for _, m := range deps {
		fmt.Fprintln(&b, "\t", m.Path, m.Version)
	}
	fmt.Fprintln(&b, ")")

	return ioutil.WriteFile(filepath.Join(thisMod.Dir, "go.mod"), b.Bytes(), 0644)
}

type ModInfo struct {
	Version string
	Name    string
	Short   string
	Time    string
}

func (m *Module) GetHash() (string, error) {
	if strings.HasPrefix(m.Replace.Path, ".") {
		return "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", nil
	}

	modFileName := m.Replace.GoMod
	if modFileName == "" {
		modFileName = m.GoMod
	}
	infoFileName := strings.TrimSuffix(modFileName, ".mod") + ".info"

	jsonFile, err := os.Open(infoFileName)
	if err != nil {
		return "", err
	}
	defer jsonFile.Close()

	bytes, _ := ioutil.ReadAll(jsonFile)
	var info ModInfo

	err = json.Unmarshal(bytes, &info)
	if err != nil {
		return "", err
	}

	return info.Name, nil
}
