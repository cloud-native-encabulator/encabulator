package settings

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// The in-cluster service account files, as the kubelet projects them.
const (
	serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"
	tokenFile         = serviceAccountDir + "/token"
	caFile            = serviceAccountDir + "/ca.crt"
)

// maxResponse bounds what is read back from the API server: one small ConfigMap.
const maxResponse = 1 << 20

// ErrNotInCluster means the process has no in-cluster API server address or
// service account token to talk to it with.
var ErrNotInCluster = errors.New("not running in a Kubernetes pod with a service account token")

// ConfigMap stores the configuration under Key in one ConfigMap, through the
// Kubernetes API. It needs exactly get and patch on that ConfigMap, and it never
// creates it: the deployment does, so the grammeter's Role can be limited to the
// one name.
type ConfigMap struct {
	url       string // .../api/v1/namespaces/<ns>/configmaps/<name>
	client    *http.Client
	tokenFile string
}

// ConfigMapOptions configures a ConfigMap store outside a pod (tests).
type ConfigMapOptions struct {
	// Server is the API server's base URL, e.g. https://10.96.0.1:443.
	Server    string
	Namespace string
	Name      string
	// Client may be nil, meaning a client with a 5s timeout and the system roots.
	Client *http.Client
	// TokenFile is read before every request, because projected tokens rotate.
	TokenFile string
}

// NewConfigMap builds a store from explicit options.
func NewConfigMap(o ConfigMapOptions) (*ConfigMap, error) {
	if o.Namespace == "" || o.Name == "" {
		return nil, errors.New("settings: a namespace and a ConfigMap name are required")
	}
	if o.Server == "" {
		return nil, errors.New("settings: an API server URL is required")
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 5 * time.Second}
	}
	return &ConfigMap{
		url: strings.TrimRight(o.Server, "/") + "/api/v1/namespaces/" + url.PathEscape(o.Namespace) +
			"/configmaps/" + url.PathEscape(o.Name),
		client:    o.Client,
		tokenFile: o.TokenFile,
	}, nil
}

// InCluster builds a store for ConfigMap name in namespace from the pod's own
// service account, the way client-go's in-cluster config does.
func InCluster(namespace, name string) (*ConfigMap, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, ErrNotInCluster
	}
	if _, err := os.Stat(tokenFile); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotInCluster, err)
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotInCluster, err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("settings: %s holds no certificate", caFile)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	return NewConfigMap(ConfigMapOptions{
		Server:    "https://" + net.JoinHostPort(host, port),
		Namespace: namespace,
		Name:      name,
		Client:    &http.Client{Timeout: 5 * time.Second, Transport: transport},
		TokenFile: tokenFile,
	})
}

// Load implements Store. A missing ConfigMap, or one without Key, is ErrNotSaved.
func (c *ConfigMap) Load(ctx context.Context) ([]byte, error) {
	body, status, err := c.do(ctx, http.MethodGet, nil, "")
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusNotFound:
		return nil, ErrNotSaved
	case status != http.StatusOK:
		return nil, apiError("read", status, body)
	}
	var cm struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(body, &cm); err != nil {
		return nil, fmt.Errorf("settings: the API server returned an unreadable ConfigMap: %w", err)
	}
	saved := cm.Data[Key]
	if strings.TrimSpace(saved) == "" {
		return nil, ErrNotSaved
	}
	return []byte(saved), nil
}

// Save implements Store with a JSON merge patch that sets only Key, leaving any
// other key and every label alone.
func (c *ConfigMap) Save(ctx context.Context, data []byte) error {
	patch, err := json.Marshal(map[string]any{"data": map[string]string{Key: string(data)}})
	if err != nil {
		return err
	}
	body, status, err := c.do(ctx, http.MethodPatch, patch, "application/merge-patch+json")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return apiError("write", status, body)
	}
	return nil
}

func (c *ConfigMap) do(ctx context.Context, method string, payload []byte, contentType string) ([]byte, int, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.tokenFile != "" {
		token, err := os.ReadFile(c.tokenFile)
		if err != nil {
			return nil, 0, fmt.Errorf("settings: read the service account token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("settings: %s ConfigMap: %w", strings.ToLower(method), err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, 0, fmt.Errorf("settings: read the API server's answer: %w", err)
	}
	return body, resp.StatusCode, nil
}

// apiError keeps the API server's own reason (a Status object's message), which
// says plainly when the Role is missing or the ConfigMap is not there.
func apiError(verb string, status int, body []byte) error {
	var st struct {
		Message string `json:"message"`
	}
	msg := http.StatusText(status)
	if json.Unmarshal(body, &st) == nil && st.Message != "" {
		msg = st.Message
	}
	return fmt.Errorf("settings: %s ConfigMap: %d %s", verb, status, msg)
}
