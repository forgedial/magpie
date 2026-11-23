package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/access"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/routes"
)

func saveRequestModelGroup(t *testing.T) {
	t.Helper()
	var g provider.Group
	if err := json.Unmarshal([]byte(`{"id":"pool","members":["one/gpt-base","two/gpt-base"],"requestModelProviders":["one","two"],"routing":"weight","affinity":"off","weights":{"one/gpt-base":3}}`), &g); err != nil {
		t.Fatal(err)
	}
	if err := provider.SaveGroup(g); err != nil {
		t.Fatal(err)
	}
}

func TestRequestedModelGroupKeepsWeights(t *testing.T) {
	fresh(t)
	for _, id := range []string{"one", "two"} {
		if err := provider.Save(provider.Provider{ID: id, Name: id, Key: "fixture-" + id, Chat: "http://127.0.0.1:1/v1", Models: []string{"gpt-base", "gpt-new", "gemini-new"}}); err != nil {
			t.Fatal(err)
		}
	}
	saveRequestModelGroup(t)
	s := &Server{}
	t.Cleanup(func() { routes.Forget("group/pool") })
	for _, model := range []string{"gpt-new", "gemini-new"} {
		g, ms, ok := provider.FindGroup("group/pool@" + model)
		if !ok || len(ms) != 2 {
			t.Fatalf("%s: found %v, members %d", model, ok, len(ms))
		}
		routes.Forget("group/pool")
		got := map[string]int{}
		for range 40 {
			cs, _ := s.planGroup(g, ms, provider.Chat)
			if len(cs) != 2 || cs[0].model != model {
				t.Fatalf("%s: planned %+v", model, cs)
			}
			got[cs[0].p.ID]++
		}
		if got["one"] != 30 || got["two"] != 10 {
			t.Fatalf("%s lost 3:1 provider weights: %v", model, got)
		}
	}
}

func TestRequestedModelKeyAliasesKeepGroupBoundary(t *testing.T) {
	fresh(t)
	var mu sync.Mutex
	var asked []string
	seen := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(asked)
	}
	vendor := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		asked = append(asked, modelOf(b))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"fixture","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	})
	for _, id := range []string{"one", "two"} {
		serveOn(t, id, "fixture-"+id, []string{"gpt-base", "gpt-new"}, vendor)
	}
	saveRequestModelGroup(t)
	if err := provider.SaveGroup(provider.Group{ID: "other", Members: []string{"two/gpt-base"}}); err != nil {
		t.Fatal(err)
	}
	keys, secrets := newCaller(t, "Pool", "Other")
	for i, group := range []string{"group/pool", "group/other"} {
		if _, err := access.Update("models-key", access.Change{Key: keys[i].ID, Models: []string{group}}); err != nil {
			t.Fatal(err)
		}
	}
	aliases := map[string]string{"gpt-*": "group/pool@$model", "gpt-expensive": "group/pool@gpt-new"}
	if _, err := access.Update("aliases-key", access.Change{Key: keys[0].ID, Aliases: aliases}); err != nil {
		t.Fatal(err)
	}
	if _, err := access.Update("aliases-key", access.Change{Key: keys[1].ID, Aliases: aliases}); err == nil {
		t.Fatal("another group's key was allowed to install the alias")
	}
	h := New().Handler()
	for _, model := range []string{"gpt-base", "gpt-new", "gpt-expensive"} {
		w := keyDo(h, secrets[0], "POST", "/v1/chat/completions", `{"model":"`+model+`","messages":[{"role":"user","content":"fixture"}]}`)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", model, w.Code, w.Body.String())
		}
	}
	if got := seen(); !slices.Equal(got, []string{"gpt-base", "gpt-new", "gpt-new"}) {
		t.Fatalf("models sent upstream: %v", got)
	}
	before := len(seen())
	for _, model := range []string{"group/pool@gpt-new", "gpt-new"} {
		w := keyDo(h, secrets[1], "POST", "/v1/chat/completions", `{"model":"`+model+`","messages":[]}`)
		if w.Code == 200 || len(seen()) != before {
			t.Fatalf("another key reached %s: %d", model, w.Code)
		}
	}
	if ids := keyListedIDs(t, h, secrets[0], "/v1/models"); slices.Contains(ids, "gpt-*") {
		t.Fatalf("a glob was listed as a real model: %v", ids)
	}
	if _, err := access.Update("models-key", access.Change{Key: keys[0].ID, Models: []string{"group/other"}}); err != nil {
		t.Fatal(err)
	}
	w := keyDo(h, secrets[0], "POST", "/v1/chat/completions", `{"model":"gpt-new","messages":[]}`)
	if w.Code != 403 || len(seen()) != before {
		t.Fatalf("alias bypassed a revoked group: %d", w.Code)
	}
}
