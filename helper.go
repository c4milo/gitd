package main

import (
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
)

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
