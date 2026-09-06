// Command demo is a minimal host composing moat, cairn and redisstore (issue
// #51, IR-05): create and resolve routes, wired the way docs/INTEGRATION.md
// describes, running against a real Redis configured per ADR-0008's
// operational contract.
//
// It is the target for the threat probes of docs/security/probe-threats.sh
// (NFR-17) and the thing a reviewer runs without reading code. See
// demo/README.md for the curl sequence.
//
// What it deliberately omits, stated so nobody infers coverage from silence:
// there is no authentication, so /links/{code}/revoke is reachable by anyone
// who knows a code. ADR-0010 is explicit that cairn.Revoke never checks
// ownership -- a host exposing it enforces that check itself. This demo has
// no user accounts to check it against, so it does not pretend to. Do not
// deploy this file as a production revoke endpoint.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JonasBorgesLM/moat/preset"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/cairnhttp"
	"github.com/JonasBorgesLM/cairn/policy"
	"github.com/JonasBorgesLM/cairn/redisstore"
)

func main() {
	if err := run(); err != nil {
		slog.Error("demo exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := configFromEnv()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	store, err := redisstore.New(cfg.redisAddr)
	if err != nil {
		return fmt.Errorf("connecting to redis: %w", err)
	}
	//nolint:errcheck // best-effort cleanup at shutdown; nothing left to report a close error to.
	defer func() { _ = store.Close() }()

	shortener, err := cairn.New(store,
		cairn.WithPolicy(policy.Default([]string{cfg.ownDomain})),
		cairn.WithCounter(store),
		cairn.WithHooks(cairn.Hooks{
			OnReject: func(_ context.Context, ev cairn.RejectEvent) {
				logger.Warn("destination rejected", "reason", ev.Reason)
			},
		}),
	)
	if err != nil {
		return fmt.Errorf("constructing shortener (ADR-0008's EvictionCheck runs here): %w", err)
	}

	redirectHandler := cairnhttp.NewHandler(shortener)
	//nolint:errcheck // best-effort cleanup at shutdown; nothing left to report a close error to.
	defer func() { _ = redirectHandler.Close() }()

	api := &linksAPI{shortener: shortener, baseURL: cfg.baseURL, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /links", api.create)
	mux.HandleFunc("POST /links/{code}/revoke", api.revoke)
	mux.Handle("/", redirectHandler)

	chain, err := preset.API(preset.Config{
		// No cookies, no browser-submitted forms: every request here is
		// either a curl call or a redirect follow, so CSRF has nothing to
		// protect against that a forged cross-site request could reach any
		// other way (see preset.Config.DisableCSRF's own doc comment).
		DisableCSRF: true,
		// The demo runs as a single container with no reverse proxy in front
		// (docker-compose.yml maps its port directly), so r.RemoteAddr
		// already is the client -- the honest answer to the proxy-topology
		// question preset.API refuses to default (see ErrProxyTopologyUnspecified).
		// A real deployment behind a load balancer would set TrustedProxies
		// instead.
		DirectlyExposed: true,
		RateLimitBurst:  40,
		RateLimitPerSec: 20,
		OnError: func(err error) {
			logger.Error("rate-limit store error", "error", err)
		},
	})
	if err != nil {
		return fmt.Errorf("building moat preset chain: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           chain.Then(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	logger.Info("demo listening", "addr", cfg.addr, "redis", cfg.redisAddr, "own_domain", cfg.ownDomain)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listen: %w", err)
		}
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}

type config struct {
	addr      string
	redisAddr string
	ownDomain string
	baseURL   string
}

func configFromEnv() config {
	return config{
		addr:      getEnv("PORT_ADDR", ":8080"),
		redisAddr: getEnv("REDIS_ADDR", "localhost:6379"),
		ownDomain: getEnv("OWN_DOMAIN", "localhost:8080"),
		baseURL:   getEnv("BASE_URL", "http://localhost:8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// linksAPI is the host-side create/revoke surface cairnhttp does not provide
// -- cairnhttp is the redirect route only (ADR-0014's division of labour).
type linksAPI struct {
	shortener *cairn.Shortener
	baseURL   string
	logger    *slog.Logger
}

type createRequest struct {
	URL string `json:"url"`
}

type createResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (a *linksAPI) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	link, err := a.shortener.Create(r.Context(), req.URL)
	if err != nil {
		a.writeCreateError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	// #nosec G104 -- the status is already written; a write error here means the connection is gone, and there is nothing left to report it to.
	//nolint:errcheck // see the #nosec comment above.
	json.NewEncoder(w).Encode(createResponse{
		Code:     string(link.Code),
		ShortURL: a.baseURL + "/" + string(link.Code),
	})
}

func (a *linksAPI) revoke(w http.ResponseWriter, r *http.Request) {
	code := cairn.Code(r.PathValue("code"))
	if err := a.shortener.Revoke(r.Context(), code); err != nil {
		switch {
		case errors.Is(err, cairn.ErrInvalidCode), errors.Is(err, cairn.ErrCodeNotFound):
			writeError(w, http.StatusNotFound, "link not found")
		case errors.Is(err, cairn.ErrStoreUnavailable):
			writeError(w, http.StatusServiceUnavailable, "store unavailable")
		default:
			a.logger.Error("revoke failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *linksAPI) writeCreateError(w http.ResponseWriter, err error) {
	var rerr *cairn.RejectionError
	switch {
	case errors.As(err, &rerr):
		writeError(w, http.StatusUnprocessableEntity, "destination rejected: "+string(rerr.Reason))
	case errors.Is(err, cairn.ErrCodeSpaceExhausted), errors.Is(err, cairn.ErrStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "temporarily unavailable")
	default:
		a.logger.Error("create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// #nosec G104 -- the status is already written; a write error here means the connection is gone, and there is nothing left to report it to.
	//nolint:errcheck // see the #nosec comment above.
	json.NewEncoder(w).Encode(errorResponse{Error: msg})
}
