// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package imap

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/emersion/go-imap"
	uidplus "github.com/emersion/go-imap-uidplus"
	"github.com/emersion/go-imap/client"
	"github.com/yzzyx/imap-sync/config"
	"github.com/yzzyx/imap-sync/maildir"
)

// IndexUpdate is used to signal that a message should be tagged with specific information
type IndexUpdate struct {
	Path      string   // Path to file to be updated
	MessageID string   // MessageID to be updated
	Tags      []string // Tags to add/remove from message (entries prefixed with "-" will be removed)
}

type Client struct {
	*client.Client
	*uidplus.UidPlusClient
}

// Handler is responsible for reading from mailboxes and updating the notmuch index
// Note that a single handler can only read from one mailbox
type Handler struct {
	mailbox config.Mailbox
	client  *Client
}

// New creates a new Handler for processing IMAP mailboxes
func New(mailbox config.Mailbox) (*Handler, error) {
	var err error
	h := Handler{}

	h.mailbox = mailbox

	if h.mailbox.PasswordCmd != "" {
		cmd := exec.Command("sh", "-c", h.mailbox.PasswordCmd)
		out := &bytes.Buffer{}
		cmd.Stdout = out
		err = cmd.Run()
		if err != nil {
			return nil, err
		}
		h.mailbox.Password = strings.TrimRight(out.String(), "\n\r")
	}

	if h.mailbox.Server == "" {
		return nil, errors.New("imap server address not configured")
	}
	if h.mailbox.Username == "" {
		return nil, errors.New("imap username not configured")
	}
	if h.mailbox.Password == "" {
		return nil, errors.New("imap password not configured")
	}

	// Set default port
	if h.mailbox.Port == 0 {
		h.mailbox.Port = 143
		if h.mailbox.UseTLS {
			h.mailbox.Port = 993
		}
	}

	connectionString := fmt.Sprintf("%s:%d", h.mailbox.Server, h.mailbox.Port)
	tlsConfig := &tls.Config{ServerName: h.mailbox.Server}
	var c *client.Client
	if h.mailbox.UseTLS {
		c, err = client.DialTLS(connectionString, tlsConfig)
	} else {
		c, err = client.Dial(connectionString)
	}

	if err != nil {
		return nil, err
	}

	h.client = &Client{
		c,
		uidplus.NewClient(c),
	}

	// Start a TLS session
	if h.mailbox.UseStartTLS {
		if err = h.client.StartTLS(tlsConfig); err != nil {
			return nil, err
		}
	}

	err = h.client.Login(h.mailbox.Username, h.mailbox.Password)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// Close closes all open handles, flushes channels and saves configuration data
func (h *Handler) Close() error {
	err := h.client.Close()
	if err != nil {
		return err
	}

	err = h.client.Logout()
	return err
}

func (h *Handler) listFolders() ([]string, error) {
	includeAll := false
	// If no specific folders are listed to be included, assume all folders should be included
	if len(h.mailbox.Folders.Include) == 0 {
		includeAll = true
	}

	// Make a map of included and excluded mailboxes
	includedFolders := make(map[string]bool)
	for _, folder := range h.mailbox.Folders.Include {
		// Note - we set this to false to keep track of if it exists on the server or not
		includedFolders[folder] = false
	}

	excludedFolders := make(map[string]bool)
	for _, folder := range h.mailbox.Folders.Exclude {
		excludedFolders[folder] = true
	}

	mboxChan := make(chan *imap.MailboxInfo, 10)
	errChan := make(chan error, 1)
	go func() {
		if err := h.client.List("", "*", mboxChan); err != nil {
			errChan <- err
		}
	}()

	var folderNames []string
	for mb := range mboxChan {
		if mb == nil {
			// We're done
			break
		}

		// Check if this mailbox should be excluded
		if _, ok := excludedFolders[mb.Name]; ok {
			continue
		}

		if !includeAll {
			if _, ok := includedFolders[mb.Name]; !ok {
				continue
			}
			includedFolders[mb.Name] = true
		}

		folderNames = append(folderNames, mb.Name)
	}

	// Check if an error occurred while fetching data
	select {
	case err := <-errChan:
		return nil, err
	default:
	}

	// Check if any of the specified folders were missing on the server
	for folder, seen := range includedFolders {
		if !seen {
			return nil, fmt.Errorf("folder %s not found on server", folder)
		}
	}

	return folderNames, nil
}

// CheckMessages checks for new/unindexed messages on the server
// If 'fullScan' is set to true, we will iterate through all messages, and check for
// any updated flags that doesn't match our current set
func (h *Handler) CheckMessages(ctx context.Context, md *maildir.Maildir) error {
	var err error

	mailboxes, err := h.listFolders()
	if err != nil {
		return err
	}

	for _, mailboxName := range mailboxes {
		err = md.CreateFolder(mailboxName)
		if err != nil {
			return err
		}

		err = h.mailboxFetchMessages(ctx, md, mailboxName)
		if err != nil {
			return err
		}
	}
	return nil
}
