package modrule

import (
	"fmt"
	"github.com/bazelbuild/bzlmod/common"
	"github.com/bazelbuild/bzlmod/common/starutil"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"io/ioutil"
)

type Ruleset struct {
	ResolveFn       *starlark.Function
	FetchFn         *starlark.Function
	Doc             string
	Members         map[string]*Rule
	MachineSpecific bool
	FetchEnviron    []string

	// The following fields are filled post-evaluation.
	Name      string
	ModuleKey common.ModuleKey
}

func (rs *Ruleset) String() string        { return fmt.Sprintf("module_ruleset(%v)", rs.Name) }
func (rs *Ruleset) Type() string          { return "module_ruleset" }
func (rs *Ruleset) Freeze()               {}
func (rs *Ruleset) Truth() starlark.Bool  { return true }
func (rs *Ruleset) Hash() (uint32, error) { return 0, fmt.Errorf("not hashable: module_ruleset") }

type Rule struct {
	Doc   string
	Attrs map[string]*Attr

	// The following fields are filled after this Rule object is used as a Ruleset's member.
	Name    string
	Ruleset *Ruleset
}

func (r *Rule) NewInstance(kwargs []starlark.Tuple) (*RuleInstance, error) {
	inst, err := InstantiateAttrs(r.Attrs, kwargs)
	if err != nil {
		return nil, err
	}
	return &RuleInstance{
		Rule:  r,
		Attrs: inst,
	}, nil
}

func (r *Rule) String() string        { return fmt.Sprintf("module_rule(%v)", r.Name) }
func (r *Rule) Type() string          { return "module_rule" }
func (r *Rule) Freeze()               {}
func (r *Rule) Truth() starlark.Bool  { return true }
func (r *Rule) Hash() (uint32, error) { return 0, fmt.Errorf("not hashable: module_rule") }

type RuleInstance struct {
	Rule  *Rule
	Attrs map[string]starlark.Value
}

func (ri *RuleInstance) String() string        { return "RuleInstance[...]" }
func (ri *RuleInstance) Type() string          { return "RuleInstance" }
func (ri *RuleInstance) Truth() starlark.Bool  { return true }
func (ri *RuleInstance) Hash() (uint32, error) { return 0, fmt.Errorf("not hashable: RuleInstance") }

func (ri *RuleInstance) Freeze() {
	for _, attr := range ri.Attrs {
		attr.Freeze()
	}
}

func (ri *RuleInstance) Attr(name string) (starlark.Value, error) {
	return ri.Attrs[name], nil
}

func (ri *RuleInstance) AttrNames() []string {
	keys := make([]string, len(ri.Attrs))
	i := 0
	for key := range ri.Attrs {
		keys[i] = key
		i++
	}
	return keys
}

type ResolveResult struct {
	Repos         map[string]starlark.Value
	Toolchains    []string
	ExecPlatforms []string
}

func (rr *ResolveResult) String() string        { return "ResolveResult[...]" }
func (rr *ResolveResult) Type() string          { return "ResolveResult" }
func (rr *ResolveResult) Freeze()               {}
func (rr *ResolveResult) Truth() starlark.Bool  { return true }
func (rr *ResolveResult) Hash() (uint32, error) { return 0, fmt.Errorf("not hashable: ResolveResult") }

func resolveResultFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("%v: unexpected positional arguments", b.Name())
	}
	var (
		repos         *starlark.Dict
		toolchains    *starlark.List
		execPlatforms *starlark.List
	)
	err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"repos", &repos,
		"toolchains_to_register?", &toolchains,
		"execution_platforms_to_register?", &execPlatforms,
	)
	if err != nil {
		return nil, err
	}
	result := &ResolveResult{Repos: make(map[string]starlark.Value)}
	for _, item := range repos.Items() {
		s, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("got %v, want string", item[0].Type())
		}
		result.Repos[s] = item[1]
	}
	result.Toolchains, err = starutil.ExtractStringSlice(toolchains)
	if err != nil {
		return nil, err
	}
	result.ExecPlatforms, err = starutil.ExtractStringSlice(execPlatforms)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func moduleRuleFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("%v: unexpected positional arguments", b.Name())
	}
	var (
		resolveFn    starlark.Value
		fetchFn      starlark.Value
		attrs        *starlark.Dict
		fetchEnviron *starlark.List
	)
	ruleset := &Ruleset{}
	err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"resolve_fn", &resolveFn,
		"fetch_fn", &fetchFn,
		"doc?", &ruleset.Doc,
		"attrs?", &attrs,
		"machine_specific?", &ruleset.MachineSpecific,
		"fetch_environ?", &fetchEnviron,
	)
	if err != nil {
		return nil, err
	}
	var ok bool
	ruleset.ResolveFn, ok = resolveFn.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("resolve_fn must be a function")
	}
	ruleset.FetchFn, ok = fetchFn.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("fetch_fn must be a function")
	}
	rule := &Rule{
		Name:    "", // to be filled after eval
		Ruleset: ruleset,
		Doc:     ruleset.Doc,
	}
	rule.Attrs, err = ExtractAttrMap(attrs)
	if err != nil {
		return nil, err
	}
	ruleset.Members = map[string]*Rule{
		"": rule, // name to be filled after eval
	}
	ruleset.FetchEnviron, err = starutil.ExtractStringSlice(fetchEnviron)
	if err != nil {
		return nil, err
	}
	return ruleset, nil
}

func moduleRulesetFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("%v: unexpected positional arguments", b.Name())
	}
	var (
		resolveFn    starlark.Value
		fetchFn      starlark.Value
		members      *starlark.Dict
		fetchEnviron *starlark.List
	)
	ruleset := &Ruleset{Members: make(map[string]*Rule)}
	err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"resolve_fn", &resolveFn,
		"fetch_fn", &fetchFn,
		"members", &members,
		"doc?", &ruleset.Doc,
		"machine_specific?", &ruleset.MachineSpecific,
		"fetch_environ?", &fetchEnviron,
	)
	if err != nil {
		return nil, err
	}
	var ok bool
	ruleset.ResolveFn, ok = resolveFn.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("resolve_fn must be a function")
	}
	ruleset.FetchFn, ok = fetchFn.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("fetch_fn must be a function")
	}
	for _, item := range members.Items() {
		s, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("got %v, want string", item[0].Type())
		}
		// TODO: Check that `s` is a valid identifier
		member, ok := item[1].(*Rule)
		if !ok {
			return nil, fmt.Errorf("got %v, want module_rule", item[1].Type())
		}
		// TODO: What if this ruleset member has already been used elsewhere?
		member.Name = s
		member.Ruleset = ruleset
		ruleset.Members[s] = member
	}
	ruleset.FetchEnviron, err = starutil.ExtractStringSlice(fetchEnviron)
	if err != nil {
		return nil, err
	}
	return ruleset, nil
}

func moduleRulesetMemberFn(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("%v: unexpected positional arguments", b.Name())
	}
	var (
		attrs *starlark.Dict
	)
	rule := &Rule{}
	err := starlark.UnpackArgs(b.Name(), args, kwargs,
		"doc?", &rule.Doc,
		"attrs?", &attrs,
	)
	if err != nil {
		return nil, err
	}
	rule.Attrs, err = ExtractAttrMap(attrs)
	if err != nil {
		return nil, err
	}
	return rule, nil
}

type evalCacheEntry struct {
	globals starlark.StringDict
	err     error
}

type ResolveLabelResult struct {
	Repo     string
	Package  string
	Filename string
}

// LabelResolver tells Eval how to deal with load statements. Eval keeps track of the current repo and package name,
// and when a load from a label is encountered, it asks LabelResolver for the repo, package, and filename of the file
// that the label is pointing to.
type LabelResolver interface {
	ResolveLabel(curRepo string, curPackage string, label *common.Label) (*ResolveLabelResult, error)
}

// Eval is used to evaluate module rule exports. It has an internal cache, so it should be reused across multiple calls.
type Eval struct {
	cache         map[string]*evalCacheEntry
	predeclared   starlark.StringDict
	labelResolver LabelResolver
}

// NewEval creates a new Eval object with the given LabelResolver.
func NewEval(labelResolver LabelResolver) *Eval {
	return &Eval{
		cache: make(map[string]*evalCacheEntry),
		predeclared: starlark.StringDict{
			"ResolveResult":         starlark.NewBuiltin("ResolveResult", resolveResultFn),
			"attrs":                 attrModule,
			"struct":                starlark.NewBuiltin("struct", starlarkstruct.Make),
			"module_rule":           starlark.NewBuiltin("module_rule", moduleRuleFn),
			"module_ruleset":        starlark.NewBuiltin("module_ruleset", moduleRulesetFn),
			"module_ruleset_member": starlark.NewBuiltin("module_ruleset_member", moduleRulesetMemberFn),
		},
		labelResolver: labelResolver,
	}
}

// `exec` executes the file named by `result`, assuming the current repo and package are given by `result`.
func (e *Eval) exec(result *ResolveLabelResult) (starlark.StringDict, error) {
	src, err := ioutil.ReadFile(result.Filename)
	if err != nil {
		return nil, err
	}
	thread := &starlark.Thread{
		Name: "exec " + result.Filename,
		Load: func(t *starlark.Thread, rawLabel string) (starlark.StringDict, error) {
			label, err := common.ParseLabel(rawLabel)
			if err != nil {
				return nil, err
			}
			return e.load(result.Repo, result.Package, label)
		},
	}
	return starlark.ExecFile(thread, result.Filename, src, e.predeclared)
}

// Given the current repo and package, `load` loads and executes the file pointed to by the given label.
func (e *Eval) load(curRepo string, curPackage string, label *common.Label) (starlark.StringDict, error) {
	result, err := e.labelResolver.ResolveLabel(curRepo, curPackage, label)
	if err != nil {
		return nil, err
	}
	entry, ok := e.cache[result.Filename]
	if entry == nil {
		if ok {
			return nil, fmt.Errorf("cycle in load graph")
		}
		e.cache[result.Filename] = nil
		globals, err := e.exec(result)
		entry = &evalCacheEntry{globals, err}
		e.cache[result.Filename] = entry
	}
	return entry.globals, entry.err
}

// GetRulesets executes the "module rule exports" file of the module with the given key and "module_rule_exports"
// parameter, and returns the Ruleset objects in the resulting globals.
func (e *Eval) ExecForRulesets(key common.ModuleKey, repoName string, moduleRuleExports string) (map[string]*Ruleset, error) {
	label, err := common.ParseLabel(moduleRuleExports)
	if err != nil {
		return nil, err
	}
	if label.HasRepo {
		return nil, fmt.Errorf("module exports label must not specify a repo: %q", moduleRuleExports)
	}

	relLabel := &common.Label{HasRepo: false, HasPackage: false, Target: label.Target}
	globals, err := e.load(repoName, label.Package, relLabel)
	if err != nil {
		return nil, err
	}
	rulesets := make(map[string]*Ruleset)
	for name, global := range globals {
		ruleset, ok := global.(*Ruleset)
		if !ok {
			// Ignore any non-Ruleset globals.
			continue
		}
		ruleset.Name = name
		ruleset.ModuleKey = key
		rulesets[name] = ruleset

		// If the ruleset was created via a `module_rule()` call, it would have only 1 member with an empty name.
		// Rename the member to have the same name as the ruleset itself.
		if eponyRule, ok := ruleset.Members[""]; ok {
			delete(ruleset.Members, "")
			eponyRule.Name = name
			ruleset.Members[name] = eponyRule
		}
	}
	return rulesets, nil
}
