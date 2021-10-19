package main

import (
	"fmt"
	"flag"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
	"path"
	"path/filepath"
	"html/template"
	"encoding/json"

	"github.com/dustin/go-humanize"
)

type templatedata struct {
	Links []string
	Size string
	Maxsize string
}

type metadata struct {
	Filename string
	Size int64
	Expiry int64
}

var conf struct {
	bind     string
	baseuri  string
	filepath string
	metapath string
	rootdir  string
	templatedir string
	filectx  string
	metactx  string
	maxsize  int64
	expiry   int64
}


func writefile(f *os.File, s io.ReadCloser, contentlength int64) error {
	buffer := make([]byte, 4096)
	eof := false
	sz := int64(0)

	defer f.Sync()

	for !eof {
		n, err := s.Read(buffer)
		if err != nil && err != io.EOF {
			return err
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
			return err
		}
		sz += r
	}

	return nil
}

func writemeta(filename string, expiry int64) error {

	f, _ := os.Open(filename)
	stat, _ := f.Stat()
	size := stat.Size()
	f.Close()

	meta := metadata{
		Filename: filepath.Base(filename),
		Size: size,
		Expiry: time.Now().Unix() + expiry,
	}

	f, err := os.Create(conf.metapath + "/" + meta.Filename + ".json")
	if err != nil {
		return err
	}
	defer f.Close()

	j, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	_, err = f.Write(j)

	return err
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
	/* limit upload size */
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

	if err = writefile(f, r.Body, r.ContentLength); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		defer os.Remove(tmp.Name())
		return
	}
	writemeta(tmp.Name(), conf.expiry)

	resp := conf.baseuri + conf.filectx + filepath.Base(tmp.Name())
	w.Write([]byte(resp))
}

func uploaderPost(w http.ResponseWriter, r *http.Request) {
	/* read 32Mb at a time */
	r.ParseMultipartForm(32 << 20)

	links := []string{}
	for _, h := range r.MultipartForm.File["uck"] {
		if h.Size > conf.maxsize {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			w.Write([]byte("File is too big"))
			return
		}

		post, err := h.Open()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer post.Close()

		tmp, _ := ioutil.TempFile(conf.filepath, "*"+path.Ext(h.Filename))
		f, err := os.Create(tmp.Name())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer f.Close()

		if err = writefile(f, post, h.Size); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			defer os.Remove(tmp.Name())
			return
		}

		writemeta(tmp.Name(), conf.expiry)


		//link := conf.baseuri + conf.filectx + filepath.Base(tmp.Name())
		link := conf.baseuri + conf.metactx + filepath.Base(tmp.Name()) + ".json"
		links = append(links, link)
	}

	if (r.PostFormValue("output") == "html") {
		data := templatedata{ Links: links }
		servetemplate(w, "/upload.html", data)
		return
	} else {
		for _, link := range links {
			w.Write([]byte(link + "\r\n"))
		}
	}
}

func uploaderGet(w http.ResponseWriter, r *http.Request) {
	// r.URL.Path is sanitized regarding "." and ".."
	filename := r.URL.Path
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		data := templatedata{ Maxsize: humanize.IBytes(uint64(conf.maxsize))}
		servetemplate(w, "/index.html", data)
		return
	}

	http.ServeFile(w, r, conf.rootdir + filename)
}

func uploader(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		uploaderPost(w, r)
	case "PUT":
		uploaderPut(w, r)
	case "GET":
		uploaderGet(w, r)
	}
}

func main() {
	flag.StringVar(&conf.bind,        "l", "0.0.0.0:8080", "Address to bind to (default: 0.0.0.0:8080)")
	flag.StringVar(&conf.baseuri,     "b", "http://127.0.0.1:8080", "Base URI to use for links (default: http://127.0.0.1:8080)")
	flag.StringVar(&conf.filepath,    "f", "./files", "Path to save files to (default: ./files)")
	flag.StringVar(&conf.metapath,    "m", "./meta", "Path to save metadata to (default: ./meta)")
	flag.StringVar(&conf.filectx,     "c", "/f/", "Context to serve files from (default: /f/)")
	flag.StringVar(&conf.metactx,     "d", "/m/", "Context to serve metadata from (default: /m/)")
	flag.StringVar(&conf.rootdir,     "r", "./static", "Root directory (default: ./static)")
	flag.StringVar(&conf.templatedir, "t", "./templates", "Templates directory (default: ./templates)")
	flag.Int64Var(&conf.maxsize,      "s", 30064771072, "Maximum file size (default: 28Gib)")
	flag.Int64Var(&conf.expiry,       "e", 86400, "Link expiration time (default: 24h)")

	flag.Parse()

	http.HandleFunc("/", uploader)
	http.Handle(conf.filectx, http.StripPrefix(conf.filectx, http.FileServer(http.Dir(conf.filepath))))
	http.Handle(conf.metactx, http.StripPrefix(conf.metactx, http.FileServer(http.Dir(conf.metapath))))
	http.ListenAndServe("0.0.0.0:8080", nil)
}
