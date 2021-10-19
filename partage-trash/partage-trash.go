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

	now := time.Now().Unix()

	if meta.Expiry > 0 && now >= meta.Expiry {
		if verbose {
			expiration :=  humanize.Time(time.Unix(meta.Expiry, 0))
			log.Printf("%s/%s expired %s\n", conf.filepath, meta.Filename, expiration)
		}
		os.Remove(conf.filepath + "/" + meta.Filename)
		os.Remove(path)
		return nil
	} else {
		count++
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
		log.Printf("%d file(s) remain on disk (total: %s)", count, humanize.IBytes(uint64(size)))
	}
}
