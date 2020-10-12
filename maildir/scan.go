// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package maildir

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yzzyx/imap-sync/mail"
)

// Scan writes all new messages to channel 'ch'
func (m *Maildir) Scan(ctx context.Context, ch chan<- mail.Info) error {
	md, err := os.Open(m.path)
	if err != nil {
		return err
	}
	defer md.Close()

	for {
		entries, err := md.Readdir(10)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		for _, e := range entries {
			// Skip files at toplevel
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name[0] == '.' {
				continue
			}

			// FIXME
			// Check if folder is included in sync
			//var include bool
			//if len(mailbox.Folders.Include) > 0 {
			//	for _, includeFolder := range mailbox.Folders.Include {
			//		if name == includeFolder {
			//			include = true
			//			break
			//		}
			//	}
			//} else {
			//	include = true
			//	for _, includeFolder := range mailbox.Folders.Exclude {
			//		if name == includeFolder {
			//			include = false
			//			break
			//		}
			//	}
			//}
			//if !include {
			//	continue
			//}

			err = m.checkMailbox(ctx, filepath.Join(m.path, name), name, ch)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Maildir) checkMailbox(ctx context.Context, mailboxPath string, folderName string, ch chan<- mail.Info) error {
	curPath := filepath.Join(mailboxPath, "cur")
	md, err := os.Open(curPath)
	if err != nil {
		return err
	}
	defer md.Close()

	entries, err := md.Readdirnames(0)
	if err != nil {
		return err
	}

	for _, name := range entries {
		if name[0] == '.' {
			continue
		}

		if IsSynced(name) {
			continue
		}
		messagePath := filepath.Join(curPath, name)

		ch <- mail.Info{
			FolderName: folderName,
			Filename:   messagePath,
			Flags:      parseFileFlags(name),
		}
	}
	return nil
}

func IsSynced(name string) bool {
	pos := strings.Index(name, "S"+SyncUUID)
	return pos > -1
}

func parseFileFlags(filename string) []string {

	parts := strings.Split(filename, ":")
	if len(parts) < 2 {
		return nil
	}
	if !strings.HasPrefix(parts[1], "2,") {
		return nil
	}

	var flags []string
	s := strings.TrimPrefix(parts[1], "2,")
	for _, r := range s {
		switch r {
		case 'P', 'R', 'S', 'T', 'D', 'F':
			flags = append(flags, string(r))
		default:
			return flags
		}
	}
	return flags
}
