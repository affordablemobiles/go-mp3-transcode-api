package main

import (
	"io"
	"net/http"
	"os"
)

func websrvTranscodeForm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	file, err := os.Open("demo.html")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	io.Copy(w, file)
}

func websrvTranscode(w http.ResponseWriter, r *http.Request) {
	// Parse our multipart form, 10 << 20 specifies a maximum
	// upload of 10 MB files.
	r.ParseMultipartForm(100 << 20)

	// FormFile returns the first file for the given key `myFile`
	// it also returns the FileHeader so we can get the Filename,
	// the Header and the size of the file
	file, _, err := r.FormFile("source")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	websrvTranscodeResponse(w, r, file)
}

func websrvTranscodeResponse(w http.ResponseWriter, r *http.Request, file io.ReadCloser) {
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)

	transcode_audio(w, file)
}
