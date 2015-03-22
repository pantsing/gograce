package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/pantsing/gograce/ghttp"
)

func main() {
	ppid := os.Getppid()
	const msg = "Serving with pid %d ppid %d"
	log.Printf(msg, os.Getpid(), ppid)

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

	http.HandleFunc("/srvctrl", ghttp.SrvCtrlhandler)

	err = gs.Serve(gl, nil)
	if err != nil {
		log.Println(err)
	}
}
