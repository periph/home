// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build ignored

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/maruel/natural"
)

func command(a ...string) *exec.Cmd {
	log.Printf("- %s", strings.Join(a, " "))
	return exec.Command(a[0], a[1:]...)
}

func cloneAndGetTag(d, url string) (string, error) {
	cmd := command("git", "clone", "--quiet", url)
	cmd.Dir = d
	if err := cmd.Run(); err != nil {
		return "", err
	}
	b := filepath.Join(d, "aioesphomeapi")

	// Get the latest tag.
	cmd = command("git", "tag")
	cmd.Dir = b
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	var tags natural.StringSlice
	for _, tag := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(tag, "v") {
			continue
		}
		tags = append(tags, tag)
	}
	if len(tags) == 0 {
		return "", errors.New("no git tag found")
	}
	sort.Sort(tags)
	tag := tags[len(tags)-1]
	log.Printf("using tag %s", tag)

	cmd = command("git", "checkout", "--quiet", tag)
	cmd.Dir = b
	return tag, cmd.Run()
}

func updateProtos(d string) error {
	const url = "https://github.com/esphome/aioesphomeapi"
	tag, err := cloneAndGetTag(d, url)
	if err != nil {
		return err
	}
	base := filepath.Join(d, "aioesphomeapi")
	for _, n := range []string{"api.proto", "api_options.proto"} {
		src := filepath.Join(base, "aioesphomeapi", n)
		if err := copyFile(n, src); err != nil {
			return err
		}
	}
	if err := copyFile("LICENSE", filepath.Join(base, "LICENSE")); err != nil {
		return err
	}

	if command("protoc-gen-go", "--version").Run() != nil {
		if err = command("go", "install", "google.golang.org/protobuf/cmd/protoc-gen-go@latest").Run(); err != nil {
			// Currently v1.26.0
			return errors.New("protoc-gen-go is needed. Run: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest\n")
		}
	}
	args := []string{
		"protoc",
		"--go_out=.",
		"--go_opt=paths=source_relative",
		"--go_opt=Mapi.proto=periph.io/x/home/thirdparty/aioesphomeapi",
		"--go_opt=Mapi_options.proto=periph.io/x/home/thirdparty/aioesphomeapi",
		"api.proto", "api_options.proto",
	}
	if err := command(args...).Run(); err != nil {
		return err
	}
	buf := fmt.Sprintf("URL: %s\nLICENSE: MIT\nVersion: %s\n", url, tag)
	return ioutil.WriteFile("README.md", []byte(buf), 0o644)
}

func copyFile(dst, src string) error {
	b, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dst, b, 0o644)
}

func mainImpl() error {
	if err := os.Chdir("aioesphomeapi"); err != nil {
		return err
	}
	// https://github.com/esphome/aioesphomeapi is under MIT, so it's safe to
	// fetch.
	d, err := ioutil.TempDir("", "esphome")
	if err != nil {
		return err
	}
	err = updateProtos(d)
	if err2 := os.RemoveAll(d); err == nil {
		err = err2
	}
	return err
}

func main() {
	if err := mainImpl(); err != nil {
		fmt.Fprintf(os.Stderr, "gen: %s\n", err)
		os.Exit(1)
	}
}
