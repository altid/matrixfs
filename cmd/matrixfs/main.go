package main

import (
	"flag"
	"log"
	"os"

	matrixfs "github.com/altid/matrixfs"
)

var (
	srv   = flag.String("s", "matrix", "name of service")
	addr  = flag.String("a", "localhost", "listening address")
	mdns  = flag.Bool("m", false, "enable mDNS broadcast of service")
	port  = flag.Int("p", 12345, "default port to listen on")
	debug = flag.Bool("d", false, "enable debug printing")
	ldir  = flag.Bool("l", false, "enable logging for main buffers")
	setup = flag.Bool("conf", false, "run configuration setup")
)

func main() {
	flag.Parse()
	if flag.Lookup("h") != nil {
		flag.Usage()
		os.Exit(1)
	}

	if *setup {
		if e := matrixfs.CreateConfig(*srv, *debug); e != nil {
			log.Fatal(e)
		}
		os.Exit(1)
	}

	matrix, err := matrixfs.Register(*ldir, *addr, *port, *srv, *debug)
	if err != nil {
		log.Fatal(err)
	}
	defer matrix.Cleanup()
	if *mdns {
		if e := matrix.Broadcast(); e != nil {
			log.Fatal(e)
		}
	}

	if e := matrix.Run(); e != nil {
		log.Fatal(e)
	}

	os.Exit(0)
}
