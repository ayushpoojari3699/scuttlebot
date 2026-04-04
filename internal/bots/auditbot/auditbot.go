package auditbot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lrstanley/girc"

	"github.com/conflicthq/scuttlebot/pkg/protocol"
)

const botNick = "auditbot"

// ===== TYPES =====

type EventKind string

const (
	KindIRC      EventKind = "irc"
	KindRegistry EventKind = "registry"
)

type Entry struct {
	At          time.Time
	Kind        EventKind
	Channel     string
	Nick        string
	MessageType string
	MessageID   string
	Detail      string
}

type Store interface {
	Append(Entry) error
}

// ===== BOT =====

type Bot struct {
	ircAddr    string
	password   string
	channels   []string
	auditTypes map[string]struct{}

	store  Store
	log    *slog.Logger
	client *girc.Client

	mu sync.Mutex
}

// ===== CONSTRUCTOR =====

func New(ircAddr, password string, channels []string, auditTypes []string, store Store, log *slog.Logger) (*Bot, error) {
	if ircAddr == "" {
		return nil, errors.New("ircAddr cannot be empty")
	}
	if store == nil {
		return nil, errors.New("store cannot be nil")
	}
	if log == nil {
		log = slog.Default()
	}

	at := make(map[string]struct{}, len(auditTypes))
	for _, t := range auditTypes {
		at[t] = struct{}{}
	}

	return &Bot{
		ircAddr:    ircAddr,
		password:   password,
		channels:   channels,
		auditTypes: at,
		store:      store,
		log:        log,
	}, nil
}

// ===== PUBLIC =====

func (b *Bot) Name() string { return botNick }

func (b *Bot) Record(nick, eventType, detail string) {
	b.write(Entry{
		Kind:        KindRegistry,
		Nick:        nick,
		MessageType: eventType,
		Detail:      detail,
	})
}

func (b *Bot) Start(ctx context.Context) error {
	host, port, err := splitHostPort(b.ircAddr)
	if err != nil {
		return err
	}

	b.log.Info("starting auditbot", "server", host, "port", port)

	client := girc.New(girc.Config{
		Server: host,
		Port:   port,
		Nick:   botNick,
		User:   botNick,
		Name:   "scuttlebot auditbot",

		SASL:        &girc.SASLPlain{User: botNick, Pass: b.password},
		PingDelay:   30 * time.Second,
		PingTimeout: 30 * time.Second,
		SSL:         false,
	})

	b.registerHandlers(client)

	b.mu.Lock()
	b.client = client
	b.mu.Unlock()

	errCh := make(chan error, 1)

	go func() {
		if err := client.Connect(); err != nil && ctx.Err() == nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		b.log.Info("shutdown signal received")
		b.Stop()
		return nil

	case err := <-errCh:
		return fmt.Errorf("irc connection failed: %w", err)
	}
}

func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client != nil {
		b.log.Info("disconnecting auditbot")
		b.client.Close()
	}
}

// ===== HANDLERS =====

func (b *Bot) registerHandlers(c *girc.Client) {
	c.Handlers.AddBg(girc.CONNECTED, func(cl *girc.Client, _ girc.Event) {
		for _, ch := range b.channels {
			cl.Cmd.Join(ch)
		}
		b.log.Info("connected", "channels", b.channels, "audit_types", b.auditTypesList())
	})

	c.Handlers.AddBg(girc.INVITE, func(cl *girc.Client, e girc.Event) {
		if ch := e.Last(); strings.HasPrefix(ch, "#") {
			cl.Cmd.Join(ch)
		}
	})

	c.Handlers.AddBg(girc.PRIVMSG, func(_ *girc.Client, e girc.Event) {
		b.handleMessage(e)
	})

	//  NEW: JOIN handler
	c.Handlers.AddBg(girc.JOIN, func(_ *girc.Client, e girc.Event) {
		if len(e.Params) == 0 {
			return
		}

		channel := e.Params[0]
		nick := ""
		if e.Source != nil {
			nick = e.Source.Name
		}

		b.write(Entry{
			Kind:        KindIRC,
			Channel:     channel,
			Nick:        nick,
			MessageType: "user.join",
		})
	})

	//  NEW: PART handler
	c.Handlers.AddBg(girc.PART, func(_ *girc.Client, e girc.Event) {
		if len(e.Params) == 0 {
			return
		}

		channel := e.Params[0]
		nick := ""
		if e.Source != nil {
			nick = e.Source.Name
		}

		b.write(Entry{
			Kind:        KindIRC,
			Channel:     channel,
			Nick:        nick,
			MessageType: "user.part",
		})
	})
}
// ===== INTERNAL =====

func (b *Bot) shouldAudit(msgType string) bool {
	if len(b.auditTypes) == 0 {
		return true
	}
	_, ok := b.auditTypes[msgType]
	return ok
}

func (b *Bot) write(e Entry) {
	e.At = time.Now()

	if err := b.store.Append(e); err != nil {
		b.log.Error("failed to write entry",
			"type", e.MessageType,
			"nick", e.Nick,
			"err", err,
		)
	}
}

func (b *Bot) auditTypesList() []string {
	if len(b.auditTypes) == 0 {
		return []string{"*"}
	}

	out := make([]string, 0, len(b.auditTypes))
	for t := range b.auditTypes {
		out = append(out, t)
	}
	return out
}

// ===== UTILS =====

func splitHostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid address %q: %w", addr, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port in %q: %w", addr, err)
	}

	return host, port, nil
}
