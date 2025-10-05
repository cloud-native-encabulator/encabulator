package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/rs/zerolog"
)

type ReverseProxy struct {
	target *url.URL
	proxy  *httputil.ReverseProxy
	logger zerolog.Logger
}

func NewReverseProxy(targetURL string, logger zerolog.Logger) (*ReverseProxy, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the director to add headers
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Origin-Host", target.Host)
		req.Header.Set("X-Gateway", "dingle-arm-api-gateway")
	}

	// Add error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error().Err(err).Str("url", r.URL.String()).Msg("Proxy error")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"Bad Gateway","message":"Failed to reach backend service"}`))
	}

	return &ReverseProxy{
		target: target,
		proxy:  proxy,
		logger: logger,
	}, nil
}

func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	rp.logger.Debug().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("target", rp.target.String()).
		Msg("Proxying request")

	rp.proxy.ServeHTTP(w, r)

	duration := time.Since(start)
	rp.logger.Info().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Dur("duration", duration).
		Msg("Request proxied")
}
