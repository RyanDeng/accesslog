package accesslog

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	logBufMax  = 1 << 13 // 8KB
	bodyBufMax = 1 << 12 // 4KB
)

var (
	logbufpool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, logBufMax))
		},
	}
	bodybufpool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, bodyBufMax))
		},
	}
	logging  logger
	reqBody  int32 = 0
	respBody int32 = 0
)

type Conf struct {
	Filename     string `json:"filename"`
	RequestBody  bool   `json:"request_body"`
	ResponseBody bool   `json:"response_body"`
	BufSize      int    `json:"buf_size"` // memory buf size of the data pending to write disk
}

// switch recording request body at runtime
func SwitchReqBody(b bool) {
	if b {
		atomic.StoreInt32(&reqBody, 1)
	} else {
		atomic.StoreInt32(&reqBody, 0)
	}
}

// switch recording response body at runtime
func SwitchRespBody(b bool) {
	if b {
		atomic.StoreInt32(&respBody, 1)
	} else {
		atomic.StoreInt32(&respBody, 0)
	}
}

func Handler(cfg *Conf, h http.Handler) http.Handler {
	var err error
	logging, err = newAsyncFileLogger(cfg)
	if err != nil {
		panic(err)
	}
	if cfg.RequestBody {
		reqBody = 1
	}
	if cfg.ResponseBody {
		respBody = 1
	}

	return &handler{
		handler: h,
	}
}

func Flush() error {
	if logging != nil {
		return logging.Close()
	}
	return nil
}

type HealthStat struct {
	LoggerBufferSize int
}

func FetchHealthStat() HealthStat {
	queueSize := 0
	if logging != nil {
		queueSize = logging.QueueBufferSize()
	}
	return HealthStat{
		LoggerBufferSize: queueSize,
	}
}

type handler struct {
	handler http.Handler
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	t := time.Now()

	// wrap req body
	reqbodybuf := bodybufpool.Get().(*bytes.Buffer)
	reqbodybuf.Reset()
	defer bodybufpool.Put(reqbodybuf)
	lr := newLogReqBody(req.Body, reqbodybuf, atomic.LoadInt32(&reqBody) == 1 && canRecordBody(req.Header))
	req.Body = lr

	// wrap ResponseWriter
	respbodybuf := bodybufpool.Get().(*bytes.Buffer)
	respbodybuf.Reset()
	defer bodybufpool.Put(respbodybuf)

	lw := newResponseWriter(w, respbodybuf, atomic.LoadInt32(&respBody) == 1)
	url := *req.URL
	h.handler.ServeHTTP(lw, req)
	logBuf := fmtLog(req, url, t, lr, lw)
	logging.Log(logBuf)
}

func fmtLog(req *http.Request, u url.URL, t time.Time, lr logReqBody, lw logResponseWriter) *bytes.Buffer {
	elapsed := time.Now().Sub(t)
	buf := logbufpool.Get().(*bytes.Buffer)
	buf.Reset()

	// now
	buf.WriteString(strconv.FormatInt(t.UnixNano(), 10))
	buf.WriteByte('\t')

	// method
	buf.WriteString(req.Method)
	buf.WriteByte('\t')

	// uri
	buf.WriteString(u.RequestURI())
	buf.WriteByte('\t')

	// req header
	buf.WriteByte('{')
	buf.WriteString(fmtHeader("Content-Length", req.ContentLength))
	buf.WriteByte(',')
	buf.WriteString(fmtHeader("Host", req.Host))
	buf.WriteByte(',')
	buf.WriteString(fmtHeader("IP", req.RemoteAddr))
	kvs, sorter := sortedKeyValues(req.Header)
	for _, kv := range kvs {
		if len(kv.values) > 0 {
			buf.WriteByte(',')
			buf.WriteString(fmtHeader(http.CanonicalHeaderKey(kv.key), kv.values[0]))
		}
	}
	headerSorterPool.Put(sorter)
	buf.WriteByte('}')
	buf.WriteByte('\t')

	// req body
	reqBodySize := len(lr.Body())
	if reqBodySize > 0 {
		if req.ContentLength != int64(reqBodySize) {
			buf.WriteString("{too large to display}")
		} else {
			for _, bb := range lr.Body() {
				if bb == '\n' {
					continue
				}
				buf.WriteByte(bb)
			}
		}
	} else {
		buf.WriteString("{no data}")
	}
	buf.WriteByte('\t')

	// status
	buf.WriteString(strconv.FormatInt(int64(lw.StatusCode()), 10))
	buf.WriteByte('\t')

	// resp header
	buf.WriteByte('{')
	kvs, sorter = sortedKeyValues(lw.Header())
	for i, kv := range kvs {
		if len(kv.values) > 0 {
			if i != 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(fmtHeader(http.CanonicalHeaderKey(kv.key), kv.values[0]))
		}
	}
	headerSorterPool.Put(sorter)
	buf.WriteByte('}')
	buf.WriteByte('\t')

	// resp body
	respBodySize := len(lw.Body())
	if respBodySize > 0 {
		if lw.Size() != respBodySize {
			buf.WriteString("{too large to display}")
		} else {
			if lw.Body()[respBodySize-1] == '\n' {
				buf.Write(lw.Body()[:respBodySize-1])
			} else {
				buf.Write(lw.Body())
			}
		}

	} else {
		buf.WriteString("{no data}")
	}
	buf.WriteByte('\t')

	// content-length
	buf.WriteString(strconv.FormatInt(int64(lw.Size()), 10))
	buf.WriteByte('\t')

	// elapsed time
	buf.WriteString(strconv.FormatInt(int64(elapsed/time.Microsecond), 10))
	buf.WriteByte('\n')

	return buf
}

func fmtHeader(key string, value interface{}) string {
	return fmt.Sprintf(`"%v":"%v"`, key, value)
}

func canRecordBody(header http.Header) bool {
	ct := header.Get("Content-type")
	if i := strings.IndexByte(ct, ';'); i != -1 {
		ct = ct[:i]
	}
	switch ct {
	case "application/json":
		return true
	case "text/plain":
		return true
	case "application/x-www-form-urlencoded":
		return true
	case "application/xml", "text/xml":
		return true
	default:
		return false
	}
}
