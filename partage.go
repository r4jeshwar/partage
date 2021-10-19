package main

import (
	"fmt"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"time"
	"path"
	"syscall"
	"strconv"
	"path/filepath"
	"html/template"
	"encoding/json"

	"github.com/dustin/go-humanize"
	"github.com/vharitonsky/iniflags"
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
	user     string
	group    string
	baseuri  string
	filepath string
	metapath string
	rootdir  string
	chroot   string
	templatedir string
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
		Size: size,
		Expiry: time.Now().Unix() + expiry,
	}

	if verbose {
		log.Printf("Saving metadata for %s in %s", meta.Filename, conf.metapath + "/" + meta.Filename + ".json")
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

	if verbose {
		log.Printf("Serving file %s", conf.rootdir + filename)
	}

	http.ServeFile(w, r, conf.rootdir + filename)
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
	flag.StringVar(&conf.bind,        "bind",        "0.0.0.0:8080", "Address to bind to (default: 0.0.0.0:8080)")
	flag.StringVar(&conf.user,        "user",        "", "User to drop privileges to on startup (default: current user)")
	flag.StringVar(&conf.group,       "group",       "", "Group to drop privileges to on startup (default: user's group)")
	flag.StringVar(&conf.baseuri,     "baseuri",     "http://127.0.0.1:8080", "Base URI to use for links (default: http://127.0.0.1:8080)")
	flag.StringVar(&conf.filepath,    "filepath",    "./files", "Path to save files to (default: ./files)")
	flag.StringVar(&conf.metapath,    "metapath",    "./meta", "Path to save metadata to (default: ./meta)")
	flag.StringVar(&conf.filectx,     "filectx",     "/f/", "Context to serve files from (default: /f/)")
	flag.StringVar(&conf.metactx,     "metactx",     "/m/", "Context to serve metadata from (default: /m/)")
	flag.StringVar(&conf.rootdir,     "rootdir",     "./static", "Root directory (default: ./static)")
	flag.StringVar(&conf.chroot,      "chroot",      "", "Directory to chroot into upon starting (default: no chroot)")
	flag.StringVar(&conf.templatedir, "templatedir", "./templates", "Templates directory (default: ./templates)")
	flag.Int64Var(&conf.maxsize,      "maxsize",     30064771072, "Maximum file size (default: 28Gib)")
	flag.Int64Var(&conf.expiry,       "expiry",      86400, "Link expiration time (default: 24h)")

	iniflags.Parse()

	if verbose {
		log.Printf("Applied configuration:\n%s", conf)
	}

	if (conf.chroot != "") {
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
