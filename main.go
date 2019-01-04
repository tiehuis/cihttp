package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru"
	"github.com/karrick/godirwalk"
)

type server struct {
	root  string
	cache *lru.Cache
	quiet bool
}

var resolveComponentScratchBuffer []byte

// Search a directory `path` for a file `childQuery` while ignoring case, which satisfies `predicate`.
// Returns an empty string if no file is found.
func (s *server) resolveComponent(dirPath, childQuery string, predicate func(os.FileInfo) bool) (string, error) {
	var err error
	var children []string

	key := strings.ToLower(dirPath)
	if s.cache != nil && s.cache.Contains(key) {
		entry, _ := s.cache.Get(key)
		children = entry.([]string)
	} else {
		if resolveComponentScratchBuffer == nil {
			resolveComponentScratchBuffer = make([]byte, 2*os.Getpagesize())
		}

		children, err = godirwalk.ReadDirnames(dirPath, resolveComponentScratchBuffer)
		if err != nil {
			return "", err
		}

		sort.Strings(children)
		if s.cache != nil {
			s.cache.Add(key, children)
		}
	}

	normSegment := ""
	for _, child := range children {
		if strings.EqualFold(child, childQuery) {
			childPath := filepath.Join(dirPath, child)
			fi, err := os.Stat(childPath)
			if err != nil {
				return "", err
			}
			if predicate(fi) {
				normSegment = child
				break
			}
		}
	}
	if normSegment == "" {
		return "", nil
	}

	return filepath.Join(dirPath, normSegment), nil
}

func (s *server) serveFile(w http.ResponseWriter, r *http.Request) {
	var err error

	start := time.Now()

	path := filepath.Clean(r.URL.Path)
	allDirs, file := filepath.Split(path)
	dirs := strings.Split(strings.TrimSuffix(allDirs, "/"), string(filepath.Separator))[1:]

	if file == "" {
		file = "index.html"
	}

	resolvedPath := s.root
	for _, dirChild := range dirs {
		resolvedPath, err = s.resolveComponent(resolvedPath, dirChild, func(fi os.FileInfo) bool {
			return fi.IsDir()
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("[500] %s (%v)", err, time.Now().Sub(start))
			return
		}
		if resolvedPath == "" {
			w.WriteHeader(http.StatusNotFound)
			log.Printf("[404] %s (%v)", filepath.Join(s.root, path), time.Now().Sub(start))
			return
		}
	}

	resolvedPath, err = s.resolveComponent(resolvedPath, file, func(fi os.FileInfo) bool {
		return !fi.IsDir()
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("[500] %s (%v)", err, time.Now().Sub(start))
		return
	}
	if resolvedPath == "" {
		w.WriteHeader(http.StatusNotFound)
		log.Printf("[404] %s (%v)", filepath.Join(s.root, path), time.Now().Sub(start))
		return
	}

	if !s.quiet {
		log.Printf("[200] %s (%v)", resolvedPath, time.Now().Sub(start))
	}
	http.ServeFile(w, r, resolvedPath)
}

func main() {
	var err error

	var address = flag.String("address", ":8000", "address to serve on")
	var cacheSize = flag.Int("cache-size", 128, "number of directory entries cached, disabled=0")
	var quiet = flag.Bool("quiet", false, "don't output any information")
	flag.Parse()

	s := &server{
		root:  ".",
		cache: nil,
		quiet: *quiet,
	}
	if flag.Arg(0) != "" {
		s.root = flag.Arg(0)
	}
	if *cacheSize != 0 {
		s.cache, err = lru.New(*cacheSize)
		if err != nil {
			log.Fatal(err)
		}
	}

	fi, err := os.Stat(s.root)
	if err != nil {
		log.Fatal(err)
	}
	if !fi.IsDir() {
		log.Fatal(err)
	}

	log.Printf("info: serving %s on %s with %d-item cache", s.root, *address, *cacheSize)
	http.HandleFunc("/", s.serveFile)
	log.Fatal(http.ListenAndServe(*address, nil))
}
