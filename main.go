// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yzzyx/imap-sync/config"
	"github.com/yzzyx/imap-sync/imap"
	"github.com/yzzyx/imap-sync/literal"
	"github.com/yzzyx/imap-sync/mail"
	"github.com/yzzyx/imap-sync/maildir"
	"gopkg.in/yaml.v2"
)

func userHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	return os.Getenv("HOME")
}

func parsePathSetting(inPath string) string {
	if strings.HasPrefix(inPath, "$HOME") {
		inPath = userHomeDir() + inPath[5:]
	} else if strings.HasPrefix(inPath, "~/") {
		inPath = userHomeDir() + inPath[1:]
	}

	if strings.HasPrefix(inPath, "$") {
		end := strings.Index(inPath, string(os.PathSeparator))
		inPath = os.Getenv(inPath[1:end]) + inPath[end:]
	}
	if filepath.IsAbs(inPath) {
		return filepath.Clean(inPath)
	}

	p, err := filepath.Abs(inPath)
	if err == nil {
		return filepath.Clean(p)
	}
	return ""
}

func main() {
	ctx := context.Background()

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		cfgDir = filepath.Join(userHomeDir(), ".config")
	}
	configPath := filepath.Join(cfgDir, "imap-sync", "config.yml")

	configFile := flag.String("config", configPath, "Use specific configuration file")
	flag.Parse()

	cfgData, err := ioutil.ReadFile(*configFile)
	if err != nil {
		fmt.Printf("Cannot read config file '%s': %s\n", configPath, err)
		os.Exit(1)
	}

	cfg := config.Config{}
	err = yaml.Unmarshal(cfgData, &cfg)
	if err != nil {
		fmt.Printf("Cannot parse config file '%s': %s\n", configPath, err)
		os.Exit(1)
	}

	// Create a IMAP setup for each mailbox
	for name, mailbox := range cfg.Mailboxes {
		if mailbox.Maildir == "" {
			log.Printf("maildir not set for mailbox %s, skipping", name)
			continue
		}
		maildirPath := parsePathSetting(mailbox.Maildir)

		// Create maildir if it doesnt exist
		err = os.MkdirAll(maildirPath, 0700)
		if err != nil {
			panic(err)
		}

		md, err := maildir.New(maildirPath)
		if err != nil {
			log.Printf("cannot create new maildir instance: %v\n", err)
			return
		}

		imapHandler, err := imap.New(mailbox)
		if err != nil {
			log.Printf("cannot initalize new imap connection: %v\n", err)
			return
		}

		ch := make(chan mail.Info, 100)
		go func() {
			err = md.Scan(ctx, ch)
			if err != nil {
				log.Printf("cannot scan maildir: %v\n", err)
				return
			}
			close(ch)
		}()

		// Upload any new files in our mail dirs to the server,
		// and then rename the messages to match our UID's
		for m := range ch {
			fd, err := os.Open(m.Filename)
			if err != nil {
				log.Printf("could not open file %s: %v", m.Filename, err)
				return
			}
			info, err := imapHandler.AddMessage(m, &literal.FileLiteral{fd})
			if err != nil {
				log.Printf("could not upload message: %v", err)
				return
			}
			fd.Close()
			info, err = md.RenameMessage(info)
			if err != nil {
				log.Printf("could not rename message: %v", err)
				return
			}
			fmt.Printf(" upload %+v\n", m)
		}

		err = imapHandler.CheckMessages(ctx, md)
		if err != nil {
			log.Printf("cannot check for new messages on server: %v\n", err)
			return
		}

		err = imapHandler.Close()
		if err != nil {
			log.Printf("Cannot close imap handler: %v", err)
			return
		}
	}

	return
}
