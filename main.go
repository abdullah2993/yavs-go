package main

import (
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	refreshDur  = flag.Duration("refreshInterval", 0, "Duration(supports go duration format) after which to fetch the data URL if set to 0 then auto refresh will be disabled. To refresh manually use the refresh endpoint")
	refreshPath = flag.String("refreshPath", "/refresh", "refresh endpoint(with a / prefix) for the server, if you use the default you can not host a vanity package with name refresh")
	addr        = flag.String("addr", "localhost:8080", "address to listen on")
)

type vanity struct {
	Pkg string
	VCS string
	URL string
}

var (
	vmap map[string]*vanity = make(map[string]*vanity)
	mu   sync.RWMutex
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("yvas-go: ")
	flag.Usage = usage
	flag.Parse()

	if flag.NArg() != 2 {
		usage()
	}

	dataURL := flag.Arg(1)

	if *refreshDur == 0 {
		log.Printf("warning: refesh durations set to zero. so cache won't refresh automatically")
	} else {
		go refreshCacheLoop(dataURL)
	}

	if *refreshPath == "" {
		log.Printf("warning: refesh path not set. you won't be able to refresh the cache manually")
	}

	n, err := refreshCache(dataURL)
	if err != nil {
		log.Fatalf("unable to load the intial packages: %v", err)
	}

	log.Printf("%d packages loaded", n)

	if *refreshPath != "" {
		http.HandleFunc(*refreshPath, func(w http.ResponseWriter, r *http.Request) {
			n, err := refreshCache(dataURL)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "%d packages refreshed", n)
		})
	}

	http.HandleFunc("/", handle)
	http.ListenAndServe(*addr, nil)
}

func handle(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	info, ok := vmap[strings.TrimPrefix(r.URL.Path, "/")]
	mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	tmpl.Execute(w, info)
}

func refreshCacheLoop(url string) {
	for {
		select {
		case <-time.After(*refreshDur):
			n, err := refreshCache(url)
			if err != nil {
				log.Printf("unable to fetch cache data: %v\n", err)
				continue
			}
			log.Printf("%d packages refreshed", n)
		}
	}
}

func refreshCache(url string) (int, error) {
	log.Printf("refreshing cache using :%s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("unable to fetch cache data: %v\n", err)
		return 0, err
	}
	// TODO(abdullah) this could potentially load a lot of data, limit data here
	vanityList := []*vanity{}
	for {
		newVanity := new(vanity)
		_, err = fmt.Fscanf(resp.Body, "%s %s %s", &newVanity.Pkg, &newVanity.VCS, &newVanity.URL)

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("unable to parse entry: %v\n", err)
			continue
		}
		vanityList = append(vanityList, newVanity)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, v := range vanityList {
		vmap[v.Pkg] = v
	}
	return len(vanityList), nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: yavs-go domain dataUrl\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
	os.Exit(2)
}

var tmpl = template.Must(template.New("pkg").Parse(`<html>
<head>
	<title>{{.Pkg}}-{{.VCS}}</title>
	<meta name="go-import" content="{{.Pkg}} {{.VCS}} {{.URL}}">
</head>
<body>
	{{.Pkg}} is hosted on <a href="{{.URL}}">{{.Pkg}}-{{.VCS}}</a>
</body>
</html>
`))
