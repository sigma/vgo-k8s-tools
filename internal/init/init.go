package init

import "os"

func Main() {
	c := NewConverter(".", os.Args[1])
	c.GenGoMods()
}
