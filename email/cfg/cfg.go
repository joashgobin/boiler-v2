package cfg

import (
	"fmt"
	"sync"
)

type MailConfig struct {
	Recipients []string
	Sender     string
	SenderName string
	Cc         []string
	Bcc        []string
	Subject    string
	Body       string
	WaitGroup  *sync.WaitGroup
}

type MailOption func(*MailConfig)

func NewMailConfig(wg *sync.WaitGroup, opts ...MailOption) MailConfig {
	cfg := &MailConfig{WaitGroup: wg}
	for _, opt := range opts {
		opt(cfg)
	}
	return *cfg
}

func WithSenderAddress(senderAddress string) MailOption {
	return func(c *MailConfig) {
		c.Sender = senderAddress
	}
}

func WithSenderName(senderName string) MailOption {
	return func(c *MailConfig) {
		c.SenderName = senderName
	}
}

func WithRecipients(cc ...string) MailOption {
	return func(c *MailConfig) {
		c.Recipients = cc
	}
}

func WithBlindRecipients(bcc ...string) MailOption {
	return func(c *MailConfig) {
		c.Bcc = bcc
	}
}

func WithSubject(subjectStr string, args ...any) MailOption {
	return func(c *MailConfig) {
		subject := subjectStr
		if len(args) > 0 {
			subject = fmt.Sprintf(subjectStr, args...)
		}
		c.Subject = subject
	}
}

func WithBody(bodyStr string, args ...any) MailOption {
	return func(c *MailConfig) {
		body := bodyStr
		if len(args) > 0 {
			body = fmt.Sprintf(bodyStr, args...)
		}
		c.Body = body
	}
}
