// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package imap

import (
	"context"
	"errors"
	"math"

	"github.com/emersion/go-imap"
	"github.com/schollz/progressbar/v3"
	"github.com/yzzyx/imap-sync/mail"
	"github.com/yzzyx/imap-sync/maildir"
)

// getMessage downloads a message from the server from a mailbox, and stores it in a maildir
func (h *Handler) getMessage(ctx context.Context, md *maildir.Maildir, folderName string, uidValidity uint32, uid uint32) error {
	// Download whole body
	section := &imap.BodySectionName{
		Peek: true, // Do not update seen-flags
	}
	items := []imap.FetchItem{section.FetchItem(), imap.FetchFlags}
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	messages := make(chan *imap.Message)
	done := make(chan error)
	go func() {
		done <- h.client.UidFetch(seqSet, items, messages)
	}()

	msg := <-messages
	if msg == nil {
		return errors.New("server didn't return message")
	}

	r := msg.GetBody(section)
	if r == nil {
		return errors.New("server didn't return message body")
	}

	err := <-done
	if err != nil {
		return err
	}

	info := mail.Info{
		FolderName:  folderName,
		UIDValidity: int(uidValidity),
		UID:         int(uid),
		Flags:       mail.FlagsFromIMAP(msg.Flags),
	}
	_, err = md.AddMessage(info, r)
	return err
}

// mailboxFetchMessages checks for any new messages in mailbox
func (h *Handler) mailboxFetchMessages(ctx context.Context, md *maildir.Maildir, folderName string) error {
	mbox, err := h.client.Select(folderName, false)
	if err != nil {
		return err
	}

	if mbox.Messages == 0 {
		return nil
	}

	// Search for new UID's
	seqSet := new(imap.SeqSet)

	lastSeenUID := uint32(0)
	//	Should we handle a full sync?
	uidValidity, uid, err := md.GetLastUID(folderName)
	if err != nil {
		return err
	}
	if uidValidity > 0 && int(mbox.UidValidity) != uidValidity {
		return errors.New("UID validity for mailbox does not match our value - not handled")
	}
	lastSeenUID = uint32(uid)

	// Note that we search from lastSeenUID to MAX, instead of
	//   lastSeenUID to '*', because the latter always returns at least one entry
	seqSet.AddRange(lastSeenUID+1, math.MaxUint32)

	// Fetch envelope information (contains messageid, and UID, which we'll use to fetch the body
	items := []imap.FetchItem{imap.FetchUid}

	messages := make(chan *imap.Message, 100)
	errchan := make(chan error, 1)

	go func() {
		if err := h.client.UidFetch(seqSet, items, messages); err != nil {
			errchan <- err
		}
	}()

	uidList := []uint32{}
	for msg := range messages {
		if msg == nil {
			// We're done
			break
		}

		if msg.Uid == 0 {
			return errors.New("server did not return UID")
		}

		if msg.Uid > lastSeenUID {
			lastSeenUID = msg.Uid
		}
		uidList = append(uidList, msg.Uid)
	}

	// Check if an error occurred while fetching data
	select {
	case err := <-errchan:
		return err
	default:
	}

	progress := progressbar.NewOptions(len(uidList), progressbar.OptionSetDescription(folderName))
	for _, uid := range uidList {
		progress.Add(1)

		err = h.getMessage(ctx, md, folderName, mbox.UidValidity, uid)

		if err != nil {
			return err
		}
	}
	return nil
}
