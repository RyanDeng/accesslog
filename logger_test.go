package accesslog

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testLogName = "./tt.log"
)

func TestLogger(t *testing.T) {
	ast := assert.New(t)
	deleteTestFile()
	l, err := newAsyncFileLogger(&Conf{Filename: testLogName})
	ast.Nil(err)

	for i := 0; i < 100; i++ {
		l.Log(bytes.NewBufferString(fmt.Sprintf("haha%d\n", i)))
	}

	err = l.Close()
	ast.Nil(err)

	f, err := os.Open(testLogName)
	ast.Nil(err)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		ast.EqualValues(fmt.Sprintf("haha%d", count), scanner.Text())
		count++
	}
	ast.EqualValues(100, count)

}

func deleteTestFile() {
	os.Remove(testLogName)
}
