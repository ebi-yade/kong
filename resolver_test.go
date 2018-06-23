package kong_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alecthomas/kong"
)

type envMap map[string]string

func tempEnv(env envMap) func() {
	for k, v := range env {
		os.Setenv(k, v)
	}

	return func() {
		for k := range env {
			os.Unsetenv(k)
		}
	}
}

func newEnvParser(t *testing.T, cli interface{}, env envMap) (*kong.Kong, func()) {
	t.Helper()
	restoreEnv := tempEnv(env)
	parser := mustNew(t, cli)
	return parser, restoreEnv
}

func TestEnvarsFlagBasic(t *testing.T) {
	var cli struct {
		String string `env:"KONG_STRING"`
		Slice  []int  `env:"KONG_SLICE"`
	}
	parser, unsetEnvs := newEnvParser(t, &cli, envMap{
		"KONG_STRING": "bye",
		"KONG_SLICE":  "5,2,9",
	})
	defer unsetEnvs()

	_, err := parser.Parse([]string{})
	require.NoError(t, err)
	require.Equal(t, "bye", cli.String)
	require.Equal(t, []int{5, 2, 9}, cli.Slice)
}

func TestEnvarsFlagOverride(t *testing.T) {
	var cli struct {
		Flag string `env:"KONG_FLAG"`
	}
	parser, restoreEnv := newEnvParser(t, &cli, envMap{"KONG_FLAG": "bye"})
	defer restoreEnv()

	_, err := parser.Parse([]string{"--flag=hello"})
	require.NoError(t, err)
	require.Equal(t, "hello", cli.Flag)
}

func TestEnvarsOnlyPopulateUsedBranches(t *testing.T) {
	// nolint
	var cli struct {
		UnvisitedArg struct {
			UnvisitedArg string `arg`
			Int          int    `env:"KONG_INT"`
		} `arg`
		UnvisitedCmd struct {
			Int int `env:"KONG_INT"`
		} `cmd`
		Visited struct {
			Int int `env:"KONG_INT"`
		} `cmd`
	}
	parser, restoreEnv := newEnvParser(t, &cli, envMap{"KONG_INT": "512"})
	defer restoreEnv()

	_, err := parser.Parse([]string{"visited"})
	require.NoError(t, err)

	require.Equal(t, 512, cli.Visited.Int)
	require.Equal(t, 0, cli.UnvisitedArg.Int)
	require.Equal(t, 0, cli.UnvisitedCmd.Int)
}

func TestEnvarsTag(t *testing.T) {
	var cli struct {
		Slice []int `env:"KONG_NUMBERS"`
	}
	parser, restoreEnv := newEnvParser(t, &cli, envMap{"KONG_NUMBERS": "5,2,9"})
	defer restoreEnv()

	_, err := parser.Parse([]string{})
	require.NoError(t, err)
	require.Equal(t, []int{5, 2, 9}, cli.Slice)
}

func TestJSONBasic(t *testing.T) {
	var cli struct {
		String          string
		Slice           []int
		SliceWithCommas []string
		Bool            bool
	}

	json := `{
		"string": "🍕",
		"slice": [5, 8],
		"bool": true,
		"slice_with_commas": ["a,b", "c"]
	}`

	r, err := kong.JSON(strings.NewReader(json))
	require.NoError(t, err)

	parser := mustNew(t, &cli, kong.Resolver(r))
	_, err = parser.Parse([]string{})
	require.NoError(t, err)
	require.Equal(t, "🍕", cli.String)
	require.Equal(t, []int{5, 8}, cli.Slice)
	require.Equal(t, []string{"a,b", "c"}, cli.SliceWithCommas)
	require.True(t, cli.Bool)
}

func TestResolvedValueTriggersHooks(t *testing.T) {
	var cli struct {
		Int int
	}
	resolver := func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (string, error) {
		if flag.Name == "int" {
			return "1", nil
		}
		return "", nil
	}
	hooked := 0
	p := mustNew(t, &cli, kong.Resolver(resolver), kong.Hook(&cli.Int, func(ctx *kong.Context, path *kong.Path) error {
		hooked++
		return nil
	}))
	_, err := p.Parse(nil)
	require.NoError(t, err)
	require.Equal(t, 1, cli.Int)
	require.Equal(t, 1, hooked)

	hooked = 0
	_, err = p.Parse([]string{"--int=2"})
	require.NoError(t, err)
	require.Equal(t, 2, cli.Int)
	require.Equal(t, 1, hooked)
}

type testUppercaseMapper struct{}

func (testUppercaseMapper) Decode(ctx *kong.DecodeContext, target reflect.Value) error {
	value := ctx.Scan.PopValue("lowercase")
	target.SetString(strings.ToUpper(value))
	return nil
}

func TestResolversWithMappers(t *testing.T) {
	var cli struct {
		Flag string `env:"KONG_MOO" type:"upper"`
	}

	restoreEnv := tempEnv(envMap{"KONG_MOO": "meow"})
	defer restoreEnv()

	parser := mustNew(t, &cli,
		kong.NamedMapper("upper", testUppercaseMapper{}),
	)
	_, err := parser.Parse([]string{})
	require.NoError(t, err)
	require.Equal(t, "MEOW", cli.Flag)
}

func TestResolverWithBool(t *testing.T) {
	var cli struct {
		Bool bool
	}

	resolver := func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (string, error) {
		if flag.Name == "bool" {
			return "true", nil
		}
		return "", nil
	}

	p := mustNew(t, &cli, kong.Resolver(resolver))

	_, err := p.Parse(nil)
	require.NoError(t, err)
	require.True(t, cli.Bool)
}

func TestLastResolverWins(t *testing.T) {
	var cli struct {
		Int []int
	}

	var first kong.ResolverFunc = func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (string, error) {
		if flag.Name == "int" {
			return "1", nil
		}
		return "", nil
	}

	var second kong.ResolverFunc = func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (string, error) {
		if flag.Name == "int" {
			return "2", nil
		}
		return "", nil
	}

	p := mustNew(t, &cli, kong.Resolver(first), kong.Resolver(second))
	_, err := p.Parse(nil)
	require.NoError(t, err)
	require.Equal(t, []int{2}, cli.Int)
}

func TestResolverSatisfiesRequired(t *testing.T) {
	// nolint: govet
	var cli struct {
		Int int `required`
	}
	resolver := func(context *kong.Context, parent *kong.Path, flag *kong.Flag) (string, error) {
		if flag.Name == "int" {
			return "1", nil
		}
		return "", nil
	}
	_, err := mustNew(t, &cli, kong.Resolver(resolver)).Parse(nil)
	require.NoError(t, err)
	require.Equal(t, 1, cli.Int)
}