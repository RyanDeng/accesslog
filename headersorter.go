package accesslog

import (
	"net/http"
	"sort"
	"sync"
)

// copied from go/src/net/http/header.go

type keyValues struct {
	key    string
	values []string
}
type headerSorter struct {
	kvs []keyValues
}

func (s *headerSorter) Len() int           { return len(s.kvs) }
func (s *headerSorter) Swap(i, j int)      { s.kvs[i], s.kvs[j] = s.kvs[j], s.kvs[i] }
func (s *headerSorter) Less(i, j int) bool { return s.kvs[i].key < s.kvs[j].key }

var headerSorterPool = sync.Pool{
	New: func() interface{} { return new(headerSorter) },
}

func sortedKeyValues(h http.Header) (kvs []keyValues, hs *headerSorter) {
	hs = headerSorterPool.Get().(*headerSorter)
	if cap(hs.kvs) < len(h) {
		hs.kvs = make([]keyValues, 0, len(h))
	}
	kvs = hs.kvs[:0]
	for k, vv := range h {
		kvs = append(kvs, keyValues{k, vv})
	}
	hs.kvs = kvs
	sort.Sort(hs)
	return kvs, hs
}
