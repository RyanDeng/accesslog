package accesslog

import (
	"bytes"
	"net/http"
)

type logResponseWriter interface {
	http.ResponseWriter
	StatusCode() int
	Size() int
	Body() []byte
}

type commonLogRespWriter struct {
	w      http.ResponseWriter
	status int
	size   int
	buf    *bytes.Buffer

	firstWrite bool
	recordBody bool
}

func newResponseWriter(w http.ResponseWriter, buf *bytes.Buffer, recordBody bool) logResponseWriter {
	return &commonLogRespWriter{
		w:          w,
		firstWrite: true,
		buf:        buf,
		recordBody: recordBody,
	}
}

func (c *commonLogRespWriter) StatusCode() int {
	if c.status == 0 {
		return 200
	}
	return c.status
}

func (c *commonLogRespWriter) Size() int {
	return c.size
}
func (c *commonLogRespWriter) Header() http.Header {
	return c.w.Header()
}

func (c *commonLogRespWriter) Write(b []byte) (int, error) {
	if c.firstWrite {
		c.firstWrite = false
		c.recordBody = c.recordBody && canRecordBody(c.w.Header())
	}
	if c.recordBody && c.size+len(b) <= c.buf.Cap() {
		c.buf.Write(b)
	}

	size, err := c.w.Write(b)
	c.size += size
	return size, err
}

func (c *commonLogRespWriter) WriteHeader(code int) {
	c.status = code
	c.w.WriteHeader(code)
}
func (c *commonLogRespWriter) Body() []byte {
	return c.buf.Bytes()
}
