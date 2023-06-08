package main

import (
	"context"
	"log"
	"net/http"
	"os"

	glog "github.com/a1comms/go-gaelog/v2"
	viap "github.com/a1comms/go-middleware-validate-iap"

	"github.com/gorilla/mux"
	"github.com/urfave/negroni"

	"cloud.google.com/go/errorreporting"
	"contrib.go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"
)

func main() {
	r := mux.NewRouter()

	// If it isn't found in the main router,
	// search in the endpoint verification router.
	r.NotFoundHandler = r.NewRoute().BuildOnly().HandlerFunc(defaultHandler).GetHandler()

	// Validate IAP
	var globalMiddleware negroni.HandlerFunc = viap.ValidateIAPAppEngineMiddleware

	r.Path("/api/audio/v1/transcode").Methods("GET").HandlerFunc(websrvTranscodeForm)
	r.Path("/api/audio/v1/transcode").Methods("POST").HandlerFunc(websrvTranscode)
	r.Path("/api/audio/v1/gcs_transcode").Methods("POST").HandlerFunc(websrvTranscodeGCS)

	//---
	// Error reporting...
	//---
	errorClient, err := errorreporting.NewClient(context.Background(), gae_project(), errorreporting.Config{
		ServiceName:    gae_service(),
		ServiceVersion: gae_version(),
		OnError: func(err error) {
			log.Printf("Could not log error: %v", err)
		},
	})
	if err != nil {
		log.Fatalf("%s", err)
	}
	defer errorClient.Close()

	recover := negroni.NewRecovery()
	recover.PrintStack = true
	recover.LogStack = false
	recover.PanicHandlerFunc = func(info *negroni.PanicInformation) {
		user, _ := viap.GetUserEmail(info.Request)

		errorClient.Report(errorreporting.Entry{
			Req:   info.Request,
			User:  user,
			Error: info.RecoveredPanic.(error),
			Stack: info.Stack,
		})
	}

	// Handle all HTTP requests with our router.
	http.Handle("/", negroni.New(
		//recover,
		negroni.Wrap(&ochttp.Handler{
			Propagation: &propagation.HTTPFormat{},
			Handler: negroni.New(
				negroni.HandlerFunc(glog.Middleware),
				negroni.HandlerFunc(globalMiddleware),
				negroni.Wrap(r),
			),
		}),
	))

	// [START setting_port]
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
	// [END setting_port]
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "File Not Found", 404)
}

func mustGetenv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Panicf("%s environment variable not set.", k)
	}
	return v
}

func gae_project() string {
	return os.Getenv("GOOGLE_CLOUD_PROJECT")
}

func gae_service() string {
	return os.Getenv("GAE_SERVICE")
}

func gae_version() string {
	return os.Getenv("GAE_VERSION")
}
