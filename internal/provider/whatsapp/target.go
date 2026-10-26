package whatsapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/provider"
)

// Target kinds.
const (
	KindNumber = "number"
	KindGroup  = "group"
)

const (
	userServer  = "s.whatsapp.net"
	groupServer = "g.us"
)

// target is the stored form of a recipient's target.
type target struct {
	Kind   string `json:"kind"`
	Number string `json:"number,omitempty"`
	JID    string `json:"jid,omitempty"`
}

func targetInvalid(field string, err error) error {
	return apperr.New(apperr.RecipientTargetInvalid, err).With("field", field)
}

// parseTarget decodes and normalizes a target. It needs no connection.
func parseTarget(raw json.RawMessage) (target, error) {
	var t target
	if err := json.Unmarshal(raw, &t); err != nil {
		return target{}, targetInvalid("kind", fmt.Errorf("decode target: %w", err))
	}
	switch t.Kind {
	case KindNumber:
		n, err := normalizeNumber(t.Number)
		if err != nil {
			return target{}, err
		}
		return target{Kind: KindNumber, Number: n}, nil
	case KindGroup:
		jid, err := normalizeGroupJID(t.JID)
		if err != nil {
			return target{}, err
		}
		return target{Kind: KindGroup, JID: jid}, nil
	default:
		return target{}, targetInvalid("kind", fmt.Errorf("unknown target kind %q", t.Kind))
	}
}

func (t target) raw() json.RawMessage {
	b, err := json.Marshal(t)
	if err != nil { // a struct of strings: only a bug gets here
		return json.RawMessage(`{}`)
	}
	return b
}

// jid is the address to send to.
func (t target) jid() string {
	if t.Kind == KindNumber {
		return strings.TrimPrefix(t.Number, "+") + "@" + userServer
	}
	return t.JID
}

// normalizeNumber turns a typed number into E.164 ("+393331234567"):
// spaces, dashes, dots and brackets are dropped; the + and the country code
// are required (a leading 00 is accepted as +). E.164 numbers have at most
// 15 digits; fewer than 8 can't be a mobile number with its country code.
func normalizeNumber(s string) (string, error) {
	in := strings.TrimSpace(s)
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')', '/', ' ':
			return -1
		}
		return r
	}, in)
	if strings.HasPrefix(clean, "00") {
		clean = "+" + clean[2:]
	}
	bad := apperr.New(apperr.WANumberInvalid, fmt.Errorf("not an international number: %s", logging.MaskPhone(in))).With("value", in)
	if !strings.HasPrefix(clean, "+") {
		return "", bad
	}
	digits := clean[1:]
	if len(digits) < 8 || len(digits) > 15 || digits[0] == '0' {
		return "", bad
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", bad
		}
	}
	return clean, nil
}

func normalizeGroupJID(s string) (string, error) {
	s = strings.TrimSpace(s)
	user, server, ok := strings.Cut(s, "@")
	if !ok || server != groupServer || user == "" || strings.ContainsAny(user, ":@") {
		return "", targetInvalid("jid", errors.New("not a group JID"))
	}
	return user + "@" + groupServer, nil
}

// errNotOnWhatsApp and errGroupNotFound reject a target for good
// (provider.ErrTargetInvalid), so the Manager stores it as not verified.
func errNotOnWhatsApp(number string) error {
	return apperr.New(apperr.WANotOnWhatsApp, fmt.Errorf("%s: %w", logging.MaskPhone(number), provider.ErrTargetInvalid)).With("value", number)
}

func errGroupNotFound(jid string) error {
	return apperr.New(apperr.WAGroupNotFound, fmt.Errorf("group not joined: %w", provider.ErrTargetInvalid)).With("name", jid)
}
