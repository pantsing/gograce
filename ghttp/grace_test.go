package ghttp_test

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/pantsing/gograce/ghttp"
)

func TestListenAndServe(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Welcome to the home page!"+strconv.Itoa(os.Getpid()))
	})

	var gs ghttp.GraceServer
	gs.ListenerCloseTimeout = 60 * time.Second

	err := gs.ListenAndServe(":6086", mux)
	if err != nil {
		log.Println(err)
	}
}

func TestServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Welcome to the home page!"+strconv.Itoa(os.Getpid()))
	})

	var gs ghttp.GraceServer
	gs.ListenerCloseTimeout = 60 * time.Second

	gl, err := ghttp.GetListener(":6087")
	if err != nil {
		log.Println(err)
		return
	}

	err = gs.Serve(gl, mux)
	if err != nil {
		log.Println(err)
	}
}
