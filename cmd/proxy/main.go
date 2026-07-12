// Command proxy runs the lab's trace tap. It is a thin wrapper: all the logic
// lives in pkg/proxy so it can be embedded and tested. Configuration comes from
// the environment (UPSTREAM, ADDR, TRACE_DIR).
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/tamnd/tomo-labs/pkg/proxy"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := proxy.Run(ctx, proxy.Options{}); err != nil {
		log.Fatal(err)
	}
}
