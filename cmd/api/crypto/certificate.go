package crypto

import (
	"crypto/x509"
	"errors"
	"os"
)

// LoadCertPool returns a new cert pool loaded from the cert file.
func LoadCertPool(path string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
		return nil, errors.New("Failed to load certificate in file: " + path)
	}
	return pool, nil
}
