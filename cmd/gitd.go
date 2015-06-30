// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/c4milo/gitd"
	"github.com/c4milo/handlers/logger"
	"github.com/hashicorp/logutils"
	"gopkg.in/tylerb/graceful.v1"
)

// Version is injected in build time and defined in the Makefile
var Version string

// Name is injected in build time and defined in the Makefile
var Name string

// Config defines the configurable options for this service.
type Config struct {
	Bind            string `toml:"bind"`
	Port            uint   `toml:"port"`
	ReposPath       string `toml:"repos_path"`
	LogLevel        string `toml:"log_level"`
	LogFilePath     string `toml:"log_file"`
	ShutdownTimeout string `toml:"shutdown_timeout"`
}

// Default configuration
var config = Config{
	Bind:            "localhost",
	Port:            12345,
	LogLevel:        "WARN",
	ShutdownTimeout: "15s",
}

// Configuration file path
var configFile string

func init() {
	reposPath, err := ioutil.TempDir(os.TempDir(), Name)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	config.ReposPath = reposPath

	flag.StringVar(&configFile, "f", "", "config file path")
	flag.Parse()

	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		log.Printf("[ERROR] %v", err)
		log.Print("[ERROR] Parsing config file, using default configuration.")
	}
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
	rack := gitd.Handler(mux, gitd.ReposPath(config.ReposPath))
	rack = logger.Handler(rack, logger.AppName(Name))

	address := fmt.Sprintf("%s:%d", config.Bind, config.Port)
	timeout, err := time.ParseDuration(config.ShutdownTimeout)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	log.Printf("[INFO] Listening on %s...", address)
	log.Printf("[INFO] Serving Git repositories over HTTP from %s", config.ReposPath)

	graceful.Run(address, timeout, rack)
}
