// loadtest signs in and fires many pay requests at the API, then reports
// how fast it answered and with which status codes.
//
// It builds its own signed regtest invoices, so it needs no Lightning node.
// It measures the accept path (sign-in check, invoice decode, database insert),
// not the Lightning payment itself.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/zpay32"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:8080", "server address")
	user := flag.String("user", "bob", "user to sign in as (needs the payer role)")
	n := flag.Int("n", 1000, "number of requests")
	c := flag.Int("c", 50, "how many at the same time")
	same := flag.Bool("same", false, "send the SAME invoice every time (duplicate storm)")
	flag.Parse()

	password := os.Getenv("LOAD_PASSWORD")
	if password == "" {
		fmt.Fprintln(os.Stderr, "set LOAD_PASSWORD to the user's password")
		os.Exit(1)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second,
		Transport: &http.Transport{MaxIdleConnsPerHost: *c, MaxConnsPerHost: *c}}

	login, _ := json.Marshal(map[string]string{"username": *user, "password": password})
	res, err := client.Post(*url+"/api/login", "application/json", bytes.NewReader(login))
	if err != nil || res.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "sign-in failed")
		os.Exit(1)
	}
	res.Body.Close()

	// Prepare the request bodies first, so signing invoices is not part of the timing.
	key, _ := btcec.NewPrivateKey()
	bodies := make([][]byte, *n)
	one := invoice(key)
	for i := range bodies {
		text := one
		if !*same {
			text = invoice(key)
		}
		bodies[i], _ = json.Marshal(map[string]string{"invoice": text})
	}

	var (
		mu     sync.Mutex
		codes  = map[int]int{}
		times  = make([]time.Duration, 0, *n)
		jobs   = make(chan []byte)
		wg     sync.WaitGroup
		failed int
	)
	start := time.Now()
	for range *c {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for body := range jobs {
				t := time.Now()
				res, err := client.Post(*url+"/api/payments", "application/json", bytes.NewReader(body))
				d := time.Since(t)
				mu.Lock()
				if err != nil {
					failed++
				} else {
					codes[res.StatusCode]++
					times = append(times, d)
					res.Body.Close()
				}
				mu.Unlock()
			}
		}()
	}
	for _, b := range bodies {
		jobs <- b
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	pct := func(p float64) time.Duration {
		if len(times) == 0 {
			return 0
		}
		return times[int(float64(len(times)-1)*p)].Round(100 * time.Microsecond)
	}
	fmt.Printf("requests=%d at-once=%d same-invoice=%v\n", *n, *c, *same)
	fmt.Printf("took=%s  per-second=%.0f  connection-errors=%d\n", total.Round(time.Millisecond), float64(*n)/total.Seconds(), failed)
	fmt.Printf("status codes: %v\n", codes)
	fmt.Printf("response time: p50=%s p95=%s p99=%s max=%s\n", pct(0.50), pct(0.95), pct(0.99), pct(1))
}

// invoice builds a signed 1,000-sat regtest invoice with a fresh payment hash.
func invoice(key *btcec.PrivateKey) string {
	var hash [32]byte
	_, _ = rand.Read(hash[:])
	inv, err := zpay32.NewInvoice(&chaincfg.RegressionNetParams, hash, time.Now(),
		zpay32.Amount(lnwire.MilliSatoshi(1_000_000)), zpay32.Description("load"), zpay32.Expiry(time.Hour))
	if err != nil {
		panic(err)
	}
	text, err := inv.Encode(zpay32.MessageSigner{SignCompact: func(msg []byte) ([]byte, error) {
		return ecdsa.SignCompact(key, chainhash.HashB(msg), true), nil
	}})
	if err != nil {
		panic(err)
	}
	return text
}
