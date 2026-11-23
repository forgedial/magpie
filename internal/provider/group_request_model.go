package provider

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// validRequestedModel admits only a bare model, not another route or a
// member's account pin. Parameterized groups are opt-in and do
// not turn unknown model names into requests to every provider.
func validRequestedModel(model string) bool {
	return model != "" && len(model) <= 120 && !strings.ContainsAny(model, "/@*#") &&
		!strings.ContainsFunc(model, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

func requestedModelMembers(entries []Entry, g Group, ms []Member, requested string, off func(string, string) bool) []Member {
	ms = slices.DeleteFunc(ms, func(m Member) bool {
		return len(m.Via) == 0 && slices.Contains(g.RequestModelProviders, m.Provider.ID) &&
			(off(m.Provider.ID, requested) || !slices.ContainsFunc(entries, func(e Entry) bool {
				return e.Provider.ID == m.Provider.ID && strings.EqualFold(e.Model, requested)
			}))
	})
	for i := range ms {
		if len(ms[i].Via) == 0 && slices.Contains(g.RequestModelProviders, ms[i].Provider.ID) {
			// Keep ID and Path: the scheduler, rules and account controls
			// use the configured member, never a newly invented member.
			ms[i].Model = requested
		}
	}
	return ms
}

// cleanRequestModelProviders keeps an old editor's omitted field and
// permits an explicit empty list to clear it. Each opt-in provider has
// one direct, named member: weights/rules/accounts have one stable place
// for it, independent of how many models that provider serves.
func cleanRequestModelProviders(g *Group, was []Group) error {
	if g.RequestModelProviders == nil {
		if i := slices.IndexFunc(was, func(w Group) bool { return w.ID == g.ID }); i >= 0 {
			g.RequestModelProviders = slices.DeleteFunc(slices.Clone(was[i].RequestModelProviders), func(pid string) bool {
				return !slices.ContainsFunc(g.Members, func(m string) bool {
					p, _, ok := strings.Cut(m, "/")
					return ok && p == pid
				})
			})
		}
	}
	g.RequestModelProviders = cleanList(g.RequestModelProviders)
	if len(g.RequestModelProviders) == 0 {
		return nil
	}
	if len(g.Match) > 0 || slices.ContainsFunc(g.Members, IsGroupRef) {
		return fmt.Errorf("requested-model providers need direct named members, without nested groups or model patterns")
	}
	for _, pid := range g.RequestModelProviders {
		n := 0
		for _, m := range g.Members {
			if p, _, ok := strings.Cut(m, "/"); ok && p == pid {
				n++
			}
		}
		if n != 1 {
			return fmt.Errorf("requested-model provider %q needs exactly one named member, found %d", pid, n)
		}
	}
	return nil
}

// mappedRequestModelProviders carries the opt-in with its member through
// a provider rename or an import that maps a member onto another provider.
func mappedRequestModelProviders(g Group, to func(string) string) []string {
	var out []string
	for _, m := range g.Members {
		pid, _, _ := strings.Cut(m, "/")
		if !slices.Contains(g.RequestModelProviders, pid) {
			continue
		}
		if mapped := to(m); mapped != "" && !IsGroupRef(mapped) {
			pid, _, ok := strings.Cut(mapped, "/")
			if ok && !slices.Contains(out, pid) {
				out = append(out, pid)
			}
		}
	}
	return out
}
