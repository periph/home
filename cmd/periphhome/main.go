// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// periphhome is a node implementation of the ESPHome protocol.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/fsnotify/fsnotify"
	"periph.io/x/home/node/config"
	"periph.io/x/host/v3"
)

// autoCancellingContext returns a global context that is canceled if SIGTERM /
// SIGINT is received or if the executable file is modified.
func autoCancellingContext(cfg string) (context.Context, func(), error) {
	// Cancel on SIGTERM / SIGINT.
	ctx, cancel := context.WithCancel(context.Background())
	chanSignal := make(chan os.Signal, 1)
	go func() {
		<-chanSignal
		cancel()
	}()
	signal.Notify(chanSignal, os.Interrupt)

	exe, err := os.Executable()
	if err != nil {
		return ctx, cancel, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return ctx, cancel, err
	}

	lookup := map[string]time.Time{}
	for _, n := range []string{exe, cfg} {
		var fi os.FileInfo
		fi, err = os.Stat(n)
		if err != nil {
			_ = watcher.Close()
			return ctx, cancel, err
		}
		if err = watcher.Add(n); err != nil {
			_ = watcher.Close()
			return ctx, cancel, err
		}
		mod := fi.ModTime()
		lookup[n] = mod
		log.Printf("watching: %s @ %s", n, mod)
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case <-watcher.Errors:
				log.Printf("got error while watching for file changes, exiting. %s", err)
				cancel()
				return
			case e := <-watcher.Events:
				log.Printf("got file event %s", e.Name)
				if fi2, err2 := os.Stat(e.Name); err2 != nil {
					log.Printf("file %s doesn't exist anymore, ignoring", e.Name)
				} else if mod := fi2.ModTime(); !mod.Equal(lookup[e.Name]) {
					log.Printf("file %s was modified, exiting.", e.Name)
					cancel()
					return
				} else {
					log.Printf("file %s not modified", e.Name)
				}
			}
		}
	}()
	return ctx, cancel, nil
}

func mainImpl() error {
	// Make sure periph can be initialized, otherwise there isn't much to do.
	if _, err := host.Init(); err != nil {
		return err
	}

	// Flag handling.
	flag.Usage = func() {
		o := flag.CommandLine.Output()
		fmt.Fprintf(o, "usage: %s <config.yaml> <command>\n", os.Args[0])
		fmt.Fprintf(o, "\nCommands are:\n")
		fmt.Fprintf(o, "  install  Install the node to run on boot\n")
		fmt.Fprintf(o, "  run      Run the node\n")
		fmt.Fprintf(o, "\n")
		flag.PrintDefaults()
	}
	cpuprofile := flag.String("cpuprofile", "", "dump CPU profile in file")
	flag.Parse()
	if flag.NArg() != 2 {
		return errors.New("expect 2 arguments. Use -help for more information")
	}
	configFile := flag.Arg(0)
	cmd := flag.Arg(1)

	if *cpuprofile != "" {
		// Run with cpuprofile, then use 'go tool pprof' to analyze it. See
		// http://blog.golang.org/profiling-go-programs for more details.
		f, err := os.Create(*cpuprofile)
		if err != nil {
			return err
		}
		if err = pprof.StartCPUProfile(f); err != nil {
			defer pprof.StopCPUProfile()
		}
	}

	// Change configFile to absolute path right away to simplify our life later
	// on.
	configFile, err := filepath.Abs(configFile)
	if err != nil {
		return err
	}

	ctx, cancel, err := autoCancellingContext(configFile)
	defer cancel()
	if err != nil {
		return err
	}

	/* #nosec G304 */
	b, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	// Load config then run the node.
	cfg := config.Root{}
	if err := cfg.LoadYaml(b); err != nil {
		return err
	}

	switch cmd {
	case "install":
		return install(configFile)
	case "run":
		return run(ctx, &cfg)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "periphhome: %s.\n", err)
		os.Exit(1)
	}
}
