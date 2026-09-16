// A stdlib-only HTTP probe executed inside the network-isolated smoke container.
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	var input struct {
		Path, Method, Body string
		Headers            map[string]string
	}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		panic(err)
	}
	req, err := http.NewRequest(input.Method, "http://127.0.0.1:8090"+input.Path, strings.NewReader(input.Body))
	if err != nil {
		panic(err)
	}
	for key, value := range input.Headers {
		if key == "Host" {
			req.Host = value
		} else {
			req.Header.Set(key, value)
		}
	}
	client := &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	headers := map[string]string{}
	for key := range res.Header {
		headers[key] = res.Header.Get(key)
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Status  int
		Headers map[string]string
		Body    []byte
	}{res.StatusCode, headers, body}); err != nil {
		panic(err)
	}
}
