package main

import (
	"net/http"

	"cloud.google.com/go/storage"
)

func websrvTranscodeGCS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	bucket, file := r.FormValue("bucket"), r.FormValue("file")
	if bucket == "" || file == "" {
		http.Error(w, "Invalid Parameters", http.StatusBadRequest)
		return
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		panic(err)
	}

	bkt := client.Bucket(bucket)
	obj := bkt.Object(file)

	reader, err := obj.NewReader(ctx)
	if err != nil {
		panic(err)
	}
	defer reader.Close()

	websrvTranscodeResponse(w, r, reader)
}
