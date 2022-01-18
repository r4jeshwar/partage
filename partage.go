package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"os/signal"
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
	listen     string
	baseuri  string
	rootdir  string
	tmplpath string
	filepath string
	metapath string
	filectx  string
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
		log.Printf("Writing %d bytes to %s", r.ContentLength, tmp.Name())
	}

	if err = writefile(f, r.Body, r.ContentLength); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		defer os.Remove(tmp.Name())
		return
	}
	writemeta(tmp.Name(), conf.expiry)

	resp := conf.baseuri + conf.filectx + filepath.Base(tmp.Name())
	w.Write([]byte(resp + "\r\n"))
}

func uploaderPost(w http.ResponseWriter, r *http.Request) {
	/* read 32Mb at a time */
	r.ParseMultipartForm(32 << 20)

	links := []string{}
	for _, h := range r.MultipartForm.File["file"] {
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

	switch r.PostFormValue("output") {
	case "html":
		data := templatedata{
			Maxsize: humanize.IBytes(uint64(conf.maxsize)),
			Links: links,
		}
		servetemplate(w, "/index.html", data)
	case "json":
		data, _ := json.Marshal(links)
		w.Write(data)
	default:
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

func parseconfig(file string) error {
	cfg, err := ini.Load(file)
	if err != nil {
		return err
	}

	conf.listen = cfg.Section("").Key("listen").String()
	conf.user = cfg.Section("").Key("user").String()
	conf.group = cfg.Section("").Key("group").String()
	conf.baseuri = cfg.Section("").Key("baseuri").String()
	conf.filepath = cfg.Section("").Key("filepath").String()
	conf.metapath = cfg.Section("").Key("metapath").String()
	conf.filectx = cfg.Section("").Key("filectx").String()
	conf.rootdir = cfg.Section("").Key("rootdir").String()
	conf.chroot = cfg.Section("").Key("chroot").String()
	conf.tmplpath = cfg.Section("").Key("tmplpath").String()
	conf.maxsize, _ = cfg.Section("").Key("maxsize").Int64()
	conf.expiry, _ = cfg.Section("").Key("expiry").Int64()

	return nil
}

func usergroupids(username string, groupname string) (int, int, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return -1, -1, err
	}

	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	if conf.group != "" {
		g, err := user.LookupGroup(groupname)
		if err != nil {
			return uid, -1, err
		}
		gid, _ = strconv.Atoi(g.Gid)
	}

	return uid, gid, nil
}

func main() {
	var err error
	var configfile string
	var listener net.Listener

	/* default values */
	conf.listen = "0.0.0.0:8080"
	conf.baseuri = "http://127.0.0.1:8080"
	conf.rootdir = "static"
	conf.tmplpath = "templates"
	conf.filepath = "files"
	conf.metapath = "meta"
	conf.filectx = "/f/"
	conf.maxsize = 34359738368
	conf.expiry = 86400

	flag.StringVar(&configfile, "f", "", "Configuration file")
	flag.BoolVar(&verbose, "v", false, "Verbose logging")
	flag.Parse()

	if configfile != "" {
		if verbose {
			log.Printf("Reading configuration %s", configfile)
		}
		parseconfig(configfile)
	}

	if conf.chroot != "" {
		if verbose {
			log.Printf("Changing root to %s", conf.chroot)
		}
		syscall.Chroot(conf.chroot)
	}

	if conf.listen[0] == '/' {
		/* Remove any stale socket */
		os.Remove(conf.listen)
		if listener, err = net.Listen("unix", conf.listen); err != nil {
			log.Fatal(err)
		}
		defer listener.Close()

		/*
		 * Ensure unix socket is removed on exit.
		 * Note: this might not work when dropping privileges…
		 */
		defer os.Remove(conf.listen)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM)
		go func() {
			_ = <-sigs
			listener.Close()
			if err = os.Remove(conf.listen); err != nil {
				log.Fatal(err)
			}
			os.Exit(0)
		}()
	} else {
		if listener, err = net.Listen("tcp", conf.listen); err != nil {
			log.Fatal(err)
		}
		defer listener.Close()
	}

	if conf.user != "" {
		if verbose {
			log.Printf("Dropping privileges to %s", conf.user)
		}
		uid, gid, err := usergroupids(conf.user, conf.group)
		if err != nil {
			log.Fatal(err)
		}

		if listener.Addr().Network() == "unix" {
			os.Chown(conf.listen, uid, gid)
		}

		syscall.Setuid(uid)
		syscall.Setgid(gid)
	}

	http.HandleFunc("/", uploader)
	http.Handle(conf.filectx, http.StripPrefix(conf.filectx, http.FileServer(http.Dir(conf.filepath))))

	if verbose {
		log.Printf("Listening on %s", conf.listen)
	}

	if listener.Addr().Network() == "unix" {
		err = fcgi.Serve(listener, nil)
		log.Fatal(err) /* NOTREACHED */
	}

	err = http.Serve(listener, nil)
	log.Fatal(err) /* NOTREACHED */
}
