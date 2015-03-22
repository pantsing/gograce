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

	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		fmt.Fprintf(w, "Welcome to the home page!"+strconv.Itoa(os.Getpid()))
	})

	err := ghttp.ListenAndServe(":6086", nil)
	if err != nil {
		log.Println(err)
	}
}
