package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer(t *testing.T) {
	ts := httptest.NewServer(newMux())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %v %v", resp, err)
	}
	_ = resp.Body.Close()
	resp, err = http.Post(ts.URL+"/v1/handle", "application/json", strings.NewReader(`{"version":"v1","type":"ping"}`))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("handle: %v %v", resp, err)
	}
	_ = resp.Body.Close()
}
