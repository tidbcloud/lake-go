package golake

import (
	"crypto/tls"
	"errors"
	"net/http"
	"sync"
)

// Based on the original implementation in the project go-sql-driver/mysql:
// https://github.com/go-sql-driver/mysql/blob/master/utils.go

var (
	tlsConfigLock     sync.RWMutex
	tlsConfigRegistry map[string]*tls.Config
)

// RegisterTLSConfig registers a custom tls.Config to be used with sql.Open.
func RegisterTLSConfig(key string, config *tls.Config) error {
	if key == "" || config == nil {
		return errors.New("TLS configuration requires a name and a non-nil config")
	}
	tlsConfigLock.Lock()
	defer tlsConfigLock.Unlock()
	if tlsConfigRegistry == nil {
		tlsConfigRegistry = make(map[string]*tls.Config)
	}

	tlsConfigRegistry[key] = config.Clone()
	return nil
}

// DeregisterTLSConfig removes the tls.Config associated with key.
func DeregisterTLSConfig(key string) {
	tlsConfigLock.Lock()
	if tlsConfigRegistry != nil {
		delete(tlsConfigRegistry, key)
	}
	tlsConfigLock.Unlock()
}

type invalidTLSTransport struct {
	err error
}

func (t invalidTLSTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

func apiTransport(cfg *Config) http.RoundTripper {
	if cfg.TLSConfig == "" {
		return http.DefaultTransport
	}
	if cfg.SSLMode == SSL_MODE_DISABLE {
		return invalidTLSTransport{errors.New("a custom TLS configuration cannot be used with sslmode=disable")}
	}
	tlsConfigLock.RLock()
	registered := tlsConfigRegistry[cfg.TLSConfig]
	tlsConfigLock.RUnlock()
	if registered == nil {
		return invalidTLSTransport{errors.New("unknown Lake TLS configuration")}
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return invalidTLSTransport{errors.New("custom Lake TLS configuration requires an HTTP transport")}
	}
	transport := base.Clone()
	transport.TLSClientConfig = registered.Clone()
	return transport
}
