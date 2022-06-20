// Copyright 2021 The Periph Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

//go:build ignored
// +build ignored

package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/maruel/natural"
)

const (
	// https://github.com/esphome/aioesphomeapi/tags
	aioesphomeapiVer = "10.10.0"
	// https://github.com/protocolbuffers/protobuf/tags
	protocVer = "3.20.1"
	// https://github.com/protocolbuffers/protobuf-go/tags
	protocGenGoVer = "1.28.0"
)

func command(a ...string) *exec.Cmd {
	log.Printf("- %s", strings.Join(a, " "))
	return exec.Command(a[0], a[1:]...)
}

func run(dir string, a ...string) (string, error) {
	cmd := command(a...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runWrapped(dir string, a ...string) error {
	out, err := run(dir, a...)
	if err != nil {
		return fmt.Errorf("%s failed:\n%s\n%w", strings.Join(a, " "), out, err)
	}
	return nil
}

// gitGetLatestTag gets the latest tag of a git repository.
func gitGetLatestTag(dir string) (string, error) {
	out, err := run(dir, "git", "tag")
	if err != nil {
		return "", fmt.Errorf("git tag failed:\n%s\n%w", out, err)
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
	return tags[len(tags)-1], nil
}

func getProtocURL(ver string) (string, error) {
	suffix := ""
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "386":
			suffix = "linux-x86_32"
		case "amd64":
			suffix = "linux-x86_64"
		case "arm64":
			suffix = "linux-aarch_64"
		default:
			return "", fmt.Errorf("implement for %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			suffix = "osx-aarch_64"
		case "amd64":
			suffix = "osx-x86_64"
		default:
			return "", fmt.Errorf("implement for %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	case "windows":
		// We don't care if it's windows 64.
		switch runtime.GOARCH {
		case "386":
			suffix = "win32"
		case "amd64":
			suffix = "win64"
		default:
			return "", fmt.Errorf("implement for %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	default:
		return "", fmt.Errorf("implement for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return "https://github.com/protocolbuffers/protobuf/releases/download/v" + ver + "/protoc-" + ver + "-" + suffix + ".zip", nil
}

func extractOne(dst string, file *zip.File) error {
	log.Printf("extractOne(%s, %s)", dst, file.Name)
	// Cheezy path traversal safety check.
	if strings.Contains(file.Name, "..") {
		return errors.New("invalid path")
	}
	name := filepath.Clean(file.Name)
	dst = filepath.Join(dst, name)
	if b := filepath.Dir(dst); b != "" {
		if _, err := os.Stat(b); os.IsNotExist(err) {
			if err = os.Mkdir(b, 0o700); err != nil {
				return err
			}
		}
	}
	if file.UncompressedSize64 == 0 {
		return nil
	}
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, rc)
	return err
}

// getProtoc downloads an hermetic version of the protocolbuffers compiler
// protoc.
func getProtoc(dst, ver string) error {
	url, err := getProtocURL(ver)
	if err != nil {
		return err
	}
	// It's only a few MiB in size.
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		if err := extractOne(dst, f); err != nil {
			return err
		}
	}
	return nil
}

// updateProtos update protos.
//
// It downloads the latest aioesphomeapi proto.
// It downloads protoc and protoc-gen-go compilers.
// It runs the compiler.
func updateProtos(dst string) error {
	// Download the latest aioesphomeapi proto.
	const gitURL = "https://github.com/esphome/aioesphomeapi"
	const tag = "v" + aioesphomeapiVer
	if err := runWrapped(dst, "git", "clone", "--quiet", "--branch", tag, gitURL); err != nil {
		return err
	}
	base := filepath.Join(dst, "aioesphomeapi")
	t, err := gitGetLatestTag(base)
	if err != nil {
		return err
	}
	if t != tag {
		log.Printf("Warning: using aioesphomeapi at %s but %s is more recent", tag, t)
	}
	for _, n := range []string{"api.proto", "api_options.proto"} {
		src := filepath.Join(base, "aioesphomeapi", n)
		if err := copyFile(n, src); err != nil {
			return err
		}
	}
	if err := copyFile("LICENSE", filepath.Join(base, "LICENSE")); err != nil {
		return err
	}

	// Download protoc and protoc-gen-go compilers.
	bin := filepath.Join(dst, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		return err
	}
	os.Setenv("GOBIN", bin)
	os.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := getProtoc(dst, protocVer); err != nil {
		return err
	}
	if err := runWrapped(dst, "go", "install", "google.golang.org/protobuf/cmd/protoc-gen-go@v"+protocGenGoVer); err != nil {
		return err
	}

	// Run the compiler.
	args := []string{
		filepath.Join(dst, "bin", "protoc"),
		"--go_out=.",
		"--go_opt=paths=source_relative",
		"--go_opt=Mapi.proto=periph.io/x/home/thirdparty/aioesphomeapi",
		"--go_opt=Mapi_options.proto=periph.io/x/home/thirdparty/aioesphomeapi",
		"api.proto", "api_options.proto",
	}
	if err := runWrapped(".", args...); err != nil {
		return err
	}
	buf := fmt.Sprintf("URL: %s\nLICENSE: MIT\nVersion: %s\n", gitURL, tag)
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
