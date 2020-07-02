package accesslog

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/enriquebris/goconcurrentqueue"
)

// the max log size
const maxSize int64 = 1024 * 1024 * 1800
const bufioSize = 4096

type logger interface {
	Log(buf *bytes.Buffer) error
	Close() error
	QueueBufferSize() int // 内部队列的长度，方便监控使用
}

type asyncFileLogger struct {
	filename string
	bufFile  *bufio.Writer
	file     *os.File
	queue    goconcurrentqueue.Queue
	close    chan struct{}
	sizeNum  int64
}

func newAsyncFileLogger(cfg *Conf) (logger, error) {
	dir, _ := filepath.Split(cfg.Filename)
	os.MkdirAll(dir, 0755)

	f, err := openAppendFile(cfg.Filename)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if cfg.BufSize == 0 {
		cfg.BufSize = bufioSize
	}
	ret := &asyncFileLogger{
		filename: cfg.Filename,
		file:     f,
		bufFile:  bufio.NewWriterSize(f, cfg.BufSize),
		queue:    goconcurrentqueue.NewFIFO(),
		close:    make(chan struct{}),
		sizeNum:  stat.Size(),
	}

	go ret.loop()

	return ret, nil
}

func openAppendFile(fileName string) (*os.File, error) {
	return os.OpenFile(fileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, os.ModeAppend|0666)
}

func (l *asyncFileLogger) Log(buf *bytes.Buffer) error {
	l.queue.Enqueue(buf)
	return nil
}

func (l *asyncFileLogger) loop() {
	for {
		select {
		case <-l.close:
			return
		default:
		}
		buf, err := l.queue.Dequeue()
		if err != nil {
			// 队列是空时返回error，sleep 一会儿
			time.Sleep(time.Millisecond * 10)
			continue
		}
		l.writeFile(buf.(*bytes.Buffer))
	}

}

func (l *asyncFileLogger) writeFile(buf *bytes.Buffer) {
	if int64(buf.Len())+l.sizeNum > maxSize {
		l.rotateLog()
	}
	n, err := l.bufFile.Write(buf.Bytes())
	logbufpool.Put(buf)
	if err != nil {
		panic("cannot write access log")
	}
	l.sizeNum += int64(n)
}

func (l *asyncFileLogger) rotateLog() {
	l.bufFile.Flush()
	l.file.Close()
	err := os.Rename(l.filename, fmt.Sprintf("%s-%s", l.filename, time.Now().Format("20060102150405")))
	if err != nil {
		panic("fail to rotate log")
	}

	l.file, err = openAppendFile(l.filename)
	if err != nil {
		panic(err)
	}
	l.bufFile = bufio.NewWriterSize(l.file, bufioSize)
	stat, err := l.file.Stat()
	if err != nil {
		panic(err)
	}
	l.sizeNum = stat.Size()
}

func (l *asyncFileLogger) QueueBufferSize() int {
	return l.queue.GetLen()
}

func (l *asyncFileLogger) Close() error {
	l.close <- struct{}{}

	for {
		buf, err := l.queue.Dequeue()
		if err != nil {
			goto Done
		}
		l.writeFile(buf.(*bytes.Buffer))
	}

Done:
	l.bufFile.Flush()
	return l.file.Close()
}

type streamLogger struct {
	io.Writer
}

func newStreamLogger(w io.Writer) logger {
	return &streamLogger{
		Writer: w,
	}
}

func (s *streamLogger) Log(buf *bytes.Buffer) error {
	_, err := s.Writer.Write(buf.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func (s *streamLogger) Close() error {
	return nil
}

func (s *streamLogger) QueueBufferSize() int {
	return 0
}
