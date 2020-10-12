// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package maildir

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/yzzyx/imap-sync/mail"
)

// SyncUUID is used to identify files that has been created by us
const SyncUUID = "7f4f3b23-ad6c-434d-9fa9-dbfa7a51397e"

// Maildir keeps track of messages in a mail dir
type Maildir struct {
	path       string
	hostname   string
	processID  int
	startTime  time.Time
	seqNumChan <-chan int
	done       chan bool
}

// New creates a new maildir instance that can be used to track messages
func New(maildirPath string) (*Maildir, error) {
	var err error
	m := &Maildir{path: maildirPath}

	m.hostname, err = os.Hostname()
	if err != nil {
		return nil, err
	}

	seqNumChan := make(chan int)
	done := make(chan bool)
	go func() {
		seqNum := 1
		for {
			select {
			case seqNumChan <- seqNum:
				seqNum++
			case <-done:
				return
			}
		}
	}()
	m.done = done
	m.seqNumChan = seqNumChan
	m.processID = os.Getpid()
	m.startTime = time.Now()

	return m, nil
}

// CreateMailDir creates new directories to store maildir entries in
// with the correct subfolders and permissions
func (m *Maildir) CreateFolder(folderName string) error {
	folderPath := filepath.Join(m.path, folderName)
	if st, err := os.Stat(folderPath); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("path %s is not a directory", folderPath)
		}
		// Path exists and is a directory, so we're done
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	err := os.MkdirAll(filepath.Join(folderPath, "tmp"), 0700)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(folderPath, "cur"), 0700)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(folderPath, "new"), 0700)
	if err != nil {
		return err
	}

	return nil
}

// Close cleans up the maildir instance
func (m *Maildir) Close() {
	// cleanup goroutine
	close(m.done)
}

// GetLastUID updates our uid validity-file for the specific folder
func (m *Maildir) GetLastUID(folderName string) (uidValidity int, uid int, err error) {
	uidValidityFd, err := os.Open(filepath.Join(m.path, folderName, ".uidvalidity"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	defer uidValidityFd.Close()

	scanner := bufio.NewScanner(uidValidityFd)
	var intList [2]int
	for row := 0; scanner.Scan() && row < 2; row++ {
		intList[row], err = strconv.Atoi(scanner.Text())
		if err != nil {
			return 0, 0, err
		}
	}
	if err = scanner.Err(); err != nil {
		return 0, 0, err
	}

	return intList[0], intList[1], nil
}

// UpdateUIDValidity updates our uid validity-file for the specific folder
func (m *Maildir) updateUIDValidity(info mail.Info) error {
	uidValidityFd, err := os.OpenFile(filepath.Join(m.path, info.FolderName, ".uidvalidity"), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer uidValidityFd.Close()

	_, err = fmt.Fprintln(uidValidityFd, info.UIDValidity)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(uidValidityFd, info.UID)
	if err != nil {
		return err
	}
	return nil
}

// HasMessage returns true if the message exists, and false if it doesn't
func (m *Maildir) HasMessage(folderName string, info mail.Info) (bool, error) {
	uidValidityFd, err := os.Open(filepath.Join(folderName, ".uidvalidity"))
	if err != nil {
		if err == os.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	defer uidValidityFd.Close()

	scanner := bufio.NewScanner(uidValidityFd)
	var intList [2]int
	for row := 0; scanner.Scan() && row < 2; row++ {
		intList[row], err = strconv.Atoi(scanner.Text())
		if err != nil {
			return false, err
		}
	}
	if err = scanner.Err(); err != nil {
		return false, err
	}

	if intList[0] != info.UIDValidity {
		return false, errors.New("uid validity has changed - not implemented")
	}
	return info.UID > intList[1], nil
}

// AddMessage adds a message to a folder, and updates the uidvalidity flags
func (m *Maildir) AddMessage(info mail.Info, contents imap.Literal) (mail.Info, error) {
	sort.Strings(info.Flags)
	flags := strings.Join(info.Flags, "")

	filename := fmt.Sprintf("%d.P%dQ%dS%s.%s,U=%d:2,%s",
		m.startTime.Unix(),
		m.processID,
		<-m.seqNumChan,
		SyncUUID,
		m.hostname,
		info.UID,
		flags)
	mailboxPath := filepath.Join(m.path, info.FolderName)
	tmpPath := filepath.Join(mailboxPath, "tmp", filename)

	fd, err := os.Create(tmpPath)
	if err != nil {
		return info, err
	}

	_, err = io.Copy(fd, contents)
	if err != nil {
		// Perform cleanup
		fd.Close()
		os.Remove(tmpPath)
		return info, err
	}
	fd.Close()

	newPath := filepath.Join(mailboxPath, "cur", filename)
	err = os.Rename(tmpPath, newPath)
	if err != nil {
		// Could not rename file - discard old entry to avoid duplicates
		os.Remove(tmpPath)
		return info, err
	}

	err = m.updateUIDValidity(info)
	if err != nil {
		// Could not update UID validity, remove the new file to avoid duplicates
		os.Remove(newPath)
		return info, err
	}

	info.Filename = newPath
	return info, nil
}

// RenameMessage renames a message from the current name to the expected imap-sync name
// This also tags the file as synced
func (m *Maildir) RenameMessage(info mail.Info) (mail.Info, error) {
	sort.Strings(info.Flags)
	flags := strings.Join(info.Flags, "")

	filename := fmt.Sprintf("%d.P%dQ%dS%s.%s,U=%d:2,%s",
		m.startTime.Unix(),
		m.processID,
		<-m.seqNumChan,
		SyncUUID,
		m.hostname,
		info.UID,
		flags)
	mailboxPath := filepath.Join(m.path, info.FolderName)
	newPath := filepath.Join(mailboxPath, "cur", filename)

	err := os.Rename(info.Filename, newPath)
	if err != nil {
		return info, err
	}
	err = m.updateUIDValidity(info)
	if err != nil {
		// Could not update UID validity, move file back to avoid inconsistency
		os.Rename(newPath, info.Filename)
		return info, err
	}

	info.Filename = newPath
	return info, err
}
