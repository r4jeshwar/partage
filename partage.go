package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"html/template"

	"github.com/dustin/go-humanize"
)

type templatedata struct {
	Maxsize string
}

var conf struct {
	bind     string
	baseuri  string
	filepath string
	rootdir  string
	templatedir string
	filectx  string
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

func writefile(f *os.File, s io.ReadCloser, contentlength int64) int64 {
	buffer := make([]byte, 4096)
	eof := false
	sz := int64(0)

	defer f.Sync()

	for !eof {
		n, err := s.Read(buffer)
		if err != nil && err != io.EOF {
			fmt.Println(err)
			return -1
		} else if err == io.EOF {
			eof = true
		}

		/* ensure we don't write more than expected */
		r := int64(n)
		if sz+r > contentlength {
			r = contentlength - sz
			eof = true
		}

		_, err = f.Write(buffer[:r])
		if err != nil {
			fmt.Println(err)
		}
		sz += r
	}

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
				if _, err := w.Write(buffer[:n]); err != nil {
					fmt.Println(err)
				}
				break
			}
			fmt.Println(err)
			return
		}

		if _, err = w.Write(buffer[:n]); err != nil {
			fmt.Println(err)
		}
	}
}

func servetemplate(w http.ResponseWriter, f string, d templatedata) {
	t, err := template.ParseFiles(conf.templatedir + "/" + f)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = t.Execute(w, d)
	if err != nil {
		fmt.Println(err)
	}
}

func uploaderPut(w http.ResponseWriter, r *http.Request) {
	// Max 15 Gb uploads
	if r.ContentLength > conf.maxsize {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte("File is too big"))
	}

	tmp, _ := ioutil.TempFile(conf.filepath, "*"+path.Ext(r.URL.Path))
	f, err := os.Create(tmp.Name())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()

	if writefile(f, r.Body, r.ContentLength) < 0 {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := conf.baseuri + conf.filectx + filepath.Base(tmp.Name()) + "\r\n"
	w.Write([]byte(resp))
}

func uploaderGet(w http.ResponseWriter, r *http.Request) {
	// r.URL.Path is sanitized regarding "." and ".."
	filename := r.URL.Path
	if r.URL.Path == "/" || r.URL.Path == "/index" {
		data := templatedata{ Maxsize: humanize.IBytes(uint64(conf.maxsize))}
		servetemplate(w, "/index.html", data)
		return
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

func uploader(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "PUT":
		uploaderPut(w, r)
	case "GET":
		uploaderGet(w, r)
	}
}

func main() {
	conf.bind = "0.0.0.0:8080"
	conf.maxsize = 30064771072 // 28Gib
	conf.filepath = "/tmp"
	conf.rootdir = "./static"
	conf.baseuri = "http://192.168.0.3:8080"
	conf.filectx = "/f/"
	conf.templatedir = "./templates"

	http.HandleFunc("/", uploader)
	http.Handle(conf.filectx, http.StripPrefix(conf.filectx, http.FileServer(http.Dir(conf.filepath))))
	http.ListenAndServe("0.0.0.0:8080", nil)
}
