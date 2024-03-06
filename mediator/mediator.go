// Package mediator impolements a mediation between the client and server that results in either serving
// a file or passing the request on to a backend.
package mediator

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DemmyDemon/tjenare/config"
	"golang.org/x/net/publicsuffix"
)

// Mediator is responsible for mediating a request, either dispatching it to a backend or handling it as a file request.
type Mediator struct {
	// ServerConfig is the configuration this mediator is expected to respect.
	ServerConfig *config.ServerConfig
}

// Begin sets up and a Mediator and starts a http.Server listening on the given configuration's TLSPort
func Begin(cfg *config.ServerConfig) {
	mediator := Mediator{ServerConfig: cfg}
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.TLSPort),
		Handler: mediator,
		TLSConfig: &tls.Config{
			GetCertificate:     mediator.getCertificate,
			GetConfigForClient: mediator.getConfigForClient,
		},
	}
	log.Printf("Listening with TLS on :%d\n", cfg.TLSPort)
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

// ServeHTTP selects the correct backend to hand off to, or serves a file request if there is no matching backend.
func (med Mediator) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	domain, subdomain, err := med.parseRequest(r)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("An error was encountered while processing your request."))
		log.Printf("Error parsing hostname %q: %s", r.Host, err)
		return
	}

	domconfig, ok := med.ServerConfig.Domains[domain]
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("An error was encountered while processing your request."))
		log.Printf("Error: No such domain in config: %q", r.Host)
		return
	}

	if subdomain == "" {
		subdomain = domconfig.Default
	}

	if med.handoff(domconfig, domain, subdomain, w, r) {
		return
	}

	retry := med.serveFile(domconfig, subdomain, w, r)
	if retry {
		med.serveFile(domconfig, subdomain, w, r)
	}
}

func (med Mediator) parseRequest(r *http.Request) (string, string, error) {
	host := strings.Split(r.Host, ":")[0]
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return "", "", fmt.Errorf("could not parse request into pieces: %w", err)
	}
	subdomain := strings.TrimSuffix(host, domain)
	subdomain = strings.TrimSuffix(subdomain, ".")
	return domain, subdomain, nil
}

func (med Mediator) createReverseProxy(backend *config.BackendConfig) (*httputil.ReverseProxy, error) {

	if backend.URL == nil {
		url, err := url.Parse(backend.Target)
		if err != nil {
			return nil, fmt.Errorf("parsing backend target: %w", err)
		}
		backend.URL = url
	}

	return &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetXForwarded()
				r.SetURL(backend.URL)
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				if errors.Is(err, context.Canceled) {
					return // This is not actually an error, though.
				}
				log.Printf("[%s] %s error: %s\n", r.RemoteAddr, backend.Target, err)
				w.WriteHeader(http.StatusBadGateway)
				w.Write([]byte(`SOMETHING ERROR HAPPEN`))
			},
		},
		nil
}

func (med Mediator) getConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	if hello.ServerName == "" {
		return nil, errors.New("TLS ClientHello has no ServerName")
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(hello.ServerName)
	if err != nil {
		log.Printf("Error parsing TLS ClientHello ServerName: %s -> %s", hello.ServerName, err)
	}
	// log.Printf("GetConfigForClient: ServerName=%s, domain=%s\n", hello.ServerName, domain)
	domconf, ok := med.ServerConfig.Domains[domain]
	if !ok {
		log.Printf("Error looking up configuration for %s", domain)
		return nil, errors.New("configuration contains no such domain")
	}

	info, err := os.Stat(domconf.CertFile)
	if err != nil {
		return nil, fmt.Errorf("stat certfile for %s: %w", domain, err)
	}
	// If it's unset, it'll be in 1970. Hopefully your cert file isn't that old.
	if domconf.CertTime.Before(info.ModTime()) {
		domconf.CertConfig = nil
		domconf.CertTime = info.ModTime()
	}

	if domconf.CertConfig == nil {
		cert, err := tls.LoadX509KeyPair(domconf.CertFile, domconf.KeyFile)
		if err != nil {
			log.Printf("Error during certificate load (%q, %q): %s", domconf.CertFile, domconf.KeyFile, err)
			return nil, fmt.Errorf("loading key pair: %w", err)
		}
		log.Printf("Loaded certificate (%q,%q)\n", domconf.CertFile, domconf.KeyFile)
		config := &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2", "http/1.1"}, // Enable HTTP/2
		}
		domconf.CertConfig = config
	}

	return domconf.CertConfig, nil
}

func (med Mediator) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return nil, nil // Forcing GetConfigForClient to run for each request
}

func (med Mediator) handoff(domconfig *config.DomainConfig, domain, subdomain string, w http.ResponseWriter, r *http.Request) bool {
	backend, ok := domconfig.Backends[subdomain]
	if !ok {
		return false // As in "handoff refused: Request not handled"
	}
	if backend.ReverseProxy == nil {
		proxy, err := med.createReverseProxy(backend)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("An error was encountered while processing your request."))
			log.Printf("failed to create reverse proxy for %q: %s\n", backend.Target, err)
			return true
		}
		backend.ReverseProxy = proxy
	}
	log.Printf("[%s] %s.%s%s -> %s\n", r.RemoteAddr, subdomain, domain, r.URL.Path, backend.Target)
	backend.ReverseProxy.ServeHTTP(w, r)
	return true
}

func (med Mediator) serveFile(domconfig *config.DomainConfig, subdomain string, w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.Path
	if strings.HasSuffix(path, "/") {
		path += "index.html"
	}
	path = filepath.Join(domconfig.BasePath, subdomain, domconfig.Subdir, path)
	path = filepath.Clean(path)

	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if domconfig.IndexFallback {
				if strings.HasSuffix(r.URL.Path, "/index.html") {
					log.Printf("[%s] circular index fallback for %s -> %s\n", r.RemoteAddr, path, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte("The requested file does not exist"))
					return false // Don't repeat the retry
				}
				r.URL.Path = filepath.Dir(r.URL.Path) + "/index.html"
				log.Printf("[%s] index fallback for %s -> %s\n", r.RemoteAddr, path, r.URL.Path)
				return true
			} else {
				log.Printf("[%s] Does not exist: %s\n", r.RemoteAddr, path)
			}
		} else {
			log.Printf("[%s] Could not stat %s: %s\n", r.RemoteAddr, path, err)
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("The requested file does not exist"))
		return false
	}

	if fileInfo.IsDir() {
		path += "/index.html"
		fileInfo, err = os.Stat(path)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("The requested file does not exist"))
			if os.IsNotExist(err) {
				log.Printf("[%s] Does not exist: %s\n", r.RemoteAddr, path)
			} else {
				log.Printf("[%s] Could not stat %s: %s\n", r.RemoteAddr, path, err)
			}
			return false
		}
	}

	file, err := os.Open(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("The requested file could not be opened"))
		log.Printf("[%s] Could not open %s: %s\n", r.RemoteAddr, path, err)
		return false
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Printf("Failed to close %q: %s\n", path, err)
		}
	}()

	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.Header().Set("Last-Modified", fileInfo.ModTime().Format(http.TimeFormat))

	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	n, err := io.Copy(w, file)
	if err != nil {
		log.Printf("[%s] Error encountered sending %s: %s\n", r.RemoteAddr, path, err)
		return false
	}
	log.Printf("[%s] [%d] %s\n", r.RemoteAddr, n, path)
	return false
}
