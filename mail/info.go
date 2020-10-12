// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package mail

// Info contains basic mail information
type Info struct {
	FolderName string
	Filename   string

	UIDValidity int
	UID         int
	Flags       []string
}
