package main

import (
	"log"
	"net/http"

	"yolo-ave-mujica/internal/server"
)

func main() {
	srv := server.NewHTTPServer()
	log.Fatal(http.ListenAndServe(":8080", srv.Handler))
}
