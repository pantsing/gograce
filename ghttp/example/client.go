package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tr := &http.Transport{
		DisableCompression:  true,
		MaxIdleConnsPerHost: 100,
		DisableKeepAlives:   false,
	}

	client := &http.Client{Transport: tr}

	var wg sync.WaitGroup
	var m, n int64
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			for j := 0; j < 10000; j++ {
				atomic.AddInt64(&n, 1)
				resp, err := client.Get("http://127.0.0.1:6086/")
				if err != nil {
					fmt.Println(err)
					if resp != nil {
						resp.Body.Close()
					}
					fmt.Println(m, "/", n)
				} else {
					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						fmt.Println(err)
					} else {
						atomic.AddInt64(&m, 1)
					}
					fmt.Println(string(body))
					resp.Body.Close()
				}
				// time.Sleep(10 * time.Millisecond)

			}
			wg.Done()
		}()
	}
	wg.Wait()
	fmt.Println(m, "/", n)
}
