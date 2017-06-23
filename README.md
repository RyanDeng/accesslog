# accesslog
A middleware to record the access logs for http roundtrip. The recording proccess is asynchronous so you don't need to worry about big log contents will block the http roundtrip.

## install 
go get github.com/ryandeng/accesslog

## features
* the format of each line is `nanotimestamp, method, URI, request header, request body (only for application/json), status, response body, response body (only for application/json), response length, elapsed time`
* least memory allocation
* automatically rotate log (the threshold is 1800MB)
* write log contents asynchronously

## example

```go
func main() {
	conf := &accesslog.Conf{
		Filename:     "./access/example.log", // the access log dir+name, dir will be generated if it doesn't exist
		RequestBody:  true, // whether to record request body (only for application/json)
		ResponseBody: true, // whether to record response body (only for application/json)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/abc", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-type", "application/json")
		resp := map[string]string{
			"user":     "Peter",
			"position": "manager",
		}
		json.NewEncoder(w).Encode(resp)
	})

	http.ListenAndServe(":8080", accesslog.Handler(conf, mux))
}
```

If you are using some gracedown http framework, you can call `accesslog.Flush()` to flush the contents to the disk in your gracedown procedure.
