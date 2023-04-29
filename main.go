package main

import (
	"log"

	"github.com/DemmyDemon/tjenare/config"
	"github.com/DemmyDemon/tjenare/mediator"
	"github.com/DemmyDemon/tjenare/redirect"
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	cfg, err := config.Load("testing/testing.json")
	if err != nil {
		log.Fatal(err)
	}
	go redirect.ServeSSLRedirect(cfg)
	mediator.Begin(cfg)
}
