package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

var conf struct {
	bind     string
	filepath string
	rootdir  string
	baseuri  string
	maxsize  int64
}

func contenttype(f *os.File) string {
	buffer := make([]byte, 512)

	_, err := f.Read(buffer)
	if err != nil {
		return ""
	}

	mime := http.DetectContentType(buffer)

	return mime
}

func writefile(f *os.File, s io.ReadCloser) int {
	buffer := make([]byte, 4096)

	var sz int

	for {
		n, err := s.Read(buffer)

		if err == io.EOF {
			n, err := f.Write(buffer[:n])
			if err != nil {
				fmt.Println(err)
			}
			sz += n
			break
		}
		if err != nil {
			fmt.Println(err)
			return -1
		}

		n, err = f.Write(buffer[:n])
		if err != nil {
			fmt.Println(err)
		}
		sz += n
	}

	f.Sync()
	return sz
}

func servefile(f *os.File, w http.ResponseWriter) {
	buffer := make([]byte, 4096)

	mime := contenttype(f)
	w.Header().Set("Content-Type", mime)

	f.Seek(0, 0)
	for {
		n, err := f.Read(buffer)

		if err != nil {
			if err == io.EOF {
				_, err := w.Write(buffer[:n])
				if err != nil {
					fmt.Println(err)
				}
				break
			}
			fmt.Println(err)
			return
		}

		_, err = w.Write(buffer[:n])
		if err != nil {
			fmt.Println(err)
		}
	}
}

func parse(w http.ResponseWriter, r *http.Request) {

	fmt.Printf("%s %s\n", r.Method, r.URL.Path)

	// Max 15 Gb uploads
	if r.ContentLength > conf.maxsize {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte("File is too big"))
	}

	err := r.ParseForm()
	if err != nil {
		fmt.Printf("%s %s: %s", r.Method, r.URL.Path, err)
	}

	switch r.Method {
	case "PUT":
		tmp, _ := ioutil.TempFile(conf.filepath, "*"+path.Ext(r.URL.Path))
		f, err := os.Create(tmp.Name())
		if err != nil {
			fmt.Println(err)
			return
		}
		defer f.Close()

		if writefile(f, r.Body) < 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp := conf.baseuri + "/" + filepath.Base(tmp.Name())
		w.Write([]byte(resp))

	case "GET":
		// r.URL.Path is sanitized regarding "." and ".."
		filename := r.URL.Path
		if r.URL.Path == "/" {
			filename = "/index.html"
		}
		
		f, err := os.Open(conf.rootdir + filename)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			fmt.Println(err)
			return
		}
		defer f.Close()

		servefile(f, w)
	}
}

func main() {
	conf.bind = "0.0.0.0:8080"
	conf.maxsize = 28 * 1024 * 1024
	conf.filepath = "/tmp"
	conf.rootdir = "./static"
	conf.baseuri = "http://192.168.0.3:8080"

	http.HandleFunc("/", parse)
	http.ListenAndServe("0.0.0.0:8080", nil)
}
