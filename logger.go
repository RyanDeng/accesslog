package accesslog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// the max log size
const maxSize int64 = 1024 * 1024 * 1800

type logger interface {
	Log(buf *bytes.Buffer) error
	Close() error
}

type asyncFileLogger struct {
	filename string
	file     *os.File
	queue    chan *bytes.Buffer
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
	ret := &asyncFileLogger{
		filename: cfg.Filename,
		file:     f,
		queue:    make(chan *bytes.Buffer, 10000),
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
	l.queue <- buf
	return nil
}

func (l *asyncFileLogger) loop() {
	for {
		select {
		case buf := <-l.queue:
			l.writeFile(buf)
		case <-l.close:
			return
		}
	}
}

func (l *asyncFileLogger) writeFile(buf *bytes.Buffer) {
	if int64(buf.Len())+l.sizeNum > maxSize {
		l.rotateLog()
	}
	n, err := l.file.Write(buf.Bytes())
	logbufpool.Put(buf)
	if err != nil {
		panic("cannot write access log")
	}
	l.sizeNum += int64(n)
}

func (l *asyncFileLogger) rotateLog() {
	l.file.Sync()
	l.file.Close()
	err := os.Rename(l.filename, fmt.Sprintf("%s-%s", l.filename, time.Now().Format("20060102150405")))
	if err != nil {
		panic("fail to rotate log")
	}

	l.file, err = openAppendFile(l.filename)
	if err != nil {
		panic(err)
	}
	stat, err := l.file.Stat()
	if err != nil {
		panic(err)
	}
	l.sizeNum = stat.Size()
}

func (l *asyncFileLogger) Close() error {
	l.close <- struct{}{}

	for buf := range l.queue {
		l.writeFile(buf)
		if len(l.queue) == 0 {
			break
		}
	}

	l.file.Sync()
	return l.file.Close()
}
