package convert

import (
	"log"
	"os"
	"path/filepath"
)

type Converter struct {
	RootDir     string
	GodepCompat bool
}

func (c *Converter) GenFiles() error {
	r := &VgoRunner{
		RootDir: c.RootDir,
	}

	log.Println("getting inventory")
	inv, err := r.GetInventory()
	if err != nil {
		return err
	}

	if c.GodepCompat {
		log.Println("computing top-level godeps.json")
		gd, err := inv.AsGodeps()
		if err != nil {
			return err
		}
		log.Println("dumping top-level godeps.json")
		err = gd.DumpToFile(filepath.Join(c.RootDir, "Godeps", "Godeps.json"))
		if err != nil {
			return err
		}
	}

	for _, m := range inv.GetSubmodules() {
		log.Println("considering submodule:", m.Path)
		r.GenVgoMod(m.Path)
		sr := &VgoRunner{
			RootDir: m.Replace.Dir,
		}
		sr.Tidy()

		if c.GodepCompat {
			log.Println("getting inventory")
			sinv, err := sr.GetInventory()
			if err != nil {
				return err
			}
			log.Println("computing godeps.json")
			gd, err := sinv.AsGodeps()
			if err != nil {
				return err
			}
			log.Println("dumping godeps.json")
			err = gd.DumpToFile(filepath.Join(m.Replace.Dir, "Godeps", "Godeps.json"))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func Main() {
	cwd, _ := os.Getwd()
	c := &Converter{
		RootDir:     cwd,
		GodepCompat: true,
	}

	err := c.GenFiles()
	if err != nil {
		panic(err)
	}
}
