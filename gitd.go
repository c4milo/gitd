// Implements Git Smart HTTP backend using
// https://github.com/git/git/blob/master/http-backend.c as reference implementation
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
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
	log "github.com/Sirupsen/logrus"
	"github.com/c4milo/handlers/logger"
	"github.com/stretchr/graceful"
)

// Version is injected in build time and defined in the Makefile
var Version string

// Name is injected in build time and defined in the Makefile
var Name string

type Config struct {
	Bind            string    `toml:"bind"`
	Port            uint      `toml:"port"`
	ReposPath       string    `toml:"repos_path"`
	LogLevel        log.Level `toml:"log_level"`
	ShutdownTimeout string    `toml:"shutdown_timeout"`
}

// Default configuration
var config Config = Config{
	Bind:            "localhost",
	Port:            12345,
	LogLevel:        log.InfoLevel,
	ShutdownTimeout: "15s",
}

// Configuration file path
var configFile string

func init() {
	if !checkGitVersion(2, 2, 1) {
		log.Panicln("Git >= v2.2.1 is required")
	}

	reposPath, err := ioutil.TempDir(os.TempDir(), Name)
	if err != nil {
		log.Fatalf("%v", err)
	}

	config.ReposPath = reposPath

	flag.StringVar(&configFile, "f", "/etc/gitd.conf", "config file path")
	flag.Parse()

	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		log.Errorf("%v", err)
		log.Warn("Error parsing config file, using default configuration")
	}
}

func main() {
	mux := http.DefaultServeMux

	// request handlers
	handlers := map[*regexp.Regexp]func(http.ResponseWriter, *http.Request, string){
		regexp.MustCompile("(.*?)/git-upload-pack$"):  UploadPack,
		regexp.MustCompile("(.*?)/git-receive-pack$"): ReceivePack,
		regexp.MustCompile("(.*?)/info/refs$"):        InfoRefs,
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		for re, handler := range handlers {
			if m := re.FindStringSubmatch(req.URL.Path); m != nil {
				repoPath := m[1]
				handler(w, req, repoPath)
				return
			}
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	})

	address := fmt.Sprintf("%s:%d", config.Bind, config.Port)
	timeout, err := time.ParseDuration(config.ShutdownTimeout)
	if err != nil {
		log.Fatalf("%v", err)
	}

	log.Printf("Listening on %s...", address)
	log.Printf("Serving Git repositories over HTTP from %s", config.ReposPath)
	graceful.Run(address, timeout, logger.Handler(mux, logger.AppName("gitd")))
}

// Runs git-upload-pack in a safe manner
func UploadPack(w http.ResponseWriter, req *http.Request, repoPath string) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
		return
	}
	cmd := "git-upload-pack"
	cwd := filepath.Join(config.ReposPath, repoPath)

	headers := w.Header()
	headers.Add("Content-Type", fmt.Sprintf("application/x-%s-result", cmd))
	w.WriteHeader(http.StatusOK)

	args := []string{"--stateless-rpc", "."}
	runCommand(cwd, cmd, args, w, req.Body)
}

//Runs git-receive-pack in a safe manner
func ReceivePack(w http.ResponseWriter, req *http.Request, repoPath string) {
	if req.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
		return
	}
	cmd := "git-receive-pack"
	cwd := filepath.Join(config.ReposPath, repoPath)

	headers := w.Header()
	headers.Add("Content-Type", fmt.Sprintf("application/x-%s-result", cmd))
	w.WriteHeader(http.StatusOK)

	args := []string{"--stateless-rpc", "."}
	runCommand(cwd, cmd, args, w, req.Body)
}

func InfoRefs(w http.ResponseWriter, req *http.Request, repoPath string) {
	if req.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
		return
	}

	cmd := req.URL.Query().Get("service")
	cwd := filepath.Join(config.ReposPath, repoPath)

	if cmd != "git-receive-pack" && cmd != "git-upload-pack" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
		return
	}

	headers := w.Header()
	headers.Add("Content-Type", fmt.Sprintf("application/x-%s-advertisement", cmd))
	w.WriteHeader(http.StatusOK)

	w.Write(packetWrite(fmt.Sprintf("# service=%s\n", cmd)))
	w.Write(packetFlush())

	args := []string{"--stateless-rpc", "--advertise-refs", "."}
	runCommand(cwd, cmd, args, w, req.Body)
}

// Executes a shell command and pipes its output to HTTP response writer.
// DO NOT expose this function directly to end users as it creates a security breach
func runCommand(cwd, command string, args []string, w io.Writer, r io.Reader) {
	log.WithFields(log.Fields{
		"command": command,
		"args":    args,
	}).Debug("Running command")

	cmd := exec.Command(command, args...)
	cmd.Dir = sanitize(cwd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Error(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error(err)
	}

	if err := cmd.Start(); err != nil {
		log.Error(err)
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

func checkGitVersion(major, minor, patch uint) bool {
	//TODO
	return true
}
