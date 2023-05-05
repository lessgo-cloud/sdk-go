package client

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/gorilla/mux"
)

// get port from args
var _port = flag.Int("port", 0, "Listen on all interfaces at the given port [$PORT]")

func init() {
	flag.Usage = func() {
		flag.PrintDefaults()
	}
	flag.Parse()
}

// init logger
var errLogger, infoLogger *log.Logger

func init() {
	errLogger = log.New(os.Stderr, "lessgo function: ", log.LstdFlags)
	infoLogger = log.New(os.Stdout, "lessgo function: ", log.LstdFlags)
}

// Use a gorilla mux for handling all HTTP requests
var router *mux.Router

var defaultHandler = func(res http.ResponseWriter, req *http.Request) {
	fmt.Fprint(res, "ok")
}

func init() {
	router = mux.NewRouter()

	router.NotFoundHandler = http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusNotFound)
		fmt.Fprint(res, "lessgo: 404 not found")
	})

	router.HandleFunc("/health/{endpoint:readiness|liveness}", defaultHandler)
}

func AddHttpHandler(path string, handler func(http.ResponseWriter, *http.Request)) {
	router.HandleFunc(path, handler)
}

// from knative main.go
// run a cloudevents client in receive mode which invokes
// the user-defined function.Handler on receipt of an event.
func ReceiveAndHandle(handler any) error {
	if handler == nil {
		handler = defaultHandler
		errLogger.Print("ReceiveAndHandle handler is nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()

	port := getFunctionPort()

	httpHandler := toHttpHandler(handler, ctx)

	if httpHandler == nil {
		infoLogger.Print("Initializing CloudEvent function")

		protocol, err := cloudevents.NewHTTP(
			cloudevents.WithPort(port),
			cloudevents.WithPath("/"),
		)
		if err != nil {
			return err
		}
		eventHandler, err := cloudevents.NewHTTPReceiveHandler(ctx, protocol, handler)
		router.Handle("/", eventHandler)
	} else {
		infoLogger.Print("Initializing HTTP function")

		router.Handle("/", httpHandler)
	}

	httpServer := &http.Server{
		Addr:           fmt.Sprintf(":%d", port),
		Handler:        router,
		ReadTimeout:    1 * time.Minute,
		WriteTimeout:   1 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}

	listenAndServeErr := make(chan error, 1)
	go func() {
		infoLogger.Printf("listening on http port %d", port)

		err := httpServer.ListenAndServe()
		cancel()
		listenAndServeErr <- err
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancelFn := context.WithTimeout(context.Background(), time.Second*5)
	defer shutdownCancelFn()
	err := httpServer.Shutdown(shutdownCtx)
	if err != nil {
		errLogger.Printf("error on server shutdown: %s", err)
	}

	if err := <-listenAndServeErr; http.ErrServerClosed == err {
		return nil
	}
	return err
}

// if the handler signature is compatible with http handler the function returns an instance of `http.Handler`,
// otherwise nil is returned
func toHttpHandler(handler interface{}, ctx context.Context) http.Handler {
	if handler == nil {
		return nil
	}

	if f, ok := handler.(func(rw http.ResponseWriter, req *http.Request)); ok {
		return recoverMiddleware(http.HandlerFunc(f))
	}

	if f, ok := handler.(func(ctx context.Context, rw http.ResponseWriter, req *http.Request)); ok {
		ff := func(rw http.ResponseWriter, req *http.Request) {
			f(ctx, rw, req)
		}
		return recoverMiddleware(http.HandlerFunc(ff))
	}

	return nil
}

func recoverMiddleware(handler http.Handler) http.Handler {
	f := func(rw http.ResponseWriter, req *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				recoverError := fmt.Errorf("user function error: %v", r)
				stack := string(debug.Stack())
				errLogger.Printf("%v\n%v\n", recoverError, stack)

				rw.WriteHeader(http.StatusInternalServerError)
			}
		}()
		handler.ServeHTTP(rw, req)
		return
	}
	return http.HandlerFunc(f)
}

func getFunctionPort() int {
	if *_port != 0 {
		return *_port
	}
	port, _ := strconv.Atoi(os.Getenv("port"))
	return port
}
