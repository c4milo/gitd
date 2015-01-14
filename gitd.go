// Implements Git Smart HTTP backend using
// https://github.com/git/git/blob/master/http-backend.c as reference implementation
package main

import (
	"fmt"
	"net/http"
	"path/filepath"

	"regexp"

	log "github.com/Sirupsen/logrus"
	"github.com/goji/glogrus"
	"github.com/zenazn/goji"
	"github.com/zenazn/goji/web"
	"github.com/zenazn/goji/web/middleware"
)

// Version is injected in build time
var Version string

// Default repo dir
var repoDir string

func init() {
	log.SetLevel(log.WarnLevel)
	repoDir = "/tmp"

	if !checkGitVersion(2, 2, 1) {
		log.Panicln("Git >= v2.2.1 is required")
	}
}

func main() {
	goji.Abandon(middleware.Logger)
	goji.Use(glogrus.NewGlogrus(log.New(), "git-service"))
	goji.Use(middleware.NoCache)

	goji.Post(regexp.MustCompile("(?P<path>.*?)/git-upload-pack$"), UploadPack)
	goji.Post(regexp.MustCompile("(?P<path>.*?)/git-receive-pack$"), ReceivePack)

	goji.Get(regexp.MustCompile("(?P<path>.*?)/info/refs$"), InfoRefs)

	log.Info("Git Smart HTTP Service started")
	goji.Serve()
}

// Runs git-upload-pack in a safe manner
func UploadPack(c web.C, w http.ResponseWriter, req *http.Request) {
	cmd := "git-upload-pack"
	cwd := filepath.Join(repoDir, c.URLParams["path"])

	headers := w.Header()
	headers.Add("Content-Type", fmt.Sprintf("application/x-%s-result", cmd))
	w.WriteHeader(http.StatusOK)

	args := []string{"--stateless-rpc", "."}
	runCommand(cwd, cmd, args, w, req.Body)
}

//Runs git-receive-pack in a safe manner
func ReceivePack(c web.C, w http.ResponseWriter, req *http.Request) {
	cmd := "git-receive-pack"
	cwd := filepath.Join(repoDir, c.URLParams["path"])

	headers := w.Header()
	headers.Add("Content-Type", fmt.Sprintf("application/x-%s-result", cmd))
	w.WriteHeader(http.StatusOK)

	args := []string{"--stateless-rpc", "."}
	runCommand(cwd, cmd, args, w, req.Body)
}

func InfoRefs(c web.C, w http.ResponseWriter, req *http.Request) {
	cmd := req.URL.Query().Get("service")
	cwd := filepath.Join(repoDir, c.URLParams["path"])

	log.WithFields(log.Fields{
		"command": cmd,
		"cwd":     cwd,
		"file":    c.URLParams["file"],
	}).Debug("InfoRefs")

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
