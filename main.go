package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DemmyDemon/tjenare/config"
	"github.com/DemmyDemon/tjenare/mediator"
	"github.com/DemmyDemon/tjenare/redirect"
)

func main() {
	log.SetFlags(log.Ltime | log.Ldate)
	if os.Getenv("DEVMODE") != "" {
		log.Println("Filename logging enabled: 'Developer mode'")
		log.SetFlags(log.Lshortfile | log.Ltime | log.Ldate)
	}

	var cfg *config.ServerConfig
	var err error
	switch {
	case len(os.Args) > 1:
		log.Printf("Loading configuration from command line argument: %s", os.Args[1])
		cfg, err = config.Load(os.Args[1])
	case os.Getenv("CONFIG") != "":
		log.Printf("Loading configuration from CONFIG environment variable: %s", os.Getenv("CONFIG"))
		cfg, err = config.Load(os.Getenv("CONFIG"))
	default:
		log.Printf("Loading hardcoded default configuration from /etc/tjenare.json")
		cfg, err = config.Load("/etc/tjenare.json")
	}
	if err != nil {
		log.Fatal(err)
	}

	if cfg.LogFile != "" {
		logfile, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			log.Fatalf("Failed to open log file %q: %s\n", cfg.LogFile, err)
		}
		log.SetOutput(logfile)
		log.Println("Log file opened")

		defer func() {
			// This is unlikely to ever happen, as the OS has likely slain the process if main is returning.
			log.Println("Log file closing")
			log.SetOutput(os.Stderr)
			err := logfile.Close()
			if err != nil {
				log.Fatalf("Error closing log file: %s", err)
			}
		}()
	}

	go redirect.ServeSSLRedirect(cfg)
	go mediator.Begin(cfg)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c // Block here until the interrupt comes in
	log.Println("Received interrupt")
}
