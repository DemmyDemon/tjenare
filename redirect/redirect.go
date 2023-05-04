// Package redirect implements a minimalist/simplistic way of redirecting all requests on one port to another.
// The inteded usage is to permanently redirect all :80 HTTP requests to :443 HTTPS requests, though the
// ports can be changed by configuration.
package redirect

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/DemmyDemon/tjenare/config"
)

// Redirect takes care of the redirection part of tjenare
type Redirect struct {
	// TargetPort is the incoming TCP port the redirect will be served on.
	TargetPort int
}

// ServeHTTP does the actual HTTP response for the Redirect
func (red Redirect) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target := *r.URL
	host := strings.Split(r.Host, ":")[0]

	target.Scheme = "https"
	if red.TargetPort == 443 {
		target.Host = host
	} else {
		target.Host = fmt.Sprintf("%s:%d", host, red.TargetPort)
	}

	log.Printf("[%s] Redirect %s%s -> %s\n", r.RemoteAddr, r.Host, r.URL.String(), target.String())

	http.Redirect(w, r, target.String(), http.StatusMovedPermanently)

}

// ServeSSLRedirect sets up a new Redirect to redirect all traffic from the InsecurePort to the TLSPort.
// Blocks until it returns an error, which is fatally logged.
func ServeSSLRedirect(cfg *config.ServerConfig) {
	handler := Redirect{
		TargetPort: cfg.TLSPort,
	}
	log.Printf("Will redirect all traffic on :%d to :%d", cfg.InsecurePort, cfg.TLSPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", cfg.InsecurePort), handler))
}
