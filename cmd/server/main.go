// Command server 是 ibukiRPG 的 HTTP 调试服务骨架。
package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok "+buildinfo.String()+"\n")
	})
	// 与移动端相同的 JSON DTO 入口，便于桌面调试。
	mux.HandleFunc("POST /v1/handle", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, mobile.Handle(r.Context(), string(body)))
	})
	return mux
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "监听地址")
	flag.Parse()
	srv := &http.Server{Addr: *addr, Handler: newMux(), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("%s server listening on %s", buildinfo.String(), *addr)
	log.Fatal(srv.ListenAndServe())
}
