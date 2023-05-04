// Package config implements the loading of a JSON-formatted configuration file for tjenare
package config

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

// ServerConfig is the configuration used by a Mediator, Redirector and other bits and bobs of the server.
type ServerConfig struct {
	// TLSPort is the TCP port number where TLS requests will be served.
	TLSPort int `json:"tlsport"`
	// InsecurePort is the TCP port number where non-TLS requests will be served.
	InsecurePort int `json:"insecureport"`
	// LogFile is a path to the file logging will be redirected to after the configuration file is loaded.
	LogFile string `json:"logfile"`
	// Domains is the map of domains that will be served, and the configuration for each.
	Domains map[string]*DomainConfig `json:"domains"`
}

// DomainConfig describes the configuration for a single domain and it's subdomains.
type DomainConfig struct {
	// BasePath is the place in the directory tree below which all the subdomains of this domain
	BasePath string `json:"basepath"`
	// Default is the subdomain used if no subdomain is specifies.
	// For example, a value of www here means requests to https://example.com will be treated as if they were to https://www.example.com
	Default string `json:"default"`
	// Subdir represents the fragment of the path between the subdomain and the actual files, such as public_html
	Subdir string `json:"subdir"`
	// CertFile is the path to the full chain certificate file to use.
	CertFile string `json:"certfile"`
	// CertTime stores the modification time of the CertFile to compare to, in order to know if it changed while the server is running.
	CertTime time.Time `json:"-"`
	// CertConfig is the final full certificate configuration used, cached here to aviod having to re-read the cert for every request.
	CertConfig *tls.Config `json:"-"`
	// KeyFile is the path to the private key associated with the given certificate file.
	KeyFile string `json:"keyfile"`
	// Backends holds each backend for this domain and it's associated configuration
	Backends map[string]*BackendConfig `json:"backends"`
}

// BackendConfig holds the configuration for a single backend configuration
type BackendConfig struct {
	// Target is the string specified in the configuration file
	Target string
	// URL holds the Target, parsed into a *url.URL object when the file is loaded.
	URL *url.URL
	// ReverseProxy holds the cached httputil.ReverseProxy object once created.
	ReverseProxy *httputil.ReverseProxy
}

// UnmarshalJSON unmarshalls the single string in the configuration file into the full object
func (bc *BackendConfig) UnmarshalJSON(b []byte) error {
	json.Unmarshal(b, &bc.Target)
	return nil
}

// MarshalJSON marshells just the Target part of the BackendConfig into a
func (bc *BackendConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal([]byte(bc.Target))
}

func Load(path string) (*ServerConfig, error) {
	var server ServerConfig
	rawJSON, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening config file: %w", err)
	}

	err = json.Unmarshal(rawJSON, &server)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling config file: %w", err)
	}

	return &server, err
}
