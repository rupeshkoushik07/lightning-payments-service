// lndcheck proves we can reach a node: it connects and lists its latest invoices.
// It only reads. It works with the invoice macaroon, which cannot spend.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/lightningnetwork/lnd/lnrpc"

	"github.com/rupeshkoushik07/ln-payment-go/internal/lnd"
)

func main() {
	host := flag.String("host", "127.0.0.1:10001", "node gRPC address")
	cert := flag.String("cert", "", "path to the node's tls.cert")
	mac := flag.String("macaroon", "", "path to invoice.macaroon")
	flag.Parse()

	conn, err := lnd.Dial(lnd.NodeConfig{Host: *host, TLSCertPath: *cert, MacaroonPath: *mac})
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not set up connection:", err)
		os.Exit(1)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := lnrpc.NewLightningClient(conn).ListInvoices(ctx, &lnrpc.ListInvoiceRequest{
		NumMaxInvoices: 5,
		Reversed:       true, // newest first
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "node refused or unreachable:", err)
		os.Exit(1)
	}

	fmt.Printf("connected to %s, %d invoice(s):\n", *host, len(res.Invoices))
	for _, inv := range res.Invoices {
		fmt.Printf("  %6d sats  state=%-8s settle_index=%d  hash=%x\n",
			inv.Value, inv.State, inv.SettleIndex, inv.RHash)
	}
}
