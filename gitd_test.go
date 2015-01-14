package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	log "github.com/Sirupsen/logrus"
	"github.com/c4milo/handlers/logger"
)

// assert fails the test if the condition is false.
func assert(tb testing.TB, condition bool, msg string, v ...interface{}) {
	if !condition {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: "+msg+"\033[39m\n\n", append([]interface{}{filepath.Base(file), line}, v...)...)
		tb.FailNow()
	}
}

// ok fails the test if an err is not nil.
func ok(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// equals fails the test if exp is not equal to act.
func equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d:\n\n\texp: %#v\n\n\tgot: %#v\033[39m\n\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestGitD(t *testing.T) {
	// Start service
	gitdHandler := http.HandlerFunc(GitDHTTPHandler)
	handler := logger.Handler(gitdHandler, logger.Output(ioutil.Discard))
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Create bare repository
	repoName := "test"
	cmd := exec.Command("git", "--bare", "init", repoName+".git")
	cmd.Dir = config.ReposPath
	err := cmd.Run()
	ok(t, err)

	workspace, err := ioutil.TempDir(os.TempDir(), Name+"-clones")
	ok(t, err)

	// Clone bare repo
	cmd = exec.Command("git", "clone", ts.URL+"/"+repoName+".git")
	cmd.Dir = workspace
	err = cmd.Run()
	ok(t, err)

	repoPath := filepath.Join(workspace, repoName)

	// Add one file, commit and push it
	f, err := os.Create(filepath.Join(repoPath, "README.md"))
	ok(t, err)
	f.WriteString("blah")
	f.Close()

	// git add the file
	cmd = exec.Command("git", "add", "--all")
	cmd.Dir = repoPath
	err = cmd.Run()
	ok(t, err)

	// commit file
	cmd = exec.Command("git", "commit", "-m", `'testing gitd'`)
	cmd.Dir = repoPath
	err = cmd.Run()
	fmt.Printf("%s\n", cmd.Dir)
	ok(t, err)

	// git push the file
	cmd = exec.Command("git", "push")
	cmd.Dir = repoPath
	err = cmd.Run()
	ok(t, err)

	// Clone bare repo again in a different location
	repo2Name := "test2"
	repo2Path := filepath.Join(workspace, repo2Name)

	cmd = exec.Command("git", "clone", ts.URL+"/"+repoName+".git", repo2Path)
	err = cmd.Run()
	ok(t, err)

	// Verify that previously pushed file exists
	data, err := ioutil.ReadFile(filepath.Join(repo2Path, "README.md"))
	ok(t, err)
	equals(t, data, []byte("blah"))
}
