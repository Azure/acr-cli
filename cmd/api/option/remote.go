package option

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	// "github.com/docker/docker-credential-helpers/credentials"
	"github.com/Azure/acr-cli/cmd/api/credential"
	"github.com/Azure/acr-cli/cmd/api/crypto"
	onet "github.com/Azure/acr-cli/cmd/api/net"
	"github.com/Azure/acr-cli/cmd/api/trace"
	"github.com/Azure/acr-cli/cmd/api/version"
	"github.com/sirupsen/logrus"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// Remote options struct contains flags and arguments specifying one registry.
// Remote implements oerrors.Handler and interface.
type Remote struct {
	DistributionSpec
	CACertFilePath string
	CertFilePath   string
	KeyFilePath    string
	Insecure       bool
	Configs        []string
	Username       string
	Secret         string

	resolveFlag []string
	headers     http.Header
	warned      map[string]*sync.Map
	plainHTTP   func() (plainHTTP bool, enforced bool)
	store       credentials.Store
}

// authClient assembles a oras auth client.
func (opts *Remote) authClient(debug bool) (client *auth.Client, err error) {
	config, err := opts.tlsConfig()
	if err != nil {
		return nil, err
	}
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = config
	dialContext, err := opts.parseResolve(baseTransport.DialContext)
	if err != nil {
		return nil, err
	}
	baseTransport.DialContext = dialContext
	client = &auth.Client{
		Client: &http.Client{
			// http.RoundTripper with a retry using the DefaultPolicy
			// see: https://pkg.go.dev/oras.land/oras-go/v2/registry/remote/retry#Policy
			Transport: retry.NewTransport(baseTransport),
		},
		Cache:  auth.NewCache(),
		Header: opts.headers,
	}
	client.SetUserAgent("oras/" + version.GetVersion())
	if debug {
		client.Client.Transport = trace.NewTransport(client.Client.Transport)
	}

	cred := opts.Credential()
	if cred != auth.EmptyCredential {
		client.Credential = func(ctx context.Context, s string) (auth.Credential, error) {
			return cred, nil
		}
	} else {
		var err error
		opts.store, err = credential.NewStore(opts.Configs...)
		if err != nil {
			return nil, err
		}
		client.Credential = credentials.Credential(opts.store)
	}
	return
}

// parseResolve parses resolve flag.
func (opts *Remote) parseResolve(baseDial onet.DialFunc) (onet.DialFunc, error) {
	if len(opts.resolveFlag) == 0 {
		return baseDial, nil
	}

	formatError := func(param, message string) error {
		return fmt.Errorf("failed to parse resolve flag %q: %s", param, message)
	}
	var dialer onet.Dialer
	for _, r := range opts.resolveFlag {
		parts := strings.SplitN(r, ":", 4)
		length := len(parts)
		if length < 3 {
			return nil, formatError(r, "expecting host:port:address[:address_port]")
		}
		host := parts[0]
		hostPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, formatError(r, "expecting uint64 host port")
		}
		// ipv6 zone is not parsed
		address := net.ParseIP(parts[2])
		if address == nil {
			return nil, formatError(r, "invalid IP address")
		}
		addressPort := hostPort
		if length > 3 {
			addressPort, err = strconv.Atoi(parts[3])
			if err != nil {
				return nil, formatError(r, "expecting uint64 address port")
			}
		}
		dialer.Add(host, hostPort, address, addressPort)
	}
	dialer.BaseDialContext = baseDial
	return dialer.DialContext, nil
}

// tlsConfig assembles the tls config.
func (opts *Remote) tlsConfig() (*tls.Config, error) {
	config := &tls.Config{
		InsecureSkipVerify: opts.Insecure,
	}
	if opts.CACertFilePath != "" {
		var err error
		config.RootCAs, err = crypto.LoadCertPool(opts.CACertFilePath)
		if err != nil {
			return nil, err
		}
	}
	if opts.CertFilePath != "" && opts.KeyFilePath != "" {
		cert, err := tls.LoadX509KeyPair(opts.CertFilePath, opts.KeyFilePath)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{cert}
	}
	return config, nil
}

// isPlainHttp returns the plain http flag for a given registry.
func (opts *Remote) isPlainHttp(registry string) bool {
	plainHTTP, enforced := opts.plainHTTP()
	if enforced {
		return plainHTTP
	}
	host, _, _ := net.SplitHostPort(registry)
	if host == "localhost" || registry == "localhost" {
		// not specified, defaults to plain http for localhost
		return true
	}
	return plainHTTP
}

func (opts *Remote) handleWarning(registry string, logger logrus.FieldLogger) func(warning remote.Warning) {
	if opts.warned == nil {
		opts.warned = make(map[string]*sync.Map)
	}
	warned := opts.warned[registry]
	if warned == nil {
		warned = &sync.Map{}
		opts.warned[registry] = warned
	}
	logger = logger.WithField("registry", registry)
	return func(warning remote.Warning) {
		if _, loaded := warned.LoadOrStore(warning.WarningValue, struct{}{}); !loaded {
			logger.Warn(warning.Text)
		}
	}
}

// Credential returns a credential based on the remote options.
func (opts *Remote) Credential() auth.Credential {
	return credential.Credential(opts.Username, opts.Secret)
}
