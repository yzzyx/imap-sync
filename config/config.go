// Copyright © 2020 Elias Norberg
// Licensed under the GPLv3 or later.
// See COPYING at the root of the repository for details.
package config

// Config describes the available configuration layout
type Config struct {
	Mailboxes map[string]Mailbox
}

// Mailbox defines the available options for a IMAP mailbox to pull from
type Mailbox struct {
	// Local settings
	Maildir string // Local maildir storage

	// Remote settings
	Server      string
	Port        int
	Username    string
	Password    string
	PasswordCmd string `yaml:"password_cmd"`
	UseTLS      bool   `yaml:"use_tls"`
	UseStartTLS bool   `yaml:"use_starttls"`

	Folders struct {
		Include []string
		Exclude []string
	}
}
