// Package tunnel runs WatTunnel ends for the panel: one managed wattun child process per end,
// its config generated from the stored tunnel, restarted when it dies, and polled over the
// daemon's loopback metrics endpoint. The transport itself lives in internal/wattunnel.
package tunnel
