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
	TLSPort      int                      `json:"tlsport"`
	InsecurePort int                      `json:"insecureport"`
	LogFile      string                   `json:"logfile"`
	Domains      map[string]*DomainConfig `json:"domains"`
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
	Target       string
	URL          *url.URL
	ReverseProxy *httputil.ReverseProxy
}

func (bc *BackendConfig) UnmarshalJSON(b []byte) error {
	json.Unmarshal(b, &bc.Target)
	return nil
}

func (bc *BackendConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal([]byte(bc.Target))
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
