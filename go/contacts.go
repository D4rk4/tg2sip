package main

import (
	"strings"
	"sync"

	client "github.com/zelenin/go-tdlib/client"
)

// ContactCache stores mappings from username and phone to Telegram user IDs.
type ContactCache struct {
	mu           sync.RWMutex
	usernameToID map[string]int64
	phoneToID    map[string]int64
}

// NewContactCache creates an empty ContactCache.
func NewContactCache() *ContactCache {
	return &ContactCache{
		usernameToID: make(map[string]int64),
		phoneToID:    make(map[string]int64),
	}
}

// Refresh reloads contacts using GetContacts and SearchContacts.
func (c *ContactCache) Refresh(cl *client.Client) error {
	ids := map[int64]struct{}{}
	// Get all contacts
	contacts, err := cl.GetContacts()
	if err != nil {
		return err
	}
	for _, id := range contacts.UserIds {
		ids[id] = struct{}{}
	}
	// Also search all contacts to fill ids (query empty returns all)
	if res, err := cl.SearchContacts(&client.SearchContactsRequest{Query: "", Limit: 100}); err == nil {
		for _, id := range res.UserIds {
			ids[id] = struct{}{}
		}
	}
	users := make([]*client.User, 0, len(ids))
	for id := range ids {
		u, err := cl.GetUser(&client.GetUserRequest{UserId: id})
		if err != nil {
			continue
		}
		users = append(users, u)
	}
	c.Set(users)
	return nil
}

// Set replaces cache content with provided users.
func (c *ContactCache) Set(users []*client.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.usernameToID = make(map[string]int64)
	c.phoneToID = make(map[string]int64)
	for _, u := range users {
		c.addLocked(u)
	}
}

// Update adds or updates a single user in the cache.
func (c *ContactCache) Update(u *client.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addLocked(u)
}

// addLocked stores user info; caller must hold write lock.
func (c *ContactCache) addLocked(u *client.User) {
	if u == nil {
		return
	}
	if u.Username != "" {
		c.usernameToID[strings.ToLower(u.Username)] = u.Id
	}
	if u.PhoneNumber != "" {
		c.phoneToID[u.PhoneNumber] = u.Id
	}
}

// Resolve returns user ID for given extension (username or phone).
func (c *ContactCache) Resolve(ext string) (int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if id, ok := c.usernameToID[strings.ToLower(ext)]; ok {
		return id, true
	}
	if id, ok := c.phoneToID[ext]; ok {
		return id, true
	}
	return 0, false
}

// SearchAndAdd searches contact by query and adds it to cache.
func (c *ContactCache) SearchAndAdd(cl *client.Client, query string) (int64, bool) {
	res, err := cl.SearchContacts(&client.SearchContactsRequest{Query: query, Limit: 1})
	if err != nil || len(res.UserIds) == 0 {
		return 0, false
	}
	u, err := cl.GetUser(&client.GetUserRequest{UserId: res.UserIds[0]})
	if err != nil {
		return 0, false
	}
	c.Update(u)
	return u.Id, true
}
