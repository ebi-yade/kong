package kong

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestOptions(t *testing.T) {
	var cli struct{}
	p, err := New(&cli, Name("name"), Description("description"), Writers(nil, nil), Exit(nil))
	assert.NoError(t, err)
	assert.Equal(t, "name", p.Model.Name)
	assert.Equal(t, "description", p.Model.Help)
	assert.Zero(t, p.Stdout)
	assert.Zero(t, p.Stderr)
	assert.Zero(t, p.Exit)
}

type impl string

func (impl) Method() {}

func TestBindTo(t *testing.T) {
	type iface interface {
		Method()
	}

	saw := ""
	method := func(i iface) error {
		saw = string(i.(impl)) //nolint
		return nil
	}

	var cli struct{}

	p, err := New(&cli, BindTo(impl("foo"), (*iface)(nil)))
	assert.NoError(t, err)
	err = callFunction(reflect.ValueOf(method), p.bindings)
	assert.NoError(t, err)
	assert.Equal(t, "foo", saw)
}

func TestInvalidCallback(t *testing.T) {
	type iface interface {
		Method()
	}

	saw := ""
	method := func(i iface) string {
		saw = string(i.(impl)) //nolint
		return saw
	}

	var cli struct{}

	p, err := New(&cli, BindTo(impl("foo"), (*iface)(nil)))
	assert.NoError(t, err)
	err = callFunction(reflect.ValueOf(method), p.bindings)
	assert.EqualError(t, err, `return value of func(kong.iface) string must implement "error"`)
}

type zrror struct{}

func (*zrror) Error() string {
	return "error"
}

func TestCallbackCustomError(t *testing.T) {
	type iface interface {
		Method()
	}

	saw := ""
	method := func(i iface) *zrror {
		saw = string(i.(impl)) //nolint
		return nil
	}

	var cli struct{}

	p, err := New(&cli, BindTo(impl("foo"), (*iface)(nil)))
	assert.NoError(t, err)
	err = callFunction(reflect.ValueOf(method), p.bindings)
	assert.NoError(t, err)
	assert.Equal(t, "foo", saw)
}

type bindToProviderCLI struct {
	Filled bool `default:"true"`
	Called bool
	Cmd    bindToProviderCmd `cmd:""`
}

type boundThing struct {
	Filled bool
}

type bindToProviderCmd struct{}

func (*bindToProviderCmd) Run(cli *bindToProviderCLI, b *boundThing) error {
	cli.Called = true
	return nil
}

func TestBindToProvider(t *testing.T) {
	var cli bindToProviderCLI
	app, err := New(&cli, BindToProvider(func(cli *bindToProviderCLI) (*boundThing, error) {
		assert.True(t, cli.Filled, "CLI struct should have already been populated by Kong")
		return &boundThing{Filled: cli.Filled}, nil
	}))
	assert.NoError(t, err)
	ctx, err := app.Parse([]string{"cmd"})
	assert.NoError(t, err)
	err = ctx.Run()
	assert.NoError(t, err)
	assert.True(t, cli.Called)
}

func TestFlagNamer(t *testing.T) {
	var cli struct {
		SomeFlag string
	}
	app, err := New(&cli, FlagNamer(strings.ToUpper))
	assert.NoError(t, err)
	assert.Equal(t, "SOMEFLAG", app.Model.Flags[1].Name)
}

type npError string

func (e npError) Error() string {
	return "ERROR: " + string(e)
}

func TestCallbackNonPointerError(t *testing.T) {
	method := func() error {
		return npError("failed")
	}

	var cli struct{}

	p, err := New(&cli)
	assert.NoError(t, err)
	err = callFunction(reflect.ValueOf(method), p.bindings)
	assert.EqualError(t, err, "ERROR: failed")
}
