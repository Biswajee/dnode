package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/biswajee/rose/display"
	"github.com/biswajee/rose/logger"
	"github.com/biswajee/rose/monitor"
	"github.com/biswajee/rose/notify"
)

func main() {
	monitorMode := flag.Bool("monitor", false, "Render a live device tree on the terminal")
	logPath := flag.String("log", "rose.log", "Path to log file")
	noNotify := flag.Bool("no-notify", false, "Disable desktop notifications")
	flag.Usage = usage
	flag.Parse()

	log, err := logger.New(*logPath, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rose: open log: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	var tree *display.Tree
	if *monitorMode {
		tree = display.New()
	}

	var notifier notify.Notifier
	if !*noNotify {
		notifier = &dedupNotifier{inner: notify.New(), seen: make(map[string]time.Time)}
	}

	mon := monitor.New()
	events := make(chan monitor.DeviceEvent, 64)

	go func() {
		for evt := range events {
			log.Log(evt)
			if tree != nil {
				tree.Update(evt)
			}
			if notifier != nil {
				if err := notifier.Send(evt); err != nil {
					fmt.Fprintf(os.Stderr, "rose: notification: %v\n", err)
				}
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		if tree == nil {
			fmt.Fprintln(os.Stderr, "rose: shutting down")
		}
		os.Exit(0)
	}()

	if err := mon.Run(events); err != nil {
		fmt.Fprintf(os.Stderr, "rose: %v\n", err)
		os.Exit(1)
	}
}

// dedupNotifier suppresses duplicate desktop notifications for the same
// device within a short window. This prevents multiple notifications when
// Windows fires WM_DEVICECHANGE once per interface class for a single device.
// Only events whose DeviceID was successfully parsed as "VID:XXXX PID:XXXX"
// trigger a notification; raw bus paths (FTDIBUS, etc.) are ignored.
type dedupNotifier struct {
	inner notify.Notifier
	mu    sync.Mutex
	seen  map[string]time.Time
}

func (d *dedupNotifier) Send(evt monitor.DeviceEvent) error {
	// Only notify for properly parsed VID:PID devices.
	if !strings.HasPrefix(evt.DeviceID, "VID:") {
		return nil
	}
	key := string(evt.Action) + "|" + evt.DeviceID
	d.mu.Lock()
	if t, ok := d.seen[key]; ok && time.Since(t) < 3*time.Second {
		d.mu.Unlock()
		return nil
	}
	d.seen[key] = time.Now()
	d.mu.Unlock()
	return d.inner.Send(evt)
}

func usage() {
	fmt.Fprint(os.Stderr, `rose — device attachment monitor

Usage:
  rose [flags]

Flags:
  -monitor        Render a live device tree on the terminal
  -log <path>     Log file path (default: rose.log)
  -no-notify      Disable desktop notifications
  -help           Show this message

`)
}
