// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package imap

import (
	"errors"
	"time"

	"github.com/emersion/go-imap"
	"github.com/yzzyx/imap-sync/mail"
)

// AddMessage uploads a message to the IMAP server, and places it in the specific folder
func (h *Handler) AddMessage(info mail.Info, reader imap.Literal) (mail.Info, error) {
	hasUIDPlus, err := h.client.SupportUidPlus()
	if err != nil {
		return info, err
	}

	if !hasUIDPlus {
		return info, errors.New("server does not support UIDPLUS, which is currently required for pushing new messages to server")
	}

	// FIXME - time should preferably be read from message
	flags := mail.FlagsToIMAP(info.Flags)
	uidValidity, uid, err := h.client.UidPlusClient.Append(info.FolderName, flags, time.Now(), reader)
	if err != nil {
		return info, err
	}

	// Servers are not forced to return UID, but we need them
	// in order to keep track, so we'll error out
	if uidValidity == 0 || uid == 0 {
		return info, errors.New("server did not return UID for added message")
	}

	// Write updated info back to database
	info.UIDValidity = int(uidValidity)
	info.UID = int(uid)
	info.Flags = mail.FlagsFromIMAP(flags)
	return info, err
}
