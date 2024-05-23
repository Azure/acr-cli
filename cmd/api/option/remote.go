package option

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	// "github.com/docker/docker-credential-helpers/credentials"
	"github.com/Azure/acr-cli/cmd/api/credential"
	"github.com/Azure/acr-cli/cmd/api/crypto"
	oerrors "github.com/Azure/acr-cli/cmd/api/errors"
	onet "github.com/Azure/acr-cli/cmd/api/net"
	"github.com/Azure/acr-cli/cmd/api/trace"

	// "github.com/Azure/acr-cli/cmd/api/option"
	"github.com/Azure/acr-cli/cmd/api/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/errcode"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	caFileFlag                 = "ca-file"
	certFileFlag               = "cert-file"
	keyFileFlag                = "key-file"
	usernameFlag               = "username"
	passwordFlag               = "password"
	passwordFromStdinFlag      = "password-stdin"
	identityTokenFlag          = "identity-token"
	identityTokenFromStdinFlag = "identity-token-stdin"
)

// Remote options struct contains flags and arguments specifying one registry.
// Remote implements oerrors.Handler and interface.
type Remote struct {
	DistributionSpec
	CACertFilePath  string
	CertFilePath    string
	KeyFilePath     string
	Insecure        bool
	Configs         []string
	Username        string
	secretFromStdin bool
	Secret          string
	flagPrefix      string

	resolveFlag           []string
	applyDistributionSpec bool
	headerFlags           []string
	headers               http.Header
	warned                map[string]*sync.Map
	plainHTTP             func() (plainHTTP bool, enforced bool)
	store                 credentials.Store
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

// EnableDistributionSpecFlag set distribution specification flag as applicable.
func (opts *Remote) EnableDistributionSpecFlag() {
	opts.applyDistributionSpec = true
}

// ApplyFlags applies flags to a command flag set.
func (opts *Remote) ApplyFlags(fs *pflag.FlagSet) {
	opts.ApplyFlagsWithPrefix(fs, "", "")
	fs.BoolVar(&opts.secretFromStdin, passwordFromStdinFlag, false, "read password from stdin")
	fs.BoolVar(&opts.secretFromStdin, identityTokenFromStdinFlag, false, "read identity token from stdin")
}

// ApplyFlagsWithPrefix applies flags to a command flag set with a prefix string.
// Commonly used for non-unary remote targets.
func (opts *Remote) ApplyFlagsWithPrefix(fs *pflag.FlagSet, prefix, description string) {
	var (
		shortUser     string
		shortPassword string
		shortHeader   string
		notePrefix    string
	)
	if prefix == "" {
		shortUser, shortPassword = "u", "p"
		shortHeader = "H"
	}
	opts.flagPrefix, notePrefix = applyPrefix(prefix, description)

	if opts.applyDistributionSpec {
		opts.DistributionSpec.ApplyFlagsWithPrefix(fs, prefix, description)
	}
	fs.StringVarP(&opts.Username, opts.flagPrefix+usernameFlag, shortUser, "", notePrefix+"registry username")
	fs.StringVarP(&opts.Secret, opts.flagPrefix+passwordFlag, shortPassword, "", notePrefix+"registry password or identity token")
	fs.StringVar(&opts.Secret, opts.flagPrefix+identityTokenFlag, "", notePrefix+"registry identity token")
	fs.BoolVar(&opts.Insecure, opts.flagPrefix+"insecure", false, "allow connections to "+notePrefix+"SSL registry without certs")
	plainHTTPFlagName := opts.flagPrefix + "plain-http"
	plainHTTP := fs.Bool(plainHTTPFlagName, false, "allow insecure connections to "+notePrefix+"registry without SSL check")
	opts.plainHTTP = func() (bool, bool) {
		return *plainHTTP, fs.Changed(plainHTTPFlagName)
	}
	fs.StringVar(&opts.CACertFilePath, opts.flagPrefix+caFileFlag, "", "server certificate authority file for the remote "+notePrefix+"registry")
	fs.StringVarP(&opts.CertFilePath, opts.flagPrefix+certFileFlag, "", "", "client certificate file for the remote "+notePrefix+"registry")
	fs.StringVarP(&opts.KeyFilePath, opts.flagPrefix+keyFileFlag, "", "", "client private key file for the remote "+notePrefix+"registry")
	fs.StringArrayVar(&opts.resolveFlag, opts.flagPrefix+"resolve", nil, "customized DNS for "+notePrefix+"registry, formatted in `host:port:address[:address_port]`")
	fs.StringArrayVar(&opts.Configs, opts.flagPrefix+"registry-config", nil, "`path` of the authentication file for "+notePrefix+"registry")
	fs.StringArrayVarP(&opts.headerFlags, opts.flagPrefix+"header", shortHeader, nil, "add custom headers to "+notePrefix+"requests")
}

func applyPrefix(prefix, description string) (flagPrefix, notePrefix string) {
	if prefix == "" {
		return "", ""
	}
	return prefix + "-", description + " "
}

// Modify modifies error during cmd execution.
func (opts *Remote) Modify(cmd *cobra.Command, err error) (error, bool) {
	var errResp *errcode.ErrorResponse

	if errors.Is(err, auth.ErrBasicCredentialNotFound) {
		return opts.DecorateCredentialError(err), true
	}

	if errors.As(err, &errResp) {
		cmd.SetErrPrefix(oerrors.RegistryErrorPrefix)
		return &oerrors.Error{
			Err: oerrors.TrimErrResp(err, errResp),
		}, true
	}
	return err, false
}

// DecorateCredentialError decorate error with recommendation.
func (opts *Remote) DecorateCredentialError(err error) *oerrors.Error {
	configPath := " "
	if path, pathErr := opts.ConfigPath(); pathErr == nil {
		configPath += fmt.Sprintf("at %q ", path)
	}
	return &oerrors.Error{
		Err:            oerrors.TrimErrBasicCredentialNotFound(err),
		Recommendation: fmt.Sprintf(`Please check whether the registry credential stored in the authentication file%sis correct`, configPath),
	}
}

// ConfigPath returns the config path of the credential store.
func (opts *Remote) ConfigPath() (string, error) {
	if opts.store == nil {
		return "", errors.New("no credential store initialized")
	}
	if ds, ok := opts.store.(*credentials.DynamicStore); ok {
		return ds.ConfigPath(), nil
	}
	return "", errors.New("store doesn't support getting config path")
}

// NewRepository assembles a oras remote repository.
func (opts *Remote) NewRepository(reference string, common Common, logger logrus.FieldLogger) (repo *remote.Repository, err error) {
	repo, err = remote.NewRepository(reference)
	if err != nil {
		if errors.Unwrap(err) == errdef.ErrInvalidReference {
			return nil, fmt.Errorf("%q: %v", reference, err)
		}
		return nil, err
	}
	registry := repo.Reference.Registry
	repo.PlainHTTP = opts.isPlainHttp(registry)
	repo.HandleWarning = opts.handleWarning(registry, logger)
	if repo.Client, err = opts.authClient(common.Debug); err != nil {
		return nil, err
	}
	repo.SkipReferrersGC = true
	if opts.ReferrersAPI != nil {
		if err := repo.SetReferrersCapability(*opts.ReferrersAPI); err != nil {
			return nil, err
		}
	}
	return
}

// Parse tries to read password with optional cmd prompt.
func (opts *Remote) Parse(cmd *cobra.Command) error {
	usernameAndIdTokenFlags := []string{opts.flagPrefix + usernameFlag, opts.flagPrefix + identityTokenFlag}
	passwordAndIdTokenFlags := []string{opts.flagPrefix + passwordFlag, opts.flagPrefix + identityTokenFlag}
	certFileAndKeyFileFlags := []string{opts.flagPrefix + certFileFlag, opts.flagPrefix + keyFileFlag}
	if cmd.Flags().Lookup(identityTokenFromStdinFlag) != nil {
		usernameAndIdTokenFlags = append(usernameAndIdTokenFlags, identityTokenFromStdinFlag)
		passwordAndIdTokenFlags = append(passwordAndIdTokenFlags, identityTokenFromStdinFlag)
	}
	if cmd.Flags().Lookup(passwordFromStdinFlag) != nil {
		passwordAndIdTokenFlags = append(passwordAndIdTokenFlags, passwordFromStdinFlag)
	}
	if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), usernameAndIdTokenFlags...); err != nil {
		return err
	}
	if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), passwordAndIdTokenFlags...); err != nil {
		return err
	}
	if err := opts.parseCustomHeaders(); err != nil {
		return err
	}
	if err := oerrors.CheckRequiredTogetherFlags(cmd.Flags(), certFileAndKeyFileFlags...); err != nil {
		return err
	}
	return opts.readSecret(cmd)
}

// readSecret tries to read password or identity token with
// optional cmd prompt.
func (opts *Remote) readSecret(cmd *cobra.Command) (err error) {
	if cmd.Flags().Changed(identityTokenFlag) {
		fmt.Fprintln(os.Stderr, "WARNING! Using --identity-token via the CLI is insecure. Use --identity-token-stdin.")
	} else if cmd.Flags().Changed(passwordFlag) {
		fmt.Fprintln(os.Stderr, "WARNING! Using --password via the CLI is insecure. Use --password-stdin.")
	} else if opts.secretFromStdin {
		// Prompt for credential
		secret, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		opts.Secret = strings.TrimSuffix(string(secret), "\n")
		opts.Secret = strings.TrimSuffix(opts.Secret, "\r")
	}
	return nil
}

func (opts *Remote) parseCustomHeaders() error {
	if len(opts.headerFlags) != 0 {
		headers := map[string][]string{}
		for _, h := range opts.headerFlags {
			name, value, found := strings.Cut(h, ":")
			if !found || strings.TrimSpace(name) == "" {
				// In conformance to the RFC 2616 specification
				// Reference: https://www.rfc-editor.org/rfc/rfc2616#section-4.2
				return fmt.Errorf("invalid header: %q", h)
			}
			headers[name] = append(headers[name], value)
		}
		opts.headers = headers
	}
	return nil
}
