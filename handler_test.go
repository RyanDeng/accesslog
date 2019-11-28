package accesslog

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"bufio"
	"github.com/stretchr/testify/assert"
)

func TestHandler(t *testing.T) {
	ast := assert.New(t)

	filename := fmt.Sprintf("%saccesslog%d.log", os.TempDir(), time.Now().Unix())
	fmt.Println("tempfile is ", filename)
	cfg := &Conf{
		Filename:     filename,
		RequestBody:  false,
		ResponseBody: false,
	}
	defer os.Remove(filename)
	ts := httptest.NewServer(Handler(cfg, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, err := ioutil.ReadAll(req.Body)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		w.Header().Add("Content-type", "application/json")
		w.Write([]byte(`{"name": "peter", "age": 12}`))
	})))
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", strings.NewReader(`{"user": "admin"}`))
	ast.Nil(err)
	resp.Body.Close()
	ast.Equal(200, resp.StatusCode)
	SwitchReqBody(true)
	SwitchRespBody(true)
	resp, err = http.Post(ts.URL, "application/json", strings.NewReader(`{"user": "admin"}`))
	ast.Nil(err)
	resp.Body.Close()
	// 换行,会自动去掉
	resp, err = http.Post(ts.URL, "application/json", strings.NewReader(`{"user": 
"admin"}`))
	ast.Nil(err)
	resp.Body.Close()
	ast.Equal(200, resp.StatusCode)
	SwitchReqBody(false)
	SwitchRespBody(false)
	resp, err = http.Post(ts.URL, "application/json", strings.NewReader(`{"user": "admin"}`))
	ast.Nil(err)
	resp.Body.Close()
	ast.Equal(200, resp.StatusCode)
	Flush()

	f, err := os.Open(filename)
	ast.Nil(err)
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := make([]string, 0, 3)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	err = scanner.Err()
	ast.Nil(err)

	ast.Equal(4, len(lines))
	checkLine(ast, lines[0], "{no data}", "{no data}")
	checkLine(ast, lines[1], `{"user": "admin"}`, `{"name": "peter", "age": 12}`)
	checkLine(ast, lines[2], `{"user": "admin"}`, `{"name": "peter", "age": 12}`)
	checkLine(ast, lines[3], "{no data}", "{no data}")

}

func checkLine(ast *assert.Assertions, line, req, resp string) {
	arr := strings.Split(line, "\t")

	ast.Equal(arr[4], req)
	ast.Equal(arr[7], resp)
}
