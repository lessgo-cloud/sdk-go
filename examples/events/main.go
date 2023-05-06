package main

import (
	"fmt"
	"log"
	"net/http"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	function "github.com/lessgo-cloud/sdk-go"
)

// See more: https://github.com/cloudevents/sdk-go
func handler(res http.ResponseWriter, req *http.Request) {
	event, err := cloudevents.NewEventFromHTTPRequest(req)
	if err != nil {
		log.Printf("failed to parse CloudEvent from request: %v", err)
		http.Error(res, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	}
	res.Write([]byte(event.String()))
	fmt.Printf("received event: %s\n", event.String())
}

func main() {
	if err := function.ReceiveAndHandle(handler); err != nil {
		log.Fatal(err)
	}
}
