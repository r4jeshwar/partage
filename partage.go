package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"gopkg.in/ini.v1"
)

type templatedata struct {
	Links   []string
	Size    string
	Maxsize string
}

type metadata struct {
	Filename string
	Size     int64
	Expiry   int64
}

var conf struct {
	user     string
	group    string
	chroot   string
	bind     string
	baseuri  string
	rootdir  string
	tmplpath string
	filepath string
	metapath string
	filectx  string
	metactx  string
	maxsize  int64
	expiry   int64
}

var verbose bool

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
		Size:     size,
		Expiry:   time.Now().Unix() + expiry,
	}

	if verbose {
		log.Printf("Saving metadata for %s in %s", meta.Filename, conf.metapath+"/"+meta.Filename+".json")
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
	t, err := template.ParseFiles(conf.tmplpath + "/" + f)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if verbose {
		log.Printf("Serving template %s", t.Name())
	}

	err = t.Execute(w, d)
	if err != nil {
		fmt.Println(err)
	}
}

func uploaderPut(w http.ResponseWriter, r *http.Request) {
	/* limit upload size */
	if r.ContentLength > conf.maxsize {
		http.Error(w, "File is too big", http.StatusRequestEntityTooLarge)
	}

	tmp, _ := ioutil.TempFile(conf.filepath, "*"+path.Ext(r.URL.Path))
	f, err := os.Create(tmp.Name())
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()

	if verbose {
		log.Printf("Writing %d bytes to %s", r.ContentLength, tmp)
	}

	if err = writefile(f, r.Body, r.ContentLength); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
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
			http.Error(w, "File is too big", http.StatusRequestEntityTooLarge)
			return
		}

		post, err := h.Open()
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		defer post.Close()

		tmp, _ := ioutil.TempFile(conf.filepath, "*"+path.Ext(h.Filename))
		f, err := os.Create(tmp.Name())
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		if err = writefile(f, post, h.Size); err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			defer os.Remove(tmp.Name())
			return
		}

		writemeta(tmp.Name(), conf.expiry)

		link := conf.baseuri + conf.filectx + filepath.Base(tmp.Name())
		links = append(links, link)
	}

	if r.PostFormValue("output") == "html" {
		data := templatedata{Links: links}
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
		data := templatedata{Maxsize: humanize.IBytes(uint64(conf.maxsize))}
		servetemplate(w, "/index.html", data)
		return
	}

	if verbose {
		log.Printf("Serving file %s", conf.rootdir+filename)
	}

	http.ServeFile(w, r, conf.rootdir+filename)
}

func uploader(w http.ResponseWriter, r *http.Request) {
	if verbose {
		log.Printf("%s: <%s> %s %s %s", r.Host, r.RemoteAddr, r.Method, r.RequestURI, r.Proto)
	}

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
	var file string
	flag.StringVar(&file, "f", "", "Configuration file")
	flag.BoolVar(&verbose, "v", false, "Verbose logging")
	flag.Parse()

	/* default values */
	conf.bind = "0.0.0.0:8080"
	conf.baseuri = "http://127.0.0.1:8080"
	conf.rootdir = "/htdocs"
	conf.tmplpath = "/htdocs/templates"
	conf.filepath = "/htdocs/files"
	conf.metapath = "/htdocs/meta"
	conf.filectx = "/f/"
	conf.metactx = "/m/"
	conf.maxsize = 34359738368
	conf.expiry = 86400

	if file != "" {
		if verbose {
			log.Printf("Reading configuration %s", file)
		}

		cfg, err := ini.Load(file)
		if err != nil {
			fmt.Println(err)
			return
		}

		conf.bind = cfg.Section("").Key("bind").String()
		conf.user = cfg.Section("").Key("user").String()
		conf.group = cfg.Section("").Key("group").String()
		conf.baseuri = cfg.Section("").Key("baseuri").String()
		conf.filepath = cfg.Section("").Key("filepath").String()
		conf.metapath = cfg.Section("").Key("metapath").String()
		conf.filectx = cfg.Section("").Key("filectx").String()
		conf.metactx = cfg.Section("").Key("metactx").String()
		conf.rootdir = cfg.Section("").Key("rootdir").String()
		conf.chroot = cfg.Section("").Key("chroot").String()
		conf.tmplpath = cfg.Section("").Key("tmplpath").String()
		conf.maxsize, _ = cfg.Section("").Key("maxsize").Int64()
		conf.expiry, _ = cfg.Section("").Key("expiry").Int64()
	}

	if verbose {
		log.Printf("Applied configuration:\n%s", conf)
	}

	if conf.chroot != "" {
		if verbose {
			log.Printf("Changing root to %s", conf.chroot)
		}
		syscall.Chroot(conf.chroot)
	}

	if conf.user != "" {
		u, err := user.Lookup(conf.user)
		if err != nil {
			fmt.Println(err)
			return
		}

		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)

		if conf.group != "" {
			g, err := user.LookupGroup(conf.group)
			if err != nil {
				fmt.Println(err)
				return
			}
			gid, _ = strconv.Atoi(g.Gid)
		}

		if verbose {
			log.Printf("Dropping privileges to %s", conf.user)
		}

		syscall.Setuid(uid)
		syscall.Setgid(gid)
	}

	http.HandleFunc("/", uploader)
	http.Handle(conf.filectx, http.StripPrefix(conf.filectx, http.FileServer(http.Dir(conf.filepath))))

	if verbose {
		log.Printf("Listening on %s", conf.bind)
	}

	http.ListenAndServe(conf.bind, nil)
}
