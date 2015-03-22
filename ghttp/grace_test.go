package ghttp_test

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/pantsing/gograce/ghttp"
)

func TestService(t *testing.T) {
	var gs ghttp.GraceServer
	gs.ListenerCloseTimeout = 60

	gl, err := ghttp.GetListener(":6086")
	if err != nil {
		log.Println(err)
		return
	}

	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Welcome to the home page!"+strconv.Itoa(os.Getpid()))
	})

	err = gs.Serve(gl, nil)
	if err != nil {
		log.Println(err)
	}
}
