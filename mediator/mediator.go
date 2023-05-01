package mediator

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
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

type Mediator struct {
	ServerConfig *config.ServerConfig
}

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

	med.serveFile(domconfig, domain, subdomain, w, r)
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
			// TODO: Do we need a custom ErrorHandler here?
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
			log.Printf("failed to create reverse proxy for %q: %s", backend.Target, err)
			return true
		}
		backend.ReverseProxy = proxy
	}
	log.Printf("[%s] %s.%s%s -> %s", r.RemoteAddr, subdomain, domain, r.URL.Path, backend.Target)
	backend.ReverseProxy.ServeHTTP(w, r)
	return true
}

func (med Mediator) serveFile(domconfig *config.DomainConfig, domain, subdomain string, w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/") {
		path += "index.html"
	}
	path = filepath.Clean(domconfig.BasePath + subdomain + domconfig.Subdir + path)

	fileInfo, err := os.Stat(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("The requested file does not exist"))
		if os.IsNotExist(err) {
			log.Printf("[%s] Does not exist: %s", r.RemoteAddr, path)
		} else {
			log.Printf("[%s] Could not stat %s: %s", r.RemoteAddr, path, err)
		}
		return
	}

	file, err := os.Open(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("The requested file could not be opened"))
		log.Printf("[%s] Could not open %s: %s", r.RemoteAddr, path, err)
		return
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Printf("Failed to close %q: %s", path, err)
		}
	}()

	w.WriteHeader(http.StatusOK)
	w.Header().Add("content-length", strconv.FormatInt(fileInfo.Size(), 10))
	w.Header().Add("last-modified", fileInfo.ModTime().Format(http.TimeFormat))
	n, err := io.Copy(w, file)
	if err != nil {
		log.Printf("[%s] Error encountered sending %s: %s", r.RemoteAddr, path, err)
		return
	}
	log.Printf("[%s] [%d] %s", r.RemoteAddr, n, path)
}
