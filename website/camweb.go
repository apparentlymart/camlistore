package main

import (
	"bytes"
	"flag"
	"fmt"
	"http"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"template"
)

const defaultAddr = ":31798" // default webserver address

var (
	httpAddr = flag.String("http", defaultAddr, "HTTP service address (e.g., '"+defaultAddr+"')")
	root     = flag.String("root", "", "Website root (parent of 'static', 'content', and 'tmpl")
	gitwebScript = flag.String("gitwebscript", "/usr/lib/cgi-bin/gitweb.cgi", "Path to gitweb.cgi, or blank to disable.")
	gitwebFiles = flag.String("gitwebfiles", "/usr/share/gitweb", "Path to gitweb's static files.")
	pageHtml, errorHtml *template.Template
)

var fmap = template.FormatterMap{
	"":         textFmt,
	"html":     htmlFmt,
	"html-esc": htmlEscFmt,
}

// Template formatter for "" (default) format.
func textFmt(w io.Writer, format string, x ...interface{}) {
	writeAny(w, false, x[0])
}

// Template formatter for "html" format.
func htmlFmt(w io.Writer, format string, x ...interface{}) {
	writeAny(w, true, x[0])
}


// Template formatter for "html-esc" format.
func htmlEscFmt(w io.Writer, format string, x ...interface{}) {
	var buf bytes.Buffer
	writeAny(&buf, false, x[0])
	template.HTMLEscape(w, buf.Bytes())
}

// Write anything to w; optionally html-escaped.
func writeAny(w io.Writer, html bool, x interface{}) {
	switch v := x.(type) {
	case []byte:
		writeText(w, v, html)
	case string:
		writeText(w, []byte(v), html)
	default:
		if html {
			var buf bytes.Buffer
			fmt.Fprint(&buf, x)
			writeText(w, buf.Bytes(), true)
		} else {
			fmt.Fprint(w, x)
		}
	}
}

// Write text to w; optionally html-escaped.
func writeText(w io.Writer, text []byte, html bool) {
	if html {
		template.HTMLEscape(w, text)
		return
	}
	w.Write(text)
}


func applyTemplate(t *template.Template, name string, data interface{}) []byte {
	var buf bytes.Buffer
	if err := t.Execute(data, &buf); err != nil {
		log.Printf("%s.Execute: %s", name, err)
	}
	return buf.Bytes()
}

func servePage(w http.ResponseWriter, title, subtitle string, content []byte) {
	d := struct {
		Title    string
		Subtitle string
		Content  []byte
	}{
		title,
		subtitle,
		content,
	}

	if err := pageHtml.Execute(&d, w); err != nil {
		log.Printf("godocHTML.Execute: %s", err)
	}
}

func readTemplate(name string) *template.Template {
	fileName := path.Join(*root, "tmpl", name)
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Exitf("ReadFile %s: %v", fileName, err)
	}
	t, err := template.Parse(string(data), fmap)
	if err != nil {
		log.Exitf("%s: %v", fileName, err)
	}
	return t
}

func readTemplates() {
	pageHtml = readTemplate("page.html")
	errorHtml = readTemplate("error.html")
}

func serveError(w http.ResponseWriter, r *http.Request, relpath string, err os.Error) {
	contents := applyTemplate(errorHtml, "errorHtml", err) // err may contain an absolute path!
	w.WriteHeader(http.StatusNotFound)
        servePage(w, "File "+relpath, "", contents)
}

func mainHandler(rw http.ResponseWriter, req *http.Request) {
	relPath := req.URL.Path[1:] // serveFile URL paths start with '/'
	if strings.Contains(relPath, "..") {
		return
	}
	if relPath == "" {
		relPath = "index.html"
	}
	absPath := path.Join(*root, "content", relPath)

	fi, err := os.Lstat(absPath)
	if err != nil {
		log.Print(err)
		serveError(rw, req, relPath, err)
		return
	}

	switch {
	case fi.IsRegular():
		serveFile(rw, req, relPath, absPath)
	}
}

func serveFile(rw http.ResponseWriter, req *http.Request, relPath, absPath string) {
	data, err := ioutil.ReadFile(absPath)
	if err != nil {
		serveError(rw, req, absPath, err)
                return
	}
	servePage(rw, "", "", []byte(data))	
}

type gitwebHandler struct{
	Cgi      http.Handler
	Static   http.Handler
}

func (h *gitwebHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath == "/code/" ||
		strings.HasPrefix(r.URL.RawPath, "/code/?") {
		h.Cgi.ServeHTTP(rw, r)
	} else {
		h.Static.ServeHTTP(rw, r)
	}
}

func main() {
	flag.Parse()
	readTemplates()

	mux := http.DefaultServeMux
	mux.Handle("/favicon.ico", http.FileServer(path.Join(*root, "static"), "/"))
	mux.Handle("/static/", http.FileServer(path.Join(*root, "static"), "/static/"))
	mux.Handle("/test.cgi", &CgiHandler{ExecutablePath: path.Join(*root, "test.cgi")})
	mux.Handle("/code", http.RedirectHandler("/code/", http.StatusFound))
	if *gitwebScript != "" {
		mux.Handle("/code/", &gitwebHandler{
		Cgi: &CgiHandler{ExecutablePath: *gitwebScript},
		Static: http.FileServer(*gitwebFiles, "/code/"),
		})
		// For now, also register this alias for convenience.
		// Some distros' default gitweb config links to resources
		// in the current directory but Lucid links to /gitweb/ instead.
		// TODO: specify a gitweb config file.
		mux.Handle("/gitweb/", http.FileServer(*gitwebFiles, "/gitweb/"))
	}
	mux.HandleFunc("/", mainHandler)

	if err := http.ListenAndServe(*httpAddr, mux); err != nil {
		log.Exitf("ListenAndServe %s: %v", *httpAddr, err)
	}
}
