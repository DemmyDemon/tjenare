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

type ServerConfig struct {
	TLSPort       int                      `json:"tlsport"`
	InsecurePort  int                      `json:"insecureport"`
	DefaultDomain string                   `json:"defaultdomain"`
	LogFile       string                   `json:"logfile"`
	Domains       map[string]*DomainConfig `json:"domains"`
}

type DomainConfig struct {
	BasePath   string                    `json:"basepath"`
	Default    string                    `json:"default"`
	CertFile   string                    `json:"certfile"`
	CertTime   time.Time                 `json:"-"`
	CertConfig *tls.Config               `json:"-"`
	KeyFile    string                    `json:"keyfile"`
	Backends   map[string]*BackendConfig `json:"backends"`
}

type BackendConfig struct {
	Target       string                 `json:"target"`
	URL          *url.URL               `json:"-"`
	ReverseProxy *httputil.ReverseProxy `json:"-"`
}

func Load(path string) (*ServerConfig, error) {
	var server ServerConfig
	rawJson, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("opening config file: %w", err)
	}

	err = json.Unmarshal(rawJson, &server)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling config file: %w", err)
	}

	return &server, err
}
