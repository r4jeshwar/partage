package main

import (
	"log"
	"flag"
	"os"
	"time"
	"path/filepath"
	"encoding/json"
	
	"github.com/dustin/go-humanize"
)

type metadata struct {
	Filename string
	Size int64
	Expiry int64
}

var conf struct {
	filepath string
	metapath string
}

var verbose bool
var count int64
var deleted int64
var size int64

func readmeta(filename string) (metadata, error) {
	j, err := os.ReadFile(filename)
	if err != nil {
		return metadata{}, err
	}

	var meta metadata
	err = json.Unmarshal(j, &meta)
	if err != nil {
		return metadata{}, err
	}

	return meta, nil
}

func checkexpiry(path string, info os.FileInfo, err error) error {
	if filepath.Ext(path) != ".json" {
		return nil
	}
	meta, err := readmeta(path)
	if err != nil {
		log.Fatal(err)
	}


	count++

	now := time.Now().Unix()
	if verbose {
		log.Printf("now: %s, expiry: %s\n", now, meta.Expiry);
	}

	if meta.Expiry > 0 && now >= meta.Expiry {
		if verbose {
			expiration :=  humanize.Time(time.Unix(meta.Expiry, 0))
			log.Printf("%s/%s: expired %s\n", conf.filepath, meta.Filename, expiration)
		}
		if err = os.Remove(conf.filepath + "/" + meta.Filename); err != nil {
			log.Fatal(err)
		}
		if err = os.Remove(path); err != nil {
			log.Fatal(err)
		}
		deleted++
		return nil
	} else {
		if verbose {
			expiration :=  humanize.Time(time.Unix(meta.Expiry, 0))
			log.Printf("%s/%s: expire in %s\n", conf.filepath, meta.Filename, expiration)
		}
		size += meta.Size
	}

	return nil
}

func main() {
	flag.BoolVar(&verbose,         "v", false, "Verbose logging")
	flag.StringVar(&conf.filepath, "f", "./files", "Directory containing files")
	flag.StringVar(&conf.metapath, "m", "./meta", "Directory containing metadata")

	flag.Parse()

	err := filepath.Walk(conf.metapath, checkexpiry)
	if err != nil {
		log.Fatal(err)
	}

	if verbose && count > 0 {
		log.Printf("%d/%d file(s) deleted (remaining: %s)", deleted, count, humanize.IBytes(uint64(size)))
	}
}
