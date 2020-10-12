// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package mail

import "github.com/emersion/go-imap"

/* Flags as defined by the maildir specification (https://cr.yp.to/proto/maildir.html)

Flag "P" (passed): the user has resent/forwarded/bounced this message to someone else.
Flag "R" (replied): the user has replied to this message.
Flag "S" (seen): the user has viewed this message, though perhaps he didn't read all the way through it.
Flag "T" (trashed): the user has moved this message to the trash; the trash will be emptied by a later user action.
Flag "D" (draft): the user considers this message a draft; toggled at user discretion.
Flag "F" (flagged): user-defined flag; toggled at user discretion.
*/
const (
	FlagPassed  = "P"
	FlagReplied = "R"
	FlagSeen    = "S"
	FlagTrashed = "T"
	FlagDraft   = "D"
	FlagFlagged = "F"
)

// FlagIMAPConversionTable is used to map between maildir flags and IMAP flags
var FlagIMAPConversionTable = map[string]string{
	FlagPassed:  "$Forwarded",
	FlagReplied: imap.AnsweredFlag,
	FlagSeen:    imap.SeenFlag,
	FlagTrashed: imap.DeletedFlag,
	FlagDraft:   imap.DraftFlag,
	FlagFlagged: imap.FlaggedFlag,
}

// FlagsToIMAP converts from maildir flags to IMAP flags
func FlagsToIMAP(s []string) (imapFlags []string) {
	for _, v := range s {
		if f, ok := FlagIMAPConversionTable[v]; ok {
			imapFlags = append(imapFlags, f)
		}
	}
	return imapFlags
}

// FlagsFromIMAP converts from IMAP flags to maildir flags
func FlagsFromIMAP(s []string) (flags []string) {
	for _, flag := range s {
		for mailFlag, imapFlag := range FlagIMAPConversionTable {
			if imapFlag != flag {
				continue
			}
			flags = append(flags, mailFlag)
		}
	}
	return flags
}
