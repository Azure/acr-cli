// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package api

import (
	"context"
	"io"

	orasauth "github.com/Azure/acr-cli/auth/oras"
	"github.com/Azure/acr-cli/cmd/api/command"
	"github.com/Azure/acr-cli/cmd/api/display"
	"github.com/Azure/acr-cli/cmd/api/option"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
)

type contextKey int

const (
	DockerMediaTypeManifest = "application/vnd.docker.distribution.manifest.v2+json"
	// MediaTypeImageManifest specifies the media type for an image manifest.
	MediaTypeImageManifest = "application/vnd.oci.image.manifest.v1+json"
	// MediaTypeArtifactManifest specifies the media type for a content descriptor.
	MediaTypeArtifactManifest = "application/vnd.oci.artifact.manifest.v1+json"
	// loggerKey is the associated key type for logger entry in context.
	// loggerKey           contextKey = iota
	// TargetTypeRemote               = "registry"
	// TargetTypeOCILayout            = "oci-layout"
)

// var (
// 	// Version is the current version of the oras.
// 	Version = "1.2.0-rc.1"
// 	// BuildMetadata is the extra build time data
// 	BuildMetadata  = "unreleased"
// 	FormatTypeJSON = &FormatType{
// 		Name:  "json",
// 		Usage: "Print in JSON format",
// 	}
// 	FormatTypeGoTemplate = &FormatType{
// 		Name:      "go-template",
// 		Usage:     "Print output using the given Go template",
// 		HasParams: true,
// 	}
// 	FormatTypeTable = &FormatType{
// 		Name:  "table",
// 		Usage: "Get direct referrers and output in table format",
// 	}
// 	FormatTypeTree = &FormatType{
// 		Name:  "tree",
// 		Usage: "Get referrers recursively and print in tree format",
// 	}
// )

// The ORASClient wraps the oras-go sdk and is used for interacting with artifacts in a registry.
// it implements the ORASClientInterface.
type ORASClient struct {
	client remote.Client
}

// Artifact describes an artifact manifest.
// This structure provides `application/vnd.oci.artifact.manifest.v1+json` mediatype when marshalled to JSON.
//
// This manifest type was introduced in image-spec v1.1.0-rc1 and was removed in
// image-spec v1.1.0-rc3. It is not part of the current image-spec and is kept
// here for Go compatibility.
//
// Reference: https://github.com/opencontainers/image-spec/pull/999
type Artifact struct {
	// MediaType is the media type of the object this schema refers to.
	MediaType string `json:"mediaType"`

	// ArtifactType is the IANA media type of the artifact this schema refers to.
	ArtifactType string `json:"artifactType"`

	// Blobs is a collection of blobs referenced by this manifest.
	Blobs []ocispec.Descriptor `json:"blobs,omitempty"`

	// Subject (reference) is an optional link from the artifact to another manifest forming an association between the artifact and the other manifest.
	Subject *ocispec.Descriptor `json:"subject,omitempty"`

	// Annotations contains arbitrary metadata for the artifact manifest.
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Common option struct.
// type Common struct {
// 	Debug   bool
// 	Verbose bool
// 	TTY     *os.File

// 	noTTY bool
// }

// // Platform option struct.
// type Platform struct {
// 	platform        string
// 	Platform        *ocispec.Platform
// 	FlagDescription string
// }

// // DistributionSpec option struct which implements pflag.Value interface.
// type DistributionSpec struct {
// 	// ReferrersAPI indicates the preference of the implementation of the Referrers API.
// 	// Set to true for referrers API, false for referrers tag scheme, and nil for auto fallback.
// 	ReferrersAPI *bool

// 	// specFlag should be provided in form of`<version>-<api>-<option>`
// 	flag string
// }

// // Remote options struct contains flags and arguments specifying one registry.
// // Remote implements oerrors.Handler and interface.
// type Remote struct {
// 	DistributionSpec
// 	CACertFilePath  string
// 	CertFilePath    string
// 	KeyFilePath     string
// 	Insecure        bool
// 	Configs         []string
// 	Username        string
// 	secretFromStdin bool
// 	Secret          string
// 	flagPrefix      string

// 	resolveFlag           []string
// 	applyDistributionSpec bool
// 	headerFlags           []string
// 	headers               http.Header
// 	warned                map[string]*sync.Map
// 	plainHTTP             func() (plainHTTP bool, enforced bool)
// 	store                 credentials.Store
// }

// // Target struct contains flags and arguments specifying one registry or image
// // layout.
// // Target implements oerrors.Handler interface.
// type Target struct {
// 	Remote
// 	RawReference string
// 	Type         string
// 	Reference    string //contains tag or digest
// 	// Path contains
// 	//  - path to the OCI image layout target, or
// 	//  - registry and repository for the remote target
// 	Path string

// 	IsOCILayout bool
// }

// // FormatType represents a format type.
// type FormatType struct {
// 	// Name is the format type name.
// 	Name string
// 	// Usage is the usage string in help doc.
// 	Usage string
// 	// HasParams indicates whether the format type has parameters.
// 	HasParams bool
// }

// // Format contains input and parsed options for formatted output flags.
// type Format struct {
// 	FormatFlag   string
// 	Type         string
// 	Template     string
// 	AllowedTypes []*FormatType
// }

type discoverOptions struct {
	option.Common
	option.Platform
	option.Target
	option.Format

	artifactType string
}

// GraphTarget is a tracked oras.GraphTarget.
type GraphTarget interface {
	oras.GraphTarget
	io.Closer
	Prompt(desc ocispec.Descriptor, prompt string) error
	Inner() oras.GraphTarget
}

// // Transport is an http.RoundTripper that keeps track of the in-flight
// // request and add hooks to report HTTP tracing events.
// type Transport struct {
// 	http.RoundTripper
// }

// // NewTransport creates and returns a new instance of Transport
// func NewTransport(base http.RoundTripper) *Transport {
// 	return &Transport{
// 		RoundTripper: base,
// 	}
// }

func (o *ORASClient) Annotate(ctx context.Context, reference string, artifactType string, annotationsArg map[string]string) error {
	dst, err := o.getTarget(reference)
	if err != nil {
		return err
	}

	// do the equivalent of
	// prepare manifest
	store, err := file.New("")
	if err != nil {
		return err
	}
	defer store.Close()

	subject, err := dst.Resolve(ctx, reference)
	if err != nil {
		return err
	}

	graphCopyOptions := oras.DefaultCopyGraphOptions
	packOpts := oras.PackManifestOptions{
		Subject:             &subject,
		ManifestAnnotations: annotationsArg,
	}

	pack := func() (ocispec.Descriptor, error) {
		return oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, artifactType, packOpts)
	}

	copyFunc := func(root ocispec.Descriptor) error {
		return oras.CopyGraph(ctx, store, dst, root, graphCopyOptions)
	}

	// Attach
	_, err = doPush(dst, pack, copyFunc)
	if err != nil {
		return err
	}

	return nil
}

func doPush(dst oras.Target, pack packFunc, copy copyFunc) (ocispec.Descriptor, error) {
	if tracked, ok := dst.(GraphTarget); ok {
		defer tracked.Close()
	}
	// Push
	return pushArtifact(pack, copy)
}

type packFunc func() (ocispec.Descriptor, error)
type copyFunc func(desc ocispec.Descriptor) error

func pushArtifact(pack packFunc, copy copyFunc) (ocispec.Descriptor, error) {
	root, err := pack()
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	// push
	if err = copy(root); err != nil {
		return ocispec.Descriptor{}, err
	}
	return root, nil
}

// getTarget gets an oras remote.Repository object that refers to the target of our annotation request
func (o *ORASClient) getTarget(reference string) (repo *remote.Repository, err error) {
	repo, err = remote.NewRepository(reference)
	if err != nil {
		return nil, err
	}

	repo.SkipReferrersGC = true
	repo.Client = o.client
	repo.SetReferrersCapability(true)
	return repo, nil
}

func (o *ORASClient) Discover(cmd *cobra.Command, opts *discoverOptions) error {
	ctx, logger := command.GetLogger(cmd, &opts.Common)
	repo, err := opts.NewReadonlyTarget(ctx, opts.Common, logger)
	if err != nil {
		return err
	}
	resolveOpts := oras.DefaultResolveOptions
	desc, err := oras.Resolve(ctx, repo, opts.Reference, resolveOpts)
	if err != nil {
		return err
	}
	handler, err := display.NewDiscoverHandler(cmd.OutOrStdout(), opts.Format, opts.Path, opts.RawReference, desc, opts.Verbose)
	if err != nil {
		return err
	}
	refs, err := registry.Referrers(ctx, repo, desc, opts.artifactType)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := handler.OnDiscovered(ref, desc); err != nil {
			return err
		}
	}

	return handler.OnCompleted()
}

// // discoverHandler handles json metadata output for discover events.
// type discoverHandler struct {
// 	out       io.Writer
// 	root      ocispec.Descriptor
// 	path      string
// 	referrers []ocispec.Descriptor
// }

// NewDiscoverHandler creates a new handler for discover events.
// func NewDiscoverHandler(out io.Writer, root ocispec.Descriptor, path string) metadata.DiscoverHandler {
// 	return &discoverHandler{
// 		out:  out,
// 		root: root,
// 		path: path,
// 	}
// }

// DiscoverHandler handles metadata output for discover events.
// type DiscoverHandler interface {
// 	// MultiLevelSupported returns true if the handler supports multi-level
// 	// discovery.
// 	MultiLevelSupported() bool
// 	// OnDiscovered is called after a referrer is discovered.
// 	OnDiscovered(referrer, subject ocispec.Descriptor) error
// 	// OnCompleted is called when referrer discovery is completed.
// 	OnCompleted() error
// }

// NewDiscoverHandler returns status and metadata handlers for discover command.
// func NewDiscoverHandler(out io.Writer, format Format, path string, rawReference string, desc ocispec.Descriptor, verbose bool) (DiscoverHandler, error) {
// 	var handler DiscoverHandler
// 	switch format.Type {
// 	case FormatTypeTree.Name, "":
// 		handler = tree.NewDiscoverHandler(out, path, desc, verbose)
// 	case FormatTypeTable.Name:
// 		handler = table.NewDiscoverHandler(out, rawReference, desc, verbose)
// 	case FormatTypeJSON.Name:
// 		handler = json.NewDiscoverHandler(out, desc, path)
// 	case FormatTypeGoTemplate.Name:
// 		handler = template.NewDiscoverHandler(out, desc, path, format.Template)
// 	default:
// 		return nil, errors.UnsupportedFormatTypeError(format.Type)
// 	}
// 	return handler, nil
// }

// // Node represents a tree node.
// type Node struct {
// 	Value any
// 	Nodes []*Node
// }

// // New creates a new tree / root node.
// func New(value any) *Node {
// 	return &Node{
// 		Value: value,
// 	}
// }

// discoverHandler handles json metadata output for discover events.
// type discoverHandlerMetadata struct {
// 	out     io.Writer
// 	path    string
// 	root    Node
// 	nodes   map[digest.Digest]Node
// 	verbose bool
// }

// // NewDiscoverHandlerEvent creates a new handler for discover events.
// func NewDiscoverHandlerEvent(out io.Writer, path string, root ocispec.Descriptor, verbose bool) DiscoverHandler {
// 	treeRoot := New(fmt.Sprintf("%s@%s", path, root.Digest))
// 	return &discoverHandlerMetadata{
// 		out:  out,
// 		path: path,
// 		root: treeRoot,
// 		nodes: map[digest.Digest]*tree.Node{
// 			root.Digest: treeRoot,
// 		},
// 		verbose: verbose,
// 	}
// }

// OnDiscovered implements metadata.DiscoverHandler.
// func (h *discoverHandler) OnDiscovered(referrer, subject ocispec.Descriptor) error {
// 	if !content.Equal(subject, h.root) {
// 		return fmt.Errorf("unexpected subject descriptor: %v", subject)
// 	}
// 	h.referrers = append(h.referrers, referrer)
// 	return nil
// }

// // NewReadonlyTargets generates a new read only target based on opts.
// func (opts *Target) NewReadonlyTarget(ctx context.Context, common Common, logger logrus.FieldLogger) (ReadOnlyGraphTagFinderTarget, error) {
// 	switch opts.Type {
// 	case TargetTypeOCILayout:
// 		info, err := os.Stat(opts.Path)
// 		if err != nil {
// 			if errors.Is(err, fs.ErrNotExist) {
// 				return nil, fmt.Errorf("invalid argument %q: failed to find path %q: %w", opts.RawReference, opts.Path, err)
// 			}
// 			return nil, err
// 		}
// 		if info.IsDir() {
// 			return oci.NewFromFS(ctx, os.DirFS(opts.Path))
// 		}
// 		store, err := oci.NewFromTar(ctx, opts.Path)
// 		if err != nil {
// 			if errors.Is(err, io.ErrUnexpectedEOF) {
// 				return nil, fmt.Errorf("%q does not look like a tar archive: %w", opts.Path, err)
// 			}
// 			return nil, err
// 		}
// 		return store, nil
// 	case TargetTypeRemote:
// 		repo, err := opts.NewRepository(opts.RawReference, common, logger)
// 		if err != nil {
// 			return nil, err
// 		}
// 		tmp := repo.Reference
// 		tmp.Reference = ""
// 		opts.Path = tmp.String()
// 		opts.Reference = repo.Reference.Reference
// 		return repo, nil
// 	}
// 	return nil, fmt.Errorf("unknown target type: %q", opts.Type)
// }

// NewRepository assembles a oras remote repository.
// func (opts *Remote) NewRepository(reference string, common Common, logger logrus.FieldLogger) (repo *remote.Repository, err error) {
// 	repo, err = remote.NewRepository(reference)
// 	if err != nil {
// 		if errors.Unwrap(err) == errdef.ErrInvalidReference {
// 			return nil, fmt.Errorf("%q: %v", reference, err)
// 		}
// 		return nil, err
// 	}
// 	registry := repo.Reference.Registry
// 	repo.PlainHTTP = opts.isPlainHttp(registry)
// 	repo.HandleWarning = opts.handleWarning(registry, logger)
// 	if repo.Client, err = opts.authClient(registry, common.Debug); err != nil {
// 		return nil, err
// 	}
// 	repo.SkipReferrersGC = true
// 	if opts.ReferrersAPI != nil {
// 		if err := repo.SetReferrersCapability(*opts.ReferrersAPI); err != nil {
// 			return nil, err
// 		}
// 	}
// 	return
// }

// authClient assembles a oras auth client.
// func (opts *Remote) authClient(registry string, debug bool) (client *auth.Client, err error) {
// 	config, err := opts.tlsConfig()
// 	if err != nil {
// 		return nil, err
// 	}
// 	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
// 	baseTransport.TLSClientConfig = config
// 	dialContext, err := opts.parseResolve(baseTransport.DialContext)
// 	if err != nil {
// 		return nil, err
// 	}
// 	baseTransport.DialContext = dialContext
// 	client = &auth.Client{
// 		Client: &http.Client{
// 			// http.RoundTripper with a retry using the DefaultPolicy
// 			// see: https://pkg.go.dev/oras.land/oras-go/v2/registry/remote/retry#Policy
// 			Transport: retry.NewTransport(baseTransport),
// 		},
// 		Cache:  auth.NewCache(),
// 		Header: opts.headers,
// 	}
// 	client.SetUserAgent("oras/" + GetVersion())
// 	if debug {
// 		client.Client.Transport = NewTransport(client.Client.Transport)
// 	}

// 	cred := opts.Credential()
// 	if cred != auth.EmptyCredential {
// 		client.Credential = func(ctx context.Context, s string) (auth.Credential, error) {
// 			return cred, nil
// 		}
// 	} else {
// 		var err error
// 		opts.store, err = NewStore(opts.Configs...)
// 		if err != nil {
// 			return nil, err
// 		}
// 		client.Credential = credentials.Credential(opts.store)
// 	}
// 	return
// }

// NewStore generates a store based on the passed-in config file paths.
// func NewStore(configPaths ...string) (credentials.Store, error) {
// 	opts := credentials.StoreOptions{AllowPlaintextPut: true}
// 	if len(configPaths) == 0 {
// 		// use default docker config file path
// 		return credentials.NewStoreFromDocker(opts)
// 	}

// 	var stores []credentials.Store
// 	for _, config := range configPaths {
// 		store, err := credentials.NewStore(config, opts)
// 		if err != nil {
// 			return nil, err
// 		}
// 		stores = append(stores, store)
// 	}
// 	return credentials.NewStoreWithFallbacks(stores[0], stores[1:]...), nil
// }

// Credential returns a credential based on the remote options.
// func (opts *Remote) Credential() auth.Credential {
// 	return Credential(opts.Username, opts.Secret)
// }

// // Credential converts user input username and password to a credential.
// func Credential(username, password string) auth.Credential {
// 	if username == "" {
// 		return auth.Credential{
// 			RefreshToken: password,
// 		}
// 	}
// 	return auth.Credential{
// 		Username: username,
// 		Password: password,
// 	}
// }

// GetVersion returns the semver string of the version
// func GetVersion() string {
// 	if BuildMetadata == "" {
// 		return Version
// 	}
// 	return Version + "+" + BuildMetadata
// }

// // parseResolve parses resolve flag.
// func (opts *Remote) parseResolve(baseDial DialFunc) (DialFunc, error) {
// 	if len(opts.resolveFlag) == 0 {
// 		return baseDial, nil
// 	}

// 	formatError := func(param, message string) error {
// 		return fmt.Errorf("failed to parse resolve flag %q: %s", param, message)
// 	}
// 	var dialer Dialer
// 	for _, r := range opts.resolveFlag {
// 		parts := strings.SplitN(r, ":", 4)
// 		length := len(parts)
// 		if length < 3 {
// 			return nil, formatError(r, "expecting host:port:address[:address_port]")
// 		}
// 		host := parts[0]
// 		hostPort, err := strconv.Atoi(parts[1])
// 		if err != nil {
// 			return nil, formatError(r, "expecting uint64 host port")
// 		}
// 		// ipv6 zone is not parsed
// 		address := net.ParseIP(parts[2])
// 		if address == nil {
// 			return nil, formatError(r, "invalid IP address")
// 		}
// 		addressPort := hostPort
// 		if length > 3 {
// 			addressPort, err = strconv.Atoi(parts[3])
// 			if err != nil {
// 				return nil, formatError(r, "expecting uint64 address port")
// 			}
// 		}
// 		dialer.Add(host, hostPort, address, addressPort)
// 	}
// 	dialer.BaseDialContext = baseDial
// 	return dialer.DialContext, nil
// }

// // DialFunc is the function type for http.DialContext.
// type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// // Dialer struct provides dialing function with predefined DNS resolves.
// type Dialer struct {
// 	BaseDialContext DialFunc
// 	resolve         map[string]string
// }

// // Add adds an entry for DNS resolve.
// func (d *Dialer) Add(from string, fromPort int, to net.IP, toPort int) {
// 	if d.resolve == nil {
// 		d.resolve = make(map[string]string)
// 	}
// 	d.resolve[fmt.Sprintf("%s:%d", from, fromPort)] = fmt.Sprintf("%s:%d", to, toPort)
// }

// // DialContext connects to the addr on the named network using the provided
// // context.
// func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
// 	if resolved, ok := d.resolve[addr]; ok {
// 		addr = resolved
// 	}
// 	return d.BaseDialContext(ctx, network, addr)
// }

// // tlsConfig assembles the tls config.
// func (opts *Remote) tlsConfig() (*tls.Config, error) {
// 	config := &tls.Config{
// 		InsecureSkipVerify: opts.Insecure,
// 	}
// 	if opts.CACertFilePath != "" {
// 		var err error
// 		config.RootCAs, err = LoadCertPool(opts.CACertFilePath)
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
// 	if opts.CertFilePath != "" && opts.KeyFilePath != "" {
// 		cert, err := tls.LoadX509KeyPair(opts.CertFilePath, opts.KeyFilePath)
// 		if err != nil {
// 			return nil, err
// 		}
// 		config.Certificates = []tls.Certificate{cert}
// 	}
// 	return config, nil
// }

// LoadCertPool returns a new cert pool loaded from the cert file.
// func LoadCertPool(path string) (*x509.CertPool, error) {
// 	pool := x509.NewCertPool()
// 	pemBytes, err := os.ReadFile(path)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
// 		return nil, errors.New("Failed to load certificate in file: " + path)
// 	}
// 	return pool, nil
// }

// func (opts *Remote) handleWarning(registry string, logger logrus.FieldLogger) func(warning remote.Warning) {
// 	if opts.warned == nil {
// 		opts.warned = make(map[string]*sync.Map)
// 	}
// 	warned := opts.warned[registry]
// 	if warned == nil {
// 		warned = &sync.Map{}
// 		opts.warned[registry] = warned
// 	}
// 	logger = logger.WithField("registry", registry)
// 	return func(warning remote.Warning) {
// 		if _, loaded := warned.LoadOrStore(warning.WarningValue, struct{}{}); !loaded {
// 			logger.Warn(warning.Text)
// 		}
// 	}
// }

// // isPlainHttp returns the plain http flag for a given registry.
// func (opts *Remote) isPlainHttp(registry string) bool {
// 	plainHTTP, enforced := opts.plainHTTP()
// 	if enforced {
// 		return plainHTTP
// 	}
// 	host, _, _ := net.SplitHostPort(registry)
// 	if host == "localhost" || registry == "localhost" {
// 		// not specified, defaults to plain http for localhost
// 		return true
// 	}
// 	return plainHTTP
// }

// GetLogger returns a new FieldLogger and an associated Context derived from command context.
// func GetLogger(cmd *cobra.Command, opts Common) (context.Context, logrus.FieldLogger) {
// 	ctx, logger := NewLogger(cmd.Context(), opts.Debug, opts.Verbose)
// 	cmd.SetContext(ctx)
// 	return ctx, logger
// }

// // NewLogger returns a logger.
// func NewLogger(ctx context.Context, debug bool, verbose bool) (context.Context, logrus.FieldLogger) {
// 	var logLevel logrus.Level
// 	if debug {
// 		logLevel = logrus.DebugLevel
// 	} else if verbose {
// 		logLevel = logrus.InfoLevel
// 	} else {
// 		logLevel = logrus.WarnLevel
// 	}

// 	logger := logrus.New()
// 	logger.SetFormatter(&logrus.TextFormatter{DisableQuote: true})
// 	logger.SetLevel(logLevel)
// 	entry := logger.WithContext(ctx)
// 	return context.WithValue(ctx, loggerKey, entry), entry
// }

func GetORASClientWithAuth(username string, password string, configs []string) (*ORASClient, error) {
	clientOpts := orasauth.ClientOptions{}
	if username != "" && password != "" {
		clientOpts.Credential = orasauth.Credential(username, password)
	} else {
		store, err := orasauth.NewStore(configs...)
		if err != nil {
			return nil, err
		}
		clientOpts.CredentialStore = store
	}
	c := orasauth.NewClient(clientOpts)
	orasClient := ORASClient{
		client: c,
	}
	return &orasClient, nil
}

// ORASClientInterface defines the required methods that the acr-cli will need to use with ORAS.
type ORASClientInterface interface {
	Annotate(ctx context.Context, reference string, artifactType string, annotations map[string]string) error
}
