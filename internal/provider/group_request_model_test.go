package provider

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

func requestModelFixture(t *testing.T) Group {
	t.Helper()
	isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, p := range []Provider{
		{ID: "one", Models: []string{"gpt-base", "gpt-new", "gemini-new"}},
		{ID: "two", Models: []string{"gpt-base", "gpt-new"}},
		{ID: "fixed", Models: []string{"other-model"}},
	} {
		p.Name, p.Key, p.Chat = p.ID, "fixture", "http://127.0.0.1:1/v1"
		if err := Save(p); err != nil {
			t.Fatal(err)
		}
	}
	var g Group
	if err := json.Unmarshal([]byte(`{"id":"pool","members":["one/gpt-base","two/gpt-base","fixed/other-model"],"requestModelProviders":["one","two"],"routing":"weight","weights":{"one/gpt-base":3}}`), &g); err != nil {
		t.Fatal(err)
	}
	if err := SaveGroup(g); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGroupRequestedModel(t *testing.T) {
	requestModelFixture(t)
	g, ms, ok := FindGroup("group/pool@gpt-new")
	if !ok || g.ID != "pool" || len(ms) != 3 {
		t.Fatalf("requested-model group: found %v, id %q, members %d", ok, g.ID, len(ms))
	}
	for i, want := range []string{"gpt-new", "gpt-new", "other-model"} {
		if ms[i].Model != want || ms[i].ID != g.Members[i] || !slices.Equal(ms[i].Path, []string{g.Members[i]}) {
			t.Fatalf("member %d lost model or stable identity: %+v", i, ms[i])
		}
	}
	if g.MemberWeight(ms[0].ID) != 3 {
		t.Fatal("requested model lost its provider's configured weight")
	}
	if p, m, ok := Resolve("group/pool@gpt-new"); !ok || p.ID != "one" || m != "gpt-new" {
		t.Fatalf("Resolve: %v %s %s", ok, p.ID, m)
	}
	_, plain, _ := FindGroup("group/pool")
	if plain[0].Model != "gpt-base" {
		t.Fatal("a request changed the stored group")
	}
	_, other, ok := FindGroup("group/pool@gemini-new")
	if !ok || len(other) != 2 || other[0].Model != "gemini-new" || other[1].Model != "other-model" {
		t.Fatalf("non-GPT model filtering: %+v", other)
	}
}

func TestGroupRequestedModelHonorsCatalogAndDisabled(t *testing.T) {
	requestModelFixture(t)
	for _, model := range []string{"gpt-unknown", "gpt-new"} {
		if model == "gpt-new" {
			if err := SetDisabledModels([]string{"one/gpt-new", "two/gpt-new"}); err != nil {
				t.Fatal(err)
			}
		}
		_, ms, ok := FindGroup("group/pool@" + model)
		if !ok || len(ms) != 1 || ms[0].Provider.ID != "fixed" {
			t.Fatalf("%s reached an unlisted or disabled model: %+v", model, ms)
		}
	}
	for _, ref := range []string{"group/pool@", "group/pool@x/y", "group/pool@x@other", "group/pool@gpt-new\u00a0", "group/pool@gpt-*"} {
		if _, _, ok := FindGroup(ref); ok {
			t.Errorf("accepted invalid model reference %q", ref)
		}
	}
	if err := SaveGroup(Group{ID: "fixed-only", Members: []string{"fixed/other-model"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := FindGroup("group/fixed-only@gpt-new"); ok {
		t.Fatal("a fixed-model group accepted an override")
	}
}

func TestGroupRequestedModelSurvivesOldEditor(t *testing.T) {
	requestModelFixture(t)
	// An older editor sends its known fields only.
	if err := SaveGroup(Group{ID: "pool", Name: "Renamed", Members: []string{"one/gpt-base", "two/gpt-base", "fixed/other-model"}}); err != nil {
		t.Fatal(err)
	}
	if _, ms, ok := FindGroup("group/pool@gpt-new"); !ok || len(ms) != 3 || ms[0].Model != "gpt-new" {
		t.Fatalf("saving from an old editor lost the opt-in: found %v, members %+v", ok, ms)
	}
	// Changing the named model does not change which provider opted in.
	if err := SaveGroup(Group{ID: "pool", Members: []string{"one/gpt-new", "two/gpt-base", "fixed/other-model"}}); err != nil {
		t.Fatal(err)
	}
	if _, ms, ok := FindGroup("group/pool@gemini-new"); !ok || len(ms) != 2 || ms[0].Model != "gemini-new" {
		t.Fatalf("changing the anchor lost the provider opt-in: %v %+v", ok, ms)
	}
}

func TestGroupRequestedModelImportMapsProviders(t *testing.T) {
	requestModelFixture(t)
	text, err := ShareGroup("pool")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	for _, p := range []Provider{
		{ID: "relay", Models: []string{"gpt-base", "gpt-new"}},
		{ID: "fixed", Models: []string{"other-model"}},
	} {
		p.Name, p.Key, p.Chat = p.ID, "fixture", "http://127.0.0.1:1/v1"
		if err := Save(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ImportGroup(text); err != nil {
		t.Fatal(err)
	}
	if _, ms, ok := FindGroup("group/pool@gpt-new"); !ok || len(ms) != 2 || ms[0].Provider.ID != "relay" || ms[0].Model != "gpt-new" {
		t.Fatalf("import lost its remapped provider opt-in: %v %+v", ok, ms)
	}
}

func TestGroupRequestedModelFollowsProviderRename(t *testing.T) {
	requestModelFixture(t)
	if err := Rename("one", "renamed"); err != nil {
		t.Fatal(err)
	}
	_, ms, ok := FindGroup("group/pool@gpt-new")
	if !ok || len(ms) != 3 || ms[0].Provider.ID != "renamed" || ms[0].Model != "gpt-new" {
		t.Fatalf("provider rename lost its opt-in: %v %+v", ok, ms)
	}
}

func TestGroupRequestedModelOptInValidation(t *testing.T) {
	requestModelFixture(t)
	for _, text := range []string{
		`{"id":"bad","members":["one/gpt-base"],"requestModelProviders":["two"]}`,
		`{"id":"bad","members":["one/gpt-base","one/gpt-new"],"requestModelProviders":["one"]}`,
		`{"id":"bad","members":["one/gpt-base","group/pool"],"requestModelProviders":["one"]}`,
	} {
		var g Group
		if err := json.Unmarshal([]byte(text), &g); err != nil {
			t.Fatal(err)
		}
		if err := SaveGroup(g); err == nil {
			t.Errorf("accepted ambiguous requested-model membership: %s", text)
		}
	}
	var cleared Group
	if err := json.Unmarshal([]byte(`{"id":"pool","members":["one/gpt-base","two/gpt-base","fixed/other-model"],"requestModelProviders":[]}`), &cleared); err != nil {
		t.Fatal(err)
	}
	if err := SaveGroup(cleared); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := FindGroup("group/pool@gpt-new"); ok {
		t.Fatal("an explicit empty list did not clear requested-model routing")
	}
}
