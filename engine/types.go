package main

// The API contract. Field names and shapes mirror src/lib/types.ts.

type Account struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Provider string `json:"provider"`
	Color    string `json:"color"`
}

type Mailbox struct {
	ID        string  `json:"id"`
	AccountID string  `json:"accountId"`
	Name      string  `json:"name"`
	Role      *string `json:"role"` // nil = user folder
	ParentID  *string `json:"parentId"`
	Unread    int     `json:"unread"`
}

type Address struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Attachment struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type Message struct {
	ID          string       `json:"id"`
	ThreadID    string       `json:"threadId"`
	From        Address      `json:"from"`
	To          []Address    `json:"to"`
	Cc          []Address    `json:"cc"`
	Date        string       `json:"date"`
	Body        string       `json:"body"`
	Attachments []Attachment `json:"attachments"`
}

// Thread is a conversation. Lists leave Messages empty (a ThreadSummary in TypeScript); GET /threads/{id} fills it.
type Thread struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"accountId"`
	MailboxIDs     []string  `json:"mailboxIds"`
	Subject        string    `json:"subject"`
	Snippet        string    `json:"snippet"`
	Participants   []Address `json:"participants"`
	LastDate       string    `json:"lastDate"`
	Unread         bool      `json:"unread"`
	Starred        bool      `json:"starred"`
	HasAttachments bool      `json:"hasAttachments"`
	Messages       []Message `json:"messages,omitempty"`
}
