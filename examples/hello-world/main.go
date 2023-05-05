package main

import (
	"io"
	"log"
	"net/http"

	function "github.com/lessgo-cloud/sdk-go"
)

func helloHandler(w http.ResponseWriter, req *http.Request) {
	io.WriteString(w, "hello, world!\n")
}

func main() {
	if err := function.ReceiveAndHandle(helloHandler); err != nil {
		log.Fatal(err)
	}
}
