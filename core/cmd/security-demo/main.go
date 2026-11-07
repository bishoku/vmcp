package main

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/securitydemo"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	s, err := securitydemo.New(context.Background(), os.Getenv("DEMO_ISSUER"), os.Getenv("DEMO_DISCOVERY_URL"), os.Getenv("DEMO_ORIGIN"), os.Getenv("DEMO_APPROVAL_SECRET"), os.Getenv("DEMO_GATEWAY"))
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: ":8443", Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	log.Fatal(server.ListenAndServeTLS(os.Getenv("DEMO_TLS_CERT"), os.Getenv("DEMO_TLS_KEY")))
}
