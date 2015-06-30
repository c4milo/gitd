// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gitd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/c4milo/handlers/logger"
	"github.com/hooklift/assert"
)

func TestGitD(t *testing.T) {
	// Creates test repos path
	rpath, err := ioutil.TempDir(os.TempDir(), "gitd")
	assert.Ok(t, err)

	// Start service
	gitdHandler := Handler(http.DefaultServeMux, ReposPath(rpath))
	handler := logger.Handler(gitdHandler, logger.Output(ioutil.Discard))
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Create bare repository
	repoName := "test"
	cmd := exec.Command("git", "--bare", "init", repoName+".git")
	cmd.Dir = rpath
	err = cmd.Run()
	assert.Ok(t, err)

	workspace, err := ioutil.TempDir(os.TempDir(), "gitd-clones")
	assert.Ok(t, err)

	// Clone bare repo
	cmd = exec.Command("git", "clone", ts.URL+"/"+repoName+".git")
	cmd.Dir = workspace
	err = cmd.Run()
	assert.Ok(t, err)

	repoPath := filepath.Join(workspace, repoName)

	// Add one file, commit and push it
	f, err := os.Create(filepath.Join(repoPath, "README.md"))
	assert.Ok(t, err)
	f.WriteString("blah")
	f.Close()

	// git add the file
	cmd = exec.Command("git", "add", "--all")
	cmd.Dir = repoPath
	err = cmd.Run()
	assert.Ok(t, err)

	// commit file
	cmd = exec.Command("git", "commit", "-m", `'testing gitd'`)
	cmd.Dir = repoPath
	err = cmd.Run()
	fmt.Printf("%s\n", cmd.Dir)
	assert.Ok(t, err)

	// git push the file
	cmd = exec.Command("git", "push")
	cmd.Dir = repoPath
	err = cmd.Run()
	assert.Ok(t, err)

	// Clone bare repo again in a different location
	repo2Name := "test2"
	repo2Path := filepath.Join(workspace, repo2Name)

	cmd = exec.Command("git", "clone", ts.URL+"/"+repoName+".git", repo2Path)
	err = cmd.Run()
	assert.Ok(t, err)

	// Verify that previously pushed file exists
	data, err := ioutil.ReadFile(filepath.Join(repo2Path, "README.md"))
	assert.Ok(t, err)
	assert.Equals(t, data, []byte("blah"))
}
