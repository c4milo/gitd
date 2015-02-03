// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Implements Git Smart HTTP backend using its C implementation as reference:
// https://github.com/git/git/blob/master/http-backend.c
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/c4milo/handlers/logger"
	"github.com/hashicorp/logutils"
	"github.com/stretchr/graceful"
)

// Version is injected in build time and defined in the Makefile
var Version string

// Name is injected in build time and defined in the Makefile
var Name string

type Config struct {
	Bind            string `toml:"bind"`
	Port            uint   `toml:"port"`
	ReposPath       string `toml:"repos_path"`
	LogLevel        string `toml:"log_level"`
	LogFilePath     string `toml:"log_file"`
	ShutdownTimeout string `toml:"shutdown_timeout"`
}

// Default configuration
var config Config = Config{
	Bind:            "localhost",
	Port:            12345,
	LogLevel:        "WARN",
	ShutdownTimeout: "15s",
}

// Configuration file path
var configFile string

func init() {
	if !checkGitVersion(2, 2, 1) {
		log.Fatalln("Git >= v2.2.1 is required")
	}

	reposPath, err := ioutil.TempDir(os.TempDir(), Name)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	config.ReposPath = reposPath

	flag.StringVar(&configFile, "f", "", "config file path")
	flag.Parse()

	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		log.Printf("[ERROR] %v", err)
		log.Print("[ERROR] Parsing config file, using default configuration")
	}
}

func Handler(w http.ResponseWriter, req *http.Request) {
	handlers := map[*regexp.Regexp]func(http.ResponseWriter, *http.Request, string){
		regexp.MustCompile("(.*?)/git-upload-pack$"):  UploadPack,
		regexp.MustCompile("(.*?)/git-receive-pack$"): ReceivePack,
		regexp.MustCompile("(.*?)/info/refs$"):        InfoRefs,
	}

	for re, handler := range handlers {
		if m := re.FindStringSubmatch(req.URL.Path); m != nil {
			repoPath := m[1]
			handler(w, req, repoPath)
			return
		}
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte("Bad Request"))
}

func main() {
	var logWriter io.Writer
	if config.LogFilePath != "" {
		var err error
		logWriter, err = os.OpenFile(config.LogFilePath, os.O_RDWR|os.O_APPEND, 0660)
		if err != nil {
			log.Printf("[WARN] %v", err)
		}
	}

	if logWriter == nil {
		logWriter = os.Stderr
	}

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "WARN", "ERROR"},
		MinLevel: logutils.LogLevel(config.LogLevel),
		Writer:   logWriter,
	}

	log.SetOutput(filter)

	mux := http.DefaultServeMux
	mux.HandleFunc("/", Handler)

	address := fmt.Sprintf("%s:%d", config.Bind, config.Port)
	timeout, err := time.ParseDuration(config.ShutdownTimeout)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	log.Printf("[INFO] Listening on %s...", address)
	log.Printf("[INFO] Serving Git repositories over HTTP from %s", config.ReposPath)
	graceful.Run(address, timeout, logger.Handler(mux, logger.AppName("gitd")))
}

// Runs git-upload-pack in a safe manner
func UploadPack(w http.ResponseWriter, req *http.Request, repoPath string) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
		return
	}
	process := "git-upload-pack"
	cwd := filepath.Join(config.ReposPath, repoPath)

	headers := w.Header()
	headers.Add("Content-Type", fmt.Sprintf("application/x-%s-result", process))
	w.WriteHeader(http.StatusOK)

	cmd := exec.Command(process, "--stateless-rpc", ".")
	cmd.Dir = cwd
	runCommand(w, req.Body, cmd)
}

//Runs git-receive-pack in a safe manner
func ReceivePack(w http.ResponseWriter, req *http.Request, repoPath string) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
		return
	}
	process := "git-receive-pack"
	cwd := filepath.Join(config.ReposPath, repoPath)

	headers := w.Header()
	headers.Add("Content-Type", fmt.Sprintf("application/x-%s-result", process))
	w.WriteHeader(http.StatusOK)

	cmd := exec.Command(process, "--stateless-rpc", ".")
	cmd.Dir = cwd
	runCommand(w, req.Body, cmd)
}

func InfoRefs(w http.ResponseWriter, req *http.Request, repoPath string) {
	if req.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
		return
	}

	process := req.URL.Query().Get("service")
	cwd := filepath.Join(config.ReposPath, repoPath)

	if process != "git-receive-pack" && process != "git-upload-pack" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
		return
	}

	headers := w.Header()
	headers.Add("Content-Type", fmt.Sprintf("application/x-%s-advertisement", process))
	w.WriteHeader(http.StatusOK)

	w.Write(packetWrite(fmt.Sprintf("# service=%s\n", process)))
	w.Write(packetFlush())

	cmd := exec.Command(process, "--stateless-rpc", "--advertise-refs", ".")
	cmd.Dir = cwd
	runCommand(w, req.Body, cmd)
}

// Executes a shell command and pipes its output to HTTP response writer.
// DO NOT expose this function directly to end users as it creates a security breach
func runCommand(w io.Writer, r io.Reader, cmd *exec.Cmd) {
	if cmd.Dir != "" {
		cmd.Dir = sanitize(cmd.Dir)
	}

	log.Printf("[DEBUG] Running command from %s: %s %s ", cmd.Dir, cmd.Path, cmd.Args)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("[ERROR] %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[ERROR] %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[ERROR] %v", err)
	}

	io.Copy(stdin, r)
	io.Copy(w, stdout)
	cmd.Wait()
}

// Returns bytes of a git packet containing the given string
func packetWrite(str string) []byte {
	s := strconv.FormatInt(int64((len(str) + 4)), 16)

	m := len(s) % 4
	if m != 0 {
		s = strings.Repeat("0", 4-m) + s
	}

	return []byte(s + str)
}

func packetFlush() []byte {
	return []byte("0000")
}

// Sanitizes name to avoid overwriting sensitive system files
// or executing forbidden binaries
func sanitize(name string) string {
	// Gets rid of volume drive label in Windows
	if len(name) > 1 && name[1] == ':' && runtime.GOOS == "windows" {
		name = name[2:]
	}

	name = filepath.Clean(name)
	name = filepath.ToSlash(name)
	for strings.HasPrefix(name, "../") {
		name = name[3:]
	}
	return name
}

func checkGitVersion(major, minor, patch int) bool {
	git, err := exec.LookPath("git")
	if err != nil {
		log.Printf("[ERROR] %v", err)
		return false
	}

	cmd := exec.Command(git, "--version")
	var stdout string
	if stdout, _, err = runAndLog(cmd); err != nil {
		log.Printf("[ERROR] %v", err)
		return false
	}

	output := strings.Split(stdout, "\n")
	if len(output) < 2 {
		log.Printf("[DEBUG] git version output: %v", output)
		return false
	}

	parts := strings.Split(output[0], " ")
	if len(parts) < 3 {
		log.Printf("[DEBUG] git version parts: %v", parts)
		return false
	}

	version := strings.Split(parts[2], ".")
	major2, _ := strconv.Atoi(version[0])
	minor2, _ := strconv.Atoi(version[1])
	patch2, _ := strconv.Atoi(version[2])

	if major2 < major || minor2 < minor || patch2 < patch {
		log.Printf("[INFO] git version not supported: %d.%d.%d", major2, minor2, patch2)
		return false
	}

	return true
}

// Borrowed from https://github.com/mitchellh/packer/blob/master/builder/vmware/common/driver.go
func runAndLog(cmd *exec.Cmd) (string, string, error) {
	var stdout, stderr bytes.Buffer

	log.Printf("[VMWare] Executing: %s %v", cmd.Path, cmd.Args[1:])
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	stdoutString := strings.TrimSpace(stdout.String())
	stderrString := strings.TrimSpace(stderr.String())

	if _, ok := err.(*exec.ExitError); ok {
		message := stderrString
		if message == "" {
			message = stdoutString
		}

		err = fmt.Errorf("[VMWare] error: %s", message)
	}

	log.Printf("stdout: %s", stdoutString)
	log.Printf("stderr: %s", stderrString)

	// Replace these for Windows, we only want to deal with Unix
	// style line endings.
	returnStdout := strings.Replace(stdout.String(), "\r\n", "\n", -1)
	returnStderr := strings.Replace(stderr.String(), "\r\n", "\n", -1)

	return returnStdout, returnStderr, err
}
