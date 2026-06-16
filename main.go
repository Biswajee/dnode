package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/biswajee/dnode/display"
	"github.com/biswajee/dnode/logger"
	"github.com/biswajee/dnode/monitor"
)

var version = "dev"

func main() {
	monitorMode := flag.Bool("monitor", false, "Render a live device tree on the terminal")
	v   := flag.Bool("v",   false, "Verbosity level 1: ID, name, port")
	vvv := flag.Bool("vvv", false, "Verbosity level 3: + USB endpoints")
	flag.Bool("vv", false, "Verbosity level 2: + manufacturer, product, serial (default)")
	logPath := flag.String("log", "dnode.log", "Path to log file")
	flag.Usage = usage
	flag.Parse()

	verbosity := 2
	switch {
	case *vvv:
		verbosity = 3
	case *v:
		verbosity = 1
	}

	log, err := logger.New(*logPath, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dnode: open log: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	var tree *display.Tree
	if *monitorMode {
		tree = display.New(verbosity)
	}

	mon := monitor.New()
	events := make(chan monitor.DeviceEvent, 64)

	go func() {
		for evt := range events {
			log.Log(evt)
			if tree != nil {
				tree.Update(evt)
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		if tree == nil {
			fmt.Fprintln(os.Stderr, "dnode: shutting down")
		}
		os.Exit(0)
	}()

	if err := mon.Run(events); err != nil {
		fmt.Fprintf(os.Stderr, "dnode: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `dnode - device node viewer

Usage:
  dnode [flags]

Flags:
  -monitor        Render a live device tree on the terminal
  -v              Minimal: ID, name, port
  -vv             Standard: + manufacturer, product, serial (default)
  -vvv            Verbose: + USB endpoints
  -log <path>     Log file path (default: dnode.log)
  -help           Show this message

`)
}
