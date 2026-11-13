// The server: one program that holds the API and the background workers.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/httpapi"
	"github.com/rupeshkoushik07/lightning-payments-service/internal/lnd"
	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
	"github.com/rupeshkoushik07/lightning-payments-service/internal/web"
	"github.com/rupeshkoushik07/lightning-payments-service/internal/worker"
	"github.com/rupeshkoushik07/lightning-payments-service/migrations"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("server stopped with an error", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	// ctx is cancelled when someone presses Ctrl+C or the platform says "stop".
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ---- 1. Settings come from the environment. Secrets are file paths, not values.
	maxAmount, err := strconv.ParseInt(env("MAX_AMOUNT_SAT", "100000"), 10, 64)
	if err != nil {
		return errors.New("MAX_AMOUNT_SAT must be a number")
	}

	// ---- 2. Database, and its tables.
	pool, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	db := store.New(pool)
	if err := db.Migrate(ctx, migrations.Files); err != nil {
		return err
	}

	// ---- 3. One connection to each node, each with its own small key.
	aliceConn, err := lnd.Dial(lnd.NodeConfig{
		Host: mustEnv("ALICE_HOST"), TLSCertPath: mustEnv("ALICE_TLS_CERT"), MacaroonPath: mustEnv("ALICE_MACAROON"),
	})
	if err != nil {
		return err
	}
	defer aliceConn.Close()
	bobConn, err := lnd.Dial(lnd.NodeConfig{
		Host: mustEnv("BOB_HOST"), TLSCertPath: mustEnv("BOB_TLS_CERT"), MacaroonPath: mustEnv("BOB_MACAROON"),
	})
	if err != nil {
		return err
	}
	defer bobConn.Close()

	receiver := lnd.NewReceiver(aliceConn)
	payer := lnd.NewPayer(bobConn)

	// ---- 4. People who may sign in.
	// Normal mode: passwords come from the environment once, and only their
	// bcrypt hashes are stored.
	// Demo mode (DEMO_LOGIN=true): no passwords exist anywhere. The page shows
	// one button per user. Allowed only when the server listens on this
	// machine, so nobody else on the network can reach those buttons.
	listenAddr := env("LISTEN_ADDR", "127.0.0.1:8080")
	demo := env("DEMO_LOGIN", "false") == "true"
	if demo && !isLoopback(listenAddr) {
		return errors.New("DEMO_LOGIN=true is only allowed when LISTEN_ADDR is on 127.0.0.1 or localhost")
	}
	for _, u := range []struct{ name, role, envKey string }{
		{"alice", "receiver", "ALICE_PASSWORD"},
		{"bob", "payer", "BOB_PASSWORD"},
		{"admin", "admin", "ADMIN_PASSWORD"},
	} {
		pw := os.Getenv(u.envKey)
		if demo {
			// A long random password nobody knows: the password door stays shut.
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return err
			}
			pw = hex.EncodeToString(raw)
		}
		if pw == "" {
			continue
		}
		hash, err := httpapi.HashPassword(pw)
		if err != nil {
			return errors.New(u.envKey + ": " + err.Error())
		}
		if err := db.UpsertUser(ctx, u.name, hash, u.role); err != nil {
			return err
		}
	}

	// ---- 5. The API. Every route except sign-in needs a session,
	//         and each route names the roles that may use it.
	auth := &httpapi.Auth{Users: db, SecureCookie: env("COOKIE_SECURE", "false") == "true", Demo: demo}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", auth.Login)
	mux.HandleFunc("GET /api/demo", auth.DemoInfo)
	mux.HandleFunc("POST /api/demo-login", auth.DemoLogin)
	mux.HandleFunc("POST /api/logout", auth.Logout)
	mux.Handle("GET /api/me", auth.Require(http.HandlerFunc(auth.Me)))

	mux.Handle("POST /api/invoices", auth.Require(
		&httpapi.InvoiceHandler{Node: receiver, Invoices: db, MaxAmountSat: maxAmount, ValidFor: time.Hour},
		"receiver", "admin"))
	mux.Handle("GET /api/invoices", auth.Require(httpapi.ListInvoices(db)))
	mux.Handle("GET /api/invoices/{hash}", auth.Require(httpapi.GetInvoice(db)))

	mux.Handle("POST /api/payments", auth.Require(
		&httpapi.PayHandler{Pause: db, Decoder: lnd.NewRegtestDecoder(), Payments: db, MaxAmountSat: maxAmount},
		"payer", "admin"))
	mux.Handle("GET /api/payments", auth.Require(httpapi.ListPayments(db)))
	mux.Handle("GET /api/payments/{id}", auth.Require(httpapi.GetPayment(db)))

	mux.Handle("GET /api/nodes", auth.Require(&httpapi.Nodes{
		Receiver: receiver, ReceiverHost: mustEnv("ALICE_HOST"),
		Payer: payer, PayerHost: mustEnv("BOB_HOST"),
	}))

	mux.Handle("GET /api/admin/payouts", auth.Require(httpapi.GetPause(db), "admin"))
	mux.Handle("POST /api/admin/payouts", auth.Require(httpapi.SetPause(db), "admin"))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.Handle("GET /", web.Handler()) // the browser pages

	srv := &http.Server{
		Addr:              listenAddr, // this machine only, by default
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ---- 6. The background workers. Each runs until ctx is cancelled.
	var workers sync.WaitGroup
	start := func(job func()) {
		workers.Add(1)
		go func() { defer workers.Done(); job() }()
	}

	tracker := &worker.Tracker{Node: payer, Payments: db, Log: log}
	sender := &worker.Sender{Node: payer, Payments: db, Log: log}
	listener := &worker.Listener{Node: receiver, Invoices: db, Log: log}

	// Tracker first: settle whatever the last run left IN_FLIGHT, then every 5 seconds.
	start(func() {
		every(ctx, 5*time.Second, func() {
			if err := tracker.ResolveAll(ctx); err != nil {
				log.Warn("tracker round failed", "err", err)
			}
		})
	})
	// Two senders, so two payments can be on their way at once.
	for range 2 {
		start(func() {
			for ctx.Err() == nil {
				err := sender.PayOne(ctx)
				if errors.Is(err, store.ErrNothingToDo) {
					sleep(ctx, 500*time.Millisecond)
				} else if err != nil {
					log.Warn("sender round failed", "err", err)
					sleep(ctx, 2*time.Second)
				}
			}
		})
	}
	start(func() { listener.Run(ctx) })
	start(func() {
		every(ctx, time.Minute, func() {
			if n, err := db.ExpireInvoices(ctx); err != nil {
				log.Warn("expiry round failed", "err", err)
			} else if n > 0 {
				log.Info("invoices expired", "count", n)
			}
			if err := db.DeleteExpiredSessions(ctx); err != nil {
				log.Warn("session clean-up failed", "err", err)
			}
		})
	})

	// ---- 7. Serve until told to stop.
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	log.Info("server started", "addr", srv.Addr)

	// Metrics on their own port, so Prometheus never needs a session
	// and the numbers are not on the public API. Same machine only by default.
	metricsAddr := env("METRICS_ADDR", "127.0.0.1:9101")
	metricsSrv := &http.Server{Addr: metricsAddr, Handler: promhttp.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("metrics server stopped", "err", err)
		}
	}()
	defer metricsSrv.Close()
	log.Info("metrics served", "addr", metricsAddr+"/metrics")

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	// ---- 8. Graceful shutdown: no new requests, 20 seconds for running ones,
	//         workers stop, unfinished payments stay IN_FLIGHT for the next start.
	log.Info("shutting down")
	stop() // a second Ctrl+C now kills the program immediately
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("some requests did not finish in time", "err", err)
	}
	workers.Wait()
	log.Info("stopped cleanly")
	return nil
}

// every runs job now, then again each interval, until ctx is cancelled.
func every(ctx context.Context, interval time.Duration, job func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		job()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("missing required setting", "name", key)
		os.Exit(1)
	}
	return v
}

// isLoopback reports whether addr ("host:port") can only be reached from this machine.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
