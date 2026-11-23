package access

import "testing"

func TestAliasGlobs(t *testing.T) {
	as, err := CleanAliases(map[string]string{
		"*": "group/general", "gpt-*": "group/pool@$model",
		"gpt-preview-*": "group/preview", "gpt-special": "group/pool@gpt-standard",
	})
	if err != nil {
		t.Fatal(err)
	}
	who := Identity{Aliases: as}
	for in, want := range map[string]string{
		"gpt-special": "group/pool@gpt-standard", "GPT-NEW": "group/pool@GPT-NEW",
		"gpt-preview-new": "group/preview", "other": "group/general",
	} {
		for range 20 {
			if got, ok := who.AliasFor(in); !ok || got != want {
				t.Fatalf("AliasFor(%q) = %q, %v; want %q", in, got, ok, want)
			}
		}
	}
	// Equal specificity is lexical, not Go map iteration order.
	who.Aliases = map[string]string{"g*t-new": "group/second", "gp*-new": "group/first"}
	for range 20 {
		if got, _ := who.AliasFor("gpt-new"); got != "group/second" {
			t.Fatalf("tied globs: %q", got)
		}
	}
	who.Aliases = map[string]string{"gpt-*": "group/pool@$model"}
	for _, bad := range []string{"gpt-x/group/other", "gpt-x@other", "gpt-x\n", "gpt-x\u00a0", "gpt-*", "gpt-x#account"} {
		if got, ok := who.AliasFor(bad); ok {
			t.Errorf("routing syntax substituted from %q: %q", bad, got)
		}
	}
	if got, ok := (Identity{}).AliasFor("gpt-new"); ok {
		t.Fatalf("another key inherited an alias: %q", got)
	}
}

func TestAliasGlobsNotListedAsModelNames(t *testing.T) {
	who := Identity{Aliases: map[string]string{"gpt-*": "group/pool", "public": "group/pool"}}
	if got, ok := who.AliasOf("group/pool"); !ok || got != "public" {
		t.Fatalf("reverse alias = %q, %v", got, ok)
	}
	delete(who.Aliases, "public")
	if got, ok := who.AliasOf("group/pool"); ok {
		t.Fatalf("listed a pattern as a concrete model: %q", got)
	}
}

func TestAliasModelPlaceholderNeedsGroupGlob(t *testing.T) {
	for _, as := range []map[string]string{
		{"plain": "group/pool@$model"}, {"gpt-*": "provider/$model"},
		{"gpt-*": "group/pool@$model-suffix"}, {"gpt-*": "group/$model"},
		{"gpt-*": "group/pool@$model@$model"},
	} {
		if _, err := CleanAliases(as); err == nil {
			t.Errorf("accepted ambiguous template %v", as)
		}
	}
}
