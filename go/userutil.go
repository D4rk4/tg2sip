package main

import client "github.com/zelenin/go-tdlib/client"

// getUsername returns the primary username for a user if available.
func getUsername(u *client.User) string {
	if u == nil || u.Usernames == nil {
		return ""
	}
	if u.Usernames.EditableUsername != "" {
		return u.Usernames.EditableUsername
	}
	if len(u.Usernames.ActiveUsernames) > 0 {
		return u.Usernames.ActiveUsernames[0]
	}
	return ""
}
