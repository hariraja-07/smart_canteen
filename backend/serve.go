package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// runServe starts the HTTP server. It is the default command, so running the
// binary with no arguments behaves as before.
func runServe() int {
	db, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	secret, err := jwtSecret()
	if err != nil {
		log.Fatal(err)
	}

	port, err := listenPort()
	if err != nil {
		log.Fatal(err)
	}
	addr := ":" + port
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, newRouter(db, secret)))
	return 0
}

// listenPort returns the TCP port to bind, from PORT when the host sets it.
//
// Hosted platforms (Render, Fly, Cloud Run) assign the port through PORT and
// route traffic to whatever it says. Binding a fixed 8080 there means the
// service is up and healthy while every request goes somewhere else, so it gets
// killed and restarted in a loop. Defaulting to 8080 keeps `go run .` and
// Docker working unchanged.
//
// An unusable PORT is reported rather than ignored: silently falling back to
// 8080 would hide the misconfiguration and reintroduce the exact bug this
// function exists to prevent.
func listenPort() (string, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		return "8080", nil
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return "", fmt.Errorf("PORT=%q is not a valid port number", port)
	}
	return port, nil
}
