package provider

import (
	"fmt"
	"slices"

	"github.com/jacopofilonzi/wchron/internal/apperr"
)

// Registry holds the known providers by type. main registers the real ones
// (internal/provider/whatsapp…); tests register provider/fake.
type Registry struct {
	byType map[string]Provider
	order  []string
}

// NewRegistry returns a registry with the given providers, in that order.
func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{byType: map[string]Provider{}}
	for _, p := range ps {
		if _, dup := r.byType[p.Type()]; !dup {
			r.order = append(r.order, p.Type())
		}
		r.byType[p.Type()] = p
	}
	return r
}

// Get returns the provider of a type, or channel.provider_unknown.
func (r *Registry) Get(typ string) (Provider, error) {
	p, ok := r.byType[typ]
	if !ok {
		return nil, apperr.New(apperr.ChannelProviderUnknown, fmt.Errorf("unknown provider %q", typ)).With("provider", typ)
	}
	return p, nil
}

// All returns the providers in registration order.
func (r *Registry) All() []Provider {
	out := make([]Provider, 0, len(r.order))
	for _, t := range slices.Clone(r.order) {
		out = append(out, r.byType[t])
	}
	return out
}

// Info describes a provider for the GUI.
type Info struct {
	Type           string       `json:"type"`
	Capabilities   Capabilities `json:"capabilities"`
	SettingsSchema []Field      `json:"settings_schema"`
	TargetSchema   []Field      `json:"target_schema"`
}

// Infos returns every provider's Info, in registration order.
func (r *Registry) Infos() []Info {
	var out []Info
	for _, p := range r.All() {
		out = append(out, Info{Type: p.Type(), Capabilities: p.Capabilities(), SettingsSchema: p.SettingsSchema(), TargetSchema: p.TargetSchema()})
	}
	return out
}
